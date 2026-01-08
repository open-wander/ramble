package config

import (
	"os"
	"path/filepath"
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
