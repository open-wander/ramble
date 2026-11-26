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
	Token     string `json:"token,omitempty"`     // API token stored by "ramble login"
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

	// Clean path to normalize
	cleanPath := filepath.Clean(path)

	// G304: Config path is derived from XDG_CONFIG_HOME or user home directory,
	// not from user input. This is the standard config file location.
	data, err := os.ReadFile(cleanPath) //#nosec G304 -- config path from XDG/home, not user input
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
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	// Write a temp file in the same directory (created 0600) and rename it
	// over the config. The rename replaces the inode, so a pre-existing file
	// with wider permissions ends up 0600 too.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	tmpName := tmp.Name()
	if err := writeAndClose(tmp, data); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to write config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to write config: %w", err)
	}
	return nil
}

func writeAndClose(f *os.File, data []byte) error {
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// AddRegistry adds or updates a registry
//
// Re-adding an existing registry keeps its stored token only when the URL is
// unchanged; a token must never follow a registry to a different host.
func (c *Config) AddRegistry(name, url, namespace string) {
	reg := Registry{URL: url, Namespace: namespace}
	if old, ok := c.Registries[name]; ok && old.URL == url {
		reg.Token = old.Token
	}
	c.Registries[name] = reg
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
	_, reg, err := c.ResolveRegistry(name)
	return reg, err
}

// ResolveRegistry is GetRegistry that also returns the resolved name, which
// differs from the argument when it is empty and the default is used.
func (c *Config) ResolveRegistry(name string) (string, *Registry, error) {
	if name == "" {
		name = c.DefaultRegistry
	}
	if name == "" {
		return "", nil, fmt.Errorf("no registry specified and no default set")
	}

	reg, ok := c.Registries[name]
	if !ok {
		return "", nil, fmt.Errorf("registry not found: %s", name)
	}
	return name, &reg, nil
}

// EnvToken is the environment variable that overrides stored tokens.
const EnvToken = "RAMBLE_TOKEN"

// TokenFor returns the API token to use for the named registry. Precedence:
// a non-empty RAMBLE_TOKEN environment variable, then the token stored by
// "ramble login", then "".
func (c *Config) TokenFor(name string) string {
	if t := os.Getenv(EnvToken); t != "" {
		return t
	}
	return c.Registries[name].Token
}

// SetToken stores token for the named registry.
func (c *Config) SetToken(name, token string) error {
	reg, ok := c.Registries[name]
	if !ok {
		return fmt.Errorf("registry not found: %s", name)
	}
	reg.Token = token
	c.Registries[name] = reg
	return nil
}

// ClearToken removes the stored token for the named registry and reports
// whether there was one.
func (c *Config) ClearToken(name string) (bool, error) {
	reg, ok := c.Registries[name]
	if !ok {
		return false, fmt.Errorf("registry not found: %s", name)
	}
	had := reg.Token != ""
	reg.Token = ""
	c.Registries[name] = reg
	return had, nil
}

// GetDefaultURL returns the URL of the default registry
func (c *Config) GetDefaultURL() string {
	if reg, err := c.GetRegistry(""); err == nil {
		return reg.URL
	}
	return "https://ramble.openwander.org"
}
