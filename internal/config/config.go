// Package config manages sandboxMatrix configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// DefaultConfigDir is the default configuration directory name.
	DefaultConfigDir = ".sandboxmatrix"
	// DefaultConfigFile is the default configuration file name.
	DefaultConfigFile = "config.yaml"
)

// Config holds the sandboxMatrix configuration.
type Config struct {
	DefaultRuntime string `yaml:"defaultRuntime" json:"defaultRuntime"`
	BlueprintDir   string `yaml:"blueprintDir" json:"blueprintDir"`
	StateDir       string `yaml:"stateDir" json:"stateDir"`
	LogLevel       string `yaml:"logLevel" json:"logLevel"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		DefaultRuntime: "docker",
		LogLevel:       "info",
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
