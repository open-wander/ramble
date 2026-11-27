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

	"rmbl/internal/names"
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

// maxSegmentLen caps a namespace, name or host used as a cache path segment.
const maxSegmentLen = 253

// validateSegment checks that a registry-supplied value is safe to use as a
// single directory name. The field name is used in the error.
func validateSegment(field, v string) error {
	bad := v == "" || len(v) > maxSegmentLen || v == "." || v == ".." ||
		!filepath.IsLocal(v) || strings.ContainsAny(v, "/\\:")
	if !bad {
		for _, r := range v {
			if r < 0x20 || r == 0x7f {
				bad = true
				break
			}
		}
	}
	if bad {
		return fmt.Errorf("invalid %s %q from registry", field, Printable(v))
	}
	return nil
}

// validatePackRef checks every registry-supplied component of a pack
// reference before any path or URL is built from it.
func validatePackRef(host, namespace, name, version string) error {
	if err := validateSegment("registry host", host); err != nil {
		return err
	}
	if err := validateSegment("namespace", namespace); err != nil {
		return err
	}
	if err := validateSegment("name", name); err != nil {
		return err
	}
	if !names.IsValidVersion(version) {
		return fmt.Errorf("invalid version %q from registry", Printable(version))
	}
	return nil
}

// packRel validates the reference and returns its cache-relative path.
func packRel(registry, namespace, name, version string) (string, error) {
	host := strings.ReplaceAll(urlToHost(registry), ":", "_")
	if err := validatePackRef(host, namespace, name, version); err != nil {
		return "", err
	}
	rel := filepath.Join(host, namespace, name, version)
	if !filepath.IsLocal(rel) {
		return "", fmt.Errorf("invalid pack reference %q", Printable(rel))
	}
	return rel, nil
}

// PackPath returns the path where a pack would be cached. It fails when any
// component could place the path outside the cache directory.
func (c *Cache) PackPath(registry, namespace, name, version string) (string, error) {
	rel, err := packRel(registry, namespace, name, version)
	if err != nil {
		return "", err
	}
	return filepath.Join(c.Dir, rel), nil
}

// IsCached checks if a pack is already cached. An invalid reference is never
// cached.
func (c *Cache) IsCached(registry, namespace, name, version string) bool {
	path, err := c.PackPath(registry, namespace, name, version)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(path, "metadata.hcl"))
	return err == nil
}

// Load loads a cached pack
func (c *Cache) Load(registry, namespace, name, version string) (string, error) {
	path, err := c.PackPath(registry, namespace, name, version)
	if err != nil {
		return "", err
	}
	if !c.IsCached(registry, namespace, name, version) {
		return "", fmt.Errorf("pack not cached")
	}
	return path, nil
}

// Store downloads and caches a pack from a tarball URL
func (c *Cache) Store(registry, namespace, name, version, tarballURL string) (string, error) {
	// Validate everything before touching the filesystem.
	rel, err := packRel(registry, namespace, name, version)
	if err != nil {
		return "", err
	}

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

	if err := os.MkdirAll(c.Dir, 0750); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}
	root, err := os.OpenRoot(c.Dir)
	if err != nil {
		return "", fmt.Errorf("failed to open cache directory: %w", err)
	}
	defer root.Close()

	if err := root.MkdirAll(rel, 0750); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	if err := extractTarGz(resp.Body, root, rel); err != nil {
		// Clean up on failure - best effort, ignore error
		_ = root.RemoveAll(rel)
		return "", fmt.Errorf("failed to extract pack: %w", err)
	}

	return filepath.Join(c.Dir, rel), nil
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

// extractTarGz extracts a tar.gz archive into destRel inside root. All writes
// go through root, so nothing can land outside it. Entry names that are
// absolute or contain ".." fail the extraction; symlink, hardlink and other
// special entries are skipped.
func extractTarGz(r io.Reader, root *os.Root, destRel string) error {
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

		name := header.Name

		// First entry usually contains the root directory
		if rootDir == "" {
			parts := strings.SplitN(name, "/", 2)
			if len(parts) > 0 {
				rootDir = parts[0] + "/"
			}
		}

		// Reject absolute and parent-relative names before stripping the
		// root directory, so "root/../x" and "/abs" cannot slip through.
		if strings.HasPrefix(name, "/") || !filepath.IsLocal(filepath.FromSlash(name)) {
			return fmt.Errorf("unsafe path in archive: %q", Printable(name))
		}

		name = strings.TrimPrefix(name, rootDir)
		if name == "" || name == "." {
			continue
		}
		if !filepath.IsLocal(filepath.FromSlash(name)) {
			return fmt.Errorf("unsafe path in archive: %q", Printable(name))
		}

		target := filepath.Join(destRel, filepath.FromSlash(name))

		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(target, 0750); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg:
			if err := root.MkdirAll(filepath.Dir(target), 0750); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}

			f, err := root.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, safeFileMode(header.Mode))
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
				return fmt.Errorf("file %s exceeds maximum size of %d bytes", Printable(name), maxFileSize)
			}
			if err := f.Close(); err != nil {
				return fmt.Errorf("failed to close file: %w", err)
			}
		default:
			// Symlinks, hardlinks and special files are not extracted.
		}
	}

	return nil
}
