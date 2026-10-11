package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/gofiber/fiber/v2"
	"golang.org/x/sync/errgroup"
	"rmbl/internal/database"
	"rmbl/internal/models"
	"rmbl/internal/services/logger"
	"rmbl/internal/services/resource"
)

// isPermanentError determines if an error should stop retries immediately
func isPermanentError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// HTTP status codes that are permanent
	if strings.Contains(errStr, "404") || strings.Contains(errStr, "not found") {
		return true
	}
	if strings.Contains(errStr, "403") || strings.Contains(errStr, "forbidden") {
		return true
	}
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "unauthorized") {
		return true
	}

	// Security/validation errors that won't be fixed by retrying
	if strings.Contains(errStr, "ssrf") || strings.Contains(errStr, "blocked") {
		return true
	}
	if strings.Contains(errStr, "invalid url") || strings.Contains(errStr, "malformed") {
		return true
	}

	return false
}

// stripURLUserinfo removes any user:password@ or token@ from a URL so it is
// safe to log. Unparseable input yields a placeholder, never the raw string.
func stripURLUserinfo(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return "[unparseable url]"
	}
	u.User = nil
	return u.String()
}

// downloadFileWithRetry wraps downloadFile with exponential backoff retry logic
func downloadFileWithRetry(ctx context.Context, repoURL, fileName string) (string, error) {
	log := logger.FromContextOr(ctx, &logger.Log).With().
		Str("repo_url", stripURLUserinfo(repoURL)).
		Str("file_name", fileName).
		Logger()

	// Configure exponential backoff
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 1 * time.Second
	expBackoff.Multiplier = 5.0
	expBackoff.MaxInterval = 15 * time.Second
	expBackoff.MaxElapsedTime = 30 * time.Second
	expBackoff.RandomizationFactor = 0.5

	// Limit to 3 attempts with context cancellation
	b := backoff.WithMaxRetries(expBackoff, 3)
	b = backoff.WithContext(b, ctx)

	var result string
	var lastErr error
	attempt := 0

	operation := func() error {
		attempt++
		log.Debug().Int("attempt", attempt).Msg("download.attempt")

		content, err := downloadFile(repoURL, fileName)
		if err != nil {
			lastErr = err

			if isPermanentError(err) {
				log.Warn().Err(err).Int("attempt", attempt).Msg("download.permanent_error")
				return backoff.Permanent(err)
			}

			log.Warn().Err(err).Int("attempt", attempt).Msg("download.transient_error")
			return err
		}

		result = content
		if attempt > 1 {
			log.Info().Int("total_attempts", attempt).Msg("download.retry_succeeded")
		}
		return nil
	}

	err := backoff.Retry(operation, b)
	if err != nil {
		log.Error().Err(err).Int("total_attempts", attempt).Msg("download.failed")
		return "", lastErr
	}

	return result, nil
}

// detachedLogCtx returns a context carrying the request logger (and so its
// request_id) for a goroutine that outlives the request. It is rebuilt on
// context.Background() so request cancellation never reaches the goroutine,
// and it never captures c.
func detachedLogCtx(c *fiber.Ctx) context.Context {
	return logger.FromContext(c.UserContext()).WithContext(context.Background())
}

// fetchVersionHook, when set by tests, runs inside fetchVersionContent after the
// fetching status is written. Nil in production.
var fetchVersionHook func()

