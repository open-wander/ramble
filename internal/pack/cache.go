package pack

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"rmbl/internal/security"
)

// Cache manages locally cached packs
type Cache struct {
	Dir string
}

// NewCache creates a new cache using the default cache directory
func NewCache() *Cache {
	return &Cache{
		Dir: getCacheDir(),
	}
}

// getCacheDir returns the cache directory path
func getCacheDir() string {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "ramble", "packs")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "ramble", "packs")
}

// PackPath returns the path where a pack would be cached
func (c *Cache) PackPath(registry, namespace, name, version string) string {
	host := urlToHost(registry)
	return filepath.Join(c.Dir, host, namespace, name, version)
}

// IsCached checks if a pack is already cached
func (c *Cache) IsCached(registry, namespace, name, version string) bool {
	path := c.PackPath(registry, namespace, name, version)
	_, err := os.Stat(filepath.Join(path, "metadata.hcl"))
	return err == nil
}

// Load loads a cached pack
func (c *Cache) Load(registry, namespace, name, version string) (string, error) {
	path := c.PackPath(registry, namespace, name, version)
	if !c.IsCached(registry, namespace, name, version) {
		return "", fmt.Errorf("pack not cached")
	}
	return path, nil
}

// Store downloads and caches a pack from a tarball URL
func (c *Cache) Store(registry, namespace, name, version, tarballURL string) (string, error) {
	packPath := c.PackPath(registry, namespace, name, version)

	// Create cache directory
	if err := os.MkdirAll(packPath, 0750); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Validate and parse URL before making request
	parsedURL, err := url.Parse(tarballURL)
	if err != nil {
		return "", fmt.Errorf("invalid tarball URL: %w", err)
	}
	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		return "", fmt.Errorf("unsupported URL scheme: %s (must be http or https)", parsedURL.Scheme)
	}

	// Create SSRF-protected HTTP client
	// G107: URL is validated above and uses SSRF-protected client with host allowlist
	client, err := security.NewProtectedClient(security.SSRFConfig{
		AllowedHosts: security.DefaultAllowedHosts(),
		Timeout:      30 * time.Second,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP client: %w", err)
	}

	// Download tarball using protected client
	resp, err := client.Get(tarballURL) //#nosec G107 -- URL validated and uses SSRF-protected client
	if err != nil {
		return "", fmt.Errorf("failed to download pack: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download pack: status %d", resp.StatusCode)
	}

	// Extract tarball
	if err := extractTarGz(resp.Body, packPath); err != nil {
		// Clean up on failure - best effort, ignore error
		_ = os.RemoveAll(packPath)
		return "", fmt.Errorf("failed to extract pack: %w", err)
	}

	return packPath, nil
}

// Clear removes all cached packs
func (c *Cache) Clear() error {
	return os.RemoveAll(c.Dir)
}

// List returns all cached packs
func (c *Cache) List() ([]CachedPack, error) {
	var packs []CachedPack

	err := filepath.Walk(c.Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.Name() == "metadata.hcl" {
			// Found a pack
			rel, _ := filepath.Rel(c.Dir, filepath.Dir(path))
			parts := strings.Split(rel, string(filepath.Separator))
			if len(parts) >= 4 {
				packs = append(packs, CachedPack{
					Registry:  parts[0],
					Namespace: parts[1],
					Name:      parts[2],
					Version:   parts[3],
					Path:      filepath.Dir(path),
				})
			}
		}
		return nil
	})

	return packs, err
}

// CachedPack represents a pack in the cache
type CachedPack struct {
	Registry  string
	Namespace string
	Name      string
	Version   string
	Path      string
}

// urlToHost extracts the host from a URL
func urlToHost(rawURL string) string {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "unknown"
	}
	return u.Host
}

// safeFileMode safely converts int64 to os.FileMode (uint32) with bounds checking.
// Invalid or out-of-range values return a safe default mode (0640 for files).
func safeFileMode(mode int64) os.FileMode {
	if mode < 0 || mode > math.MaxUint32 {
		return 0640 // Safe default for files
	}
	return os.FileMode(mode)
}

// extractTarGz extracts a tar.gz archive to a directory
func extractTarGz(r io.Reader, destDir string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	// Track the root directory name to strip it
	var rootDir string

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar: %w", err)
		}

		// Get the path relative to root
		name := header.Name

		// First entry usually contains the root directory
		if rootDir == "" {
			parts := strings.SplitN(name, "/", 2)
			if len(parts) > 0 {
				rootDir = parts[0] + "/"
			}
		}

		// Strip root directory
		if strings.HasPrefix(name, rootDir) {
			name = strings.TrimPrefix(name, rootDir)
		}

		if name == "" {
			continue
		}

		target := filepath.Join(destDir, filepath.Clean(name))

		// Security check: ensure target is within destDir
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0750); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg:
			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}

			// G304: target path is validated above (lines 194-199) to be within destDir
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, safeFileMode(header.Mode)) //#nosec G304 -- path validated to be within destDir
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}

			// Limit decompressed file size to prevent decompression bombs (G110)
			const maxFileSize = 100 * 1024 * 1024 // 100MB per file
			limitedReader := io.LimitReader(tr, maxFileSize+1)
			n, err := io.Copy(f, limitedReader)
			if err != nil {
				_ = f.Close() // Close file on write error, ignore close error
				return fmt.Errorf("failed to write file: %w", err)
			}
			if n > maxFileSize {
				_ = f.Close() // Close file on size limit, ignore close error
				return fmt.Errorf("file %s exceeds maximum size of %d bytes", name, maxFileSize)
			}
			if err := f.Close(); err != nil {
				return fmt.Errorf("failed to close file: %w", err)
			}
		}
	}

	return nil
}
