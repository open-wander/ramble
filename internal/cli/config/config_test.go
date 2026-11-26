package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaultConfig(t *testing.T) {
	// Use a temp directory so we don't interfere with real config
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	cfg, err := Load()
	require.NoError(t, err)

	// Default config should have ramble registry
	assert.Equal(t, "ramble", cfg.DefaultRegistry)
	assert.Contains(t, cfg.Registries, "ramble")
	assert.Equal(t, "https://ramble.openwander.org", cfg.Registries["ramble"].URL)
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create and save a config
	cfg := &Config{
		DefaultRegistry: "myregistry",
		Registries: map[string]Registry{
			"myregistry": {URL: "https://example.com", Namespace: "myteam"},
		},
	}
	require.NoError(t, cfg.Save())

	// Verify file was created
	configPath := filepath.Join(tmpDir, "ramble", "config.json")
	_, err := os.Stat(configPath)
	require.NoError(t, err)

	// Load and verify
	loaded, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "myregistry", loaded.DefaultRegistry)
	assert.Equal(t, "https://example.com", loaded.Registries["myregistry"].URL)
	assert.Equal(t, "myteam", loaded.Registries["myregistry"].Namespace)
}

func TestAddRegistry(t *testing.T) {
	cfg := &Config{
		Registries: make(map[string]Registry),
	}

	cfg.AddRegistry("test", "https://test.com", "")
	assert.Contains(t, cfg.Registries, "test")
	assert.Equal(t, "https://test.com", cfg.Registries["test"].URL)
	assert.Empty(t, cfg.Registries["test"].Namespace)

	cfg.AddRegistry("namespaced", "https://test.com", "myns")
	assert.Equal(t, "myns", cfg.Registries["namespaced"].Namespace)
}

func TestRemoveRegistry(t *testing.T) {
	cfg := &Config{
		DefaultRegistry: "test",
		Registries: map[string]Registry{
			"test":  {URL: "https://test.com"},
			"other": {URL: "https://other.com"},
		},
	}

	// Remove existing registry
	err := cfg.RemoveRegistry("test")
	require.NoError(t, err)
	assert.NotContains(t, cfg.Registries, "test")
	assert.Empty(t, cfg.DefaultRegistry) // Default was cleared

	// Remove non-existent registry
	err = cfg.RemoveRegistry("nonexistent")
	assert.Error(t, err)
}

func TestSetDefault(t *testing.T) {
	cfg := &Config{
		Registries: map[string]Registry{
			"a": {URL: "https://a.com"},
			"b": {URL: "https://b.com"},
		},
	}

	// Set valid default
	err := cfg.SetDefault("a")
	require.NoError(t, err)
	assert.Equal(t, "a", cfg.DefaultRegistry)

	// Set invalid default
	err = cfg.SetDefault("nonexistent")
	assert.Error(t, err)
}

func TestGetRegistry(t *testing.T) {
	cfg := &Config{
		DefaultRegistry: "default",
		Registries: map[string]Registry{
			"default": {URL: "https://default.com"},
			"other":   {URL: "https://other.com"},
		},
	}

	// Get by name
	reg, err := cfg.GetRegistry("other")
	require.NoError(t, err)
	assert.Equal(t, "https://other.com", reg.URL)

	// Get default (empty name)
	reg, err = cfg.GetRegistry("")
	require.NoError(t, err)
	assert.Equal(t, "https://default.com", reg.URL)

	// Get non-existent
	_, err = cfg.GetRegistry("nonexistent")
	assert.Error(t, err)
}

func TestGetDefaultURL(t *testing.T) {
	cfg := &Config{
		DefaultRegistry: "custom",
		Registries: map[string]Registry{
			"custom": {URL: "https://custom.com"},
		},
	}

	assert.Equal(t, "https://custom.com", cfg.GetDefaultURL())

	// When no default is set
	cfg2 := &Config{}
	assert.Equal(t, "https://ramble.openwander.org", cfg2.GetDefaultURL())
}

func TestTokenFor(t *testing.T) {
	cfg := &Config{Registries: map[string]Registry{
		"a": {URL: "https://a.example", Token: "stored"},
		"b": {URL: "https://b.example"},
	}}
	tests := []struct {
		name, env, reg, want string
	}{
		{"stored", "", "a", "stored"},
		{"env overrides stored", "fromenv", "a", "fromenv"},
		{"env without stored", "fromenv", "b", "fromenv"},
		{"none", "", "b", ""},
		{"unknown registry", "", "zzz", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvToken, tt.env)
			assert.Equal(t, tt.want, cfg.TokenFor(tt.reg))
		})
	}
}

func TestSetAndClearToken(t *testing.T) {
	cfg := &Config{Registries: map[string]Registry{"a": {URL: "https://a.example"}}}
	require.NoError(t, cfg.SetToken("a", "tok"))
	assert.Equal(t, "tok", cfg.Registries["a"].Token)
	assert.Error(t, cfg.SetToken("nope", "tok"))

	had, err := cfg.ClearToken("a")
	require.NoError(t, err)
	assert.True(t, had)
	had, err = cfg.ClearToken("a")
	require.NoError(t, err)
	assert.False(t, had)
	_, err = cfg.ClearToken("nope")
	assert.Error(t, err)
}

func TestSaveTightensExistingMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes only")
	}
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "ramble", "config.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte("{}"), 0o644)) //nolint:gosec
	require.NoError(t, os.Chmod(path, 0o644))                   //nolint:gosec

	cfg := &Config{Registries: map[string]Registry{"a": {URL: "https://a.example", Token: "tok"}}}
	require.NoError(t, cfg.Save())

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temp files left behind")
}

func TestAddRegistryToken(t *testing.T) {
	cfg := &Config{Registries: map[string]Registry{"a": {URL: "https://a.example", Token: "tok"}}}
	cfg.AddRegistry("a", "https://a.example", "ns")
	assert.Equal(t, "tok", cfg.Registries["a"].Token, "same URL keeps token")
	cfg.AddRegistry("a", "https://other.example", "ns")
	assert.Empty(t, cfg.Registries["a"].Token, "changed URL drops token")
}
