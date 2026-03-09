package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DefaultRuntime != "docker" {
		t.Errorf("DefaultRuntime = %q, want %q", cfg.DefaultRuntime, "docker")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.Server.Addr != ":8080" {
		t.Errorf("Server.Addr = %q, want %q", cfg.Server.Addr, ":8080")
	}
	if cfg.Dashboard.Addr != ":9090" {
		t.Errorf("Dashboard.Addr = %q, want %q", cfg.Dashboard.Addr, ":9090")
	}
	if cfg.Pool.MinReady != 2 {
		t.Errorf("Pool.MinReady = %d, want %d", cfg.Pool.MinReady, 2)
	}
	if cfg.Pool.MaxSize != 5 {
		t.Errorf("Pool.MaxSize = %d, want %d", cfg.Pool.MaxSize, 5)
	}
	if cfg.BlueprintDir != "" {
		t.Errorf("BlueprintDir = %q, want empty", cfg.BlueprintDir)
	}
	if cfg.StateDir != "" {
		t.Errorf("StateDir = %q, want empty", cfg.StateDir)
	}
}

func TestLoadFromPathNonExistent(t *testing.T) {
	cfg, err := LoadFromPath("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("LoadFromPath returned error: %v", err)
	}
	// Should return defaults when file doesn't exist.
	def := DefaultConfig()
	if cfg.DefaultRuntime != def.DefaultRuntime {
		t.Errorf("DefaultRuntime = %q, want default %q", cfg.DefaultRuntime, def.DefaultRuntime)
	}
	if cfg.LogLevel != def.LogLevel {
		t.Errorf("LogLevel = %q, want default %q", cfg.LogLevel, def.LogLevel)
	}
	if cfg.Server.Addr != def.Server.Addr {
		t.Errorf("Server.Addr = %q, want default %q", cfg.Server.Addr, def.Server.Addr)
	}
}

func TestSaveToPathAndLoadFromPath(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")

	cfg := &Config{
		DefaultRuntime: "gvisor",
		LogLevel:       "debug",
		BlueprintDir:   "/tmp/blueprints",
		StateDir:       "/tmp/state",
		Server:         ServerConfig{Addr: ":9999"},
		Dashboard:      DashboardConfig{Addr: ":7777"},
		Pool:           PoolConfig{MinReady: 5, MaxSize: 20},
	}

	if err := SaveToPath(cfg, path); err != nil {
		t.Fatalf("SaveToPath failed: %v", err)
	}

	// Verify the file was created.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}

	loaded, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath failed: %v", err)
	}

	if loaded.DefaultRuntime != "gvisor" {
		t.Errorf("DefaultRuntime = %q, want %q", loaded.DefaultRuntime, "gvisor")
	}
	if loaded.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", loaded.LogLevel, "debug")
	}
	if loaded.BlueprintDir != "/tmp/blueprints" {
		t.Errorf("BlueprintDir = %q, want %q", loaded.BlueprintDir, "/tmp/blueprints")
	}
	if loaded.StateDir != "/tmp/state" {
		t.Errorf("StateDir = %q, want %q", loaded.StateDir, "/tmp/state")
	}
	if loaded.Server.Addr != ":9999" {
		t.Errorf("Server.Addr = %q, want %q", loaded.Server.Addr, ":9999")
	}
	if loaded.Dashboard.Addr != ":7777" {
		t.Errorf("Dashboard.Addr = %q, want %q", loaded.Dashboard.Addr, ":7777")
	}
	if loaded.Pool.MinReady != 5 {
		t.Errorf("Pool.MinReady = %d, want %d", loaded.Pool.MinReady, 5)
	}
	if loaded.Pool.MaxSize != 20 {
		t.Errorf("Pool.MaxSize = %d, want %d", loaded.Pool.MaxSize, 20)
	}
}

func TestLoadFromPathInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")

	// Write invalid YAML content.
	if err := os.WriteFile(path, []byte("{{invalid yaml:::"), 0o644); err != nil {
		t.Fatalf("failed to write invalid yaml: %v", err)
	}

	_, err := LoadFromPath(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoadFromPathPartialConfig(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")

	// Write a partial config; defaults should fill in the rest.
	partial := []byte("logLevel: warn\n")
	if err := os.WriteFile(path, partial, 0o644); err != nil {
		t.Fatalf("failed to write partial config: %v", err)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath failed: %v", err)
	}

	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "warn")
	}
	// Other fields should retain defaults.
	if cfg.DefaultRuntime != "docker" {
		t.Errorf("DefaultRuntime = %q, want default %q", cfg.DefaultRuntime, "docker")
	}
	if cfg.Server.Addr != ":8080" {
		t.Errorf("Server.Addr = %q, want default %q", cfg.Server.Addr, ":8080")
	}
}

func TestDir(t *testing.T) {
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() returned error: %v", err)
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("Dir() returned non-absolute path: %s", dir)
	}
	if filepath.Base(dir) != DefaultConfigDir {
		t.Errorf("Dir() base = %q, want %q", filepath.Base(dir), DefaultConfigDir)
	}
}

func TestFilePath(t *testing.T) {
	path, err := FilePath()
	if err != nil {
		t.Fatalf("FilePath() returned error: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("FilePath() returned non-absolute path: %s", path)
	}
	if filepath.Base(path) != DefaultConfigFile {
		t.Errorf("FilePath() base = %q, want %q", filepath.Base(path), DefaultConfigFile)
	}
}

func TestEnsureDir(t *testing.T) {
	dir, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir() returned error: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory %s does not exist: %v", dir, err)
	}
	if !info.IsDir() {
		t.Errorf("%s is not a directory", dir)
	}
}