// fetchVersionContent handles the complete lifecycle of fetching resource version content
// This function is spawned as a goroutine and handles all file downloads for a version
// ctx carries the logger of the request that triggered the fetch (see
// detachedLogCtx); with none, the global logger is used.
func fetchVersionContent(ctx context.Context, resID uint, versionStr, repoURL, resType, resName, filePath string) {
	log := logger.FromContextOr(ctx, &logger.Log).With().
		Str("operation", "version_fetch").
		Uint("resource_id", resID).
		Str("version", versionStr).
		Logger()

	// Catch panics to ensure we always record failures
	defer func() {
		if r := recover(); r != nil {
			log.Error().
				Interface("panic", r).
				Stack().
				Msg("version_fetch.panic")

			// Persist panic as failure
			if err := resource.Service.WithContext(ctx).RecordFetchFailed(resID, versionStr, fmt.Sprintf("panic: %v", r)); err != nil {
				log.Error().Err(err).Msg("version_fetch.panic_record_failed")
			}
		}
	}()

	startTime := time.Now()

	// Set status to fetching
	now := time.Now()
	database.DB.WithContext(ctx).Model(&models.ResourceVersion{}).
		Where("resource_id = ? AND version = ?", resID, versionStr).
		Updates(map[string]interface{}{
			"fetch_status":     models.FetchStatusFetching,
			"fetch_started_at": now,
		})

	if fetchVersionHook != nil {
		fetchVersionHook()
	}

	// Use errgroup for parallel file downloads
	g, gctx := errgroup.WithContext(ctx)

	var readme, content, variablesJSON string
	var readmeErr, contentErr, varsErr error

	// Download README (non-fatal if fails)
	g.Go(func() error {
		var err error
		readme, err = downloadFileWithRetry(gctx, repoURL, "README.md")
		readmeErr = err
		return nil // Don't fail the group for README
	})

	// Download content file (fatal if fails)
	g.Go(func() error {
		var err error
		if resType == string(models.ResourceTypeJob) {
			fetchPath := filePath
			if fetchPath == "" {
				fetchPath = resName
				if !strings.HasSuffix(fetchPath, ".nomad.hcl") {
					fetchPath = fetchPath + ".nomad.hcl"
				}
			}
			content, err = downloadFileWithRetry(gctx, repoURL, fetchPath)
			contentErr = err
			if err != nil {
				return fmt.Errorf("failed to fetch job file: %w", err)
			}
		} else if resType == string(models.ResourceTypePack) {
			content, err = downloadFileWithRetry(gctx, repoURL, "metadata.hcl")
			contentErr = err
			if err != nil {
				return fmt.Errorf("failed to fetch metadata.hcl: %w", err)
			}
		}
		return nil
	})

	// Download variables for pack type (non-fatal if fails)
	if resType == string(models.ResourceTypePack) {
		g.Go(func() error {
			varsContent, err := downloadFileWithRetry(gctx, repoURL, "variables.hcl")
			varsErr = err
			if err == nil && varsContent != "" {
				if vars, err := parsePackVariables(varsContent); err == nil {
					if b, err := json.Marshal(vars); err == nil {
						variablesJSON = string(b)
					}
				}
			}
			return nil // Don't fail the group for variables
		})
	}

	// Wait for all downloads to complete
	if err := g.Wait(); err != nil {
		// Primary content failed
		duration := time.Since(startTime)
		log.Error().
			Err(err).
			Dur("duration", duration).
			Msg("version_fetch.failed")

		if recErr := resource.Service.WithContext(ctx).RecordFetchFailed(resID, versionStr, err.Error()); recErr != nil {
			log.Error().Err(recErr).Msg("version_fetch.failure_record_failed")
		}
		return
	}

	// Success - update with all fetched content
	duration := time.Since(startTime)
	log.Info().
		Dur("duration", duration).
		Bool("readme_fetched", readmeErr == nil).
		Bool("content_fetched", contentErr == nil).
		Bool("variables_fetched", varsErr == nil).
		Msg("version_fetch.completed")

	now = time.Now()
	database.DB.WithContext(ctx).Model(&models.ResourceVersion{}).
		Where("resource_id = ? AND version = ?", resID, versionStr).
		Updates(map[string]interface{}{
			"readme":             readme,
			"content":            content,
			"variables":          variablesJSON,
			"fetch_status":       models.FetchStatusCompleted,
			"fetch_error":        "",
			"fetch_completed_at": now,
		})
}
