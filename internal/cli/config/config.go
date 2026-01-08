package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Registry represents a saved registry configuration
type Registry struct {
	URL       string `json:"url"`
	Namespace string `json:"namespace,omitempty"` // Optional: limit to specific namespace
}

// Config holds CLI configuration
type Config struct {
	DefaultRegistry string              `json:"default_registry"`
	Registries      map[string]Registry `json:"registries"`
}

// configPath returns the path to the config file
func configPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "ramble", "config.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ramble", "config.json")
}

// Load loads the config from disk
func Load() (*Config, error) {
	path := configPath()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Return default config
		return &Config{
			DefaultRegistry: "ramble",
			Registries: map[string]Registry{
				"ramble": {URL: "https://ramble.openwander.org"},
			},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if cfg.Registries == nil {
		cfg.Registries = make(map[string]Registry)
	}

	return &cfg, nil
}

// Save saves the config to disk
func (c *Config) Save() error {
	path := configPath()

	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// AddRegistry adds or updates a registry
func (c *Config) AddRegistry(name, url, namespace string) {
	c.Registries[name] = Registry{
		URL:       url,
		Namespace: namespace,
	}
}

// RemoveRegistry removes a registry
func (c *Config) RemoveRegistry(name string) error {
	if _, ok := c.Registries[name]; !ok {
		return fmt.Errorf("registry not found: %s", name)
	}
	delete(c.Registries, name)

	// Reset default if we deleted it
	if c.DefaultRegistry == name {
		c.DefaultRegistry = ""
	}
	return nil
}

// SetDefault sets the default registry
func (c *Config) SetDefault(name string) error {
	if _, ok := c.Registries[name]; !ok {
		return fmt.Errorf("registry not found: %s", name)
	}
	c.DefaultRegistry = name
	return nil
}

// GetRegistry returns a registry by name, or the default
func (c *Config) GetRegistry(name string) (*Registry, error) {
	if name == "" {
		name = c.DefaultRegistry
	}
	if name == "" {
		return nil, fmt.Errorf("no registry specified and no default set")
	}

	reg, ok := c.Registries[name]
	if !ok {
		return nil, fmt.Errorf("registry not found: %s", name)
	}
	return &reg, nil
}

// GetDefaultURL returns the URL of the default registry
func (c *Config) GetDefaultURL() string {
	if reg, err := c.GetRegistry(""); err == nil {
		return reg.URL
	}
	return "https://ramble.openwander.org"
}
