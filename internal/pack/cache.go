package pack

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
	if err := os.MkdirAll(packPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Download tarball
	resp, err := http.Get(tarballURL)
	if err != nil {
		return "", fmt.Errorf("failed to download pack: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download pack: status %d", resp.StatusCode)
	}

	// Extract tarball
	if err := extractTarGz(resp.Body, packPath); err != nil {
		// Clean up on failure
		os.RemoveAll(packPath)
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
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg:
			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}

			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}

			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return fmt.Errorf("failed to write file: %w", err)
			}
			f.Close()
		}
	}

	return nil
}
