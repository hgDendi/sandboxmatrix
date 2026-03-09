// Package config manages sandboxMatrix configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	// DefaultConfigDir is the default configuration directory name.
	DefaultConfigDir = ".sandboxmatrix"
	// DefaultConfigFile is the default configuration file name.
	DefaultConfigFile = "config.yaml"
)

// Config holds the sandboxMatrix configuration.
type Config struct {
	DefaultRuntime string          `yaml:"defaultRuntime" json:"defaultRuntime"`
	BlueprintDir   string          `yaml:"blueprintDir" json:"blueprintDir"`
	StateDir       string          `yaml:"stateDir" json:"stateDir"`
	LogLevel       string          `yaml:"logLevel" json:"logLevel"`
	Server         ServerConfig    `yaml:"server" json:"server"`
	Dashboard      DashboardConfig `yaml:"dashboard" json:"dashboard"`
	Pool           PoolConfig      `yaml:"pool" json:"pool"`
}

// ServerConfig holds the API server configuration.
type ServerConfig struct {
	Addr string `yaml:"addr" json:"addr"`
}

// DashboardConfig holds the dashboard configuration.
type DashboardConfig struct {
	Addr string `yaml:"addr" json:"addr"`
}

// PoolConfig holds the sandbox pool configuration.
type PoolConfig struct {
	MinReady int `yaml:"minReady" json:"minReady"`
	MaxSize  int `yaml:"maxSize" json:"maxSize"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		DefaultRuntime: "docker",
		LogLevel:       "info",
		Server:         ServerConfig{Addr: ":8080"},
		Dashboard:      DashboardConfig{Addr: ":9090"},
		Pool:           PoolConfig{MinReady: 2, MaxSize: 5},
	}
}

// Dir returns the configuration directory path.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, DefaultConfigDir), nil
}

// FilePath returns the full path to the config file.
func FilePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DefaultConfigFile), nil
}

// EnsureDir creates the configuration directory if it doesn't exist.
func EnsureDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}
	return dir, nil
}

// Load reads the configuration from the default config file path.
// If the file does not exist, it returns the default configuration.
func Load() (*Config, error) {
	path, err := FilePath()
	if err != nil {
		return DefaultConfig(), nil
	}
	return LoadFromPath(path)
}

// LoadFromPath reads the configuration from the specified file path.
// If the file does not exist, it returns the default configuration.
func LoadFromPath(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// Save writes the configuration to the default config file.
func Save(cfg *Config) error {
	dir, err := EnsureDir()
	if err != nil {
		return err
	}
	return SaveToPath(cfg, filepath.Join(dir, DefaultConfigFile))
}

// SaveToPath writes the configuration to the specified file path.
func SaveToPath(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}
