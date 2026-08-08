package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
)

func TestLoadCollectorConfig_DefaultsWhenNoPath(t *testing.T) {
	cfg, err := config.LoadCollectorConfig("")
	if err != nil {
		t.Fatalf("LoadCollectorConfig(\"\") returned error: %v", err)
	}
	if cfg.Logging != config.DefaultLoggingConfig() {
		t.Errorf("Logging = %+v, want defaults %+v", cfg.Logging, config.DefaultLoggingConfig())
	}
}

func TestLoadCollectorConfig_FromValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "collector.yaml")
	if err := os.WriteFile(path, []byte("logging:\n  level: warn\n  format: json\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := config.LoadCollectorConfig(path)
	if err != nil {
		t.Fatalf("LoadCollectorConfig(%q) returned error: %v", path, err)
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("Logging.Level = %q, want %q", cfg.Logging.Level, "warn")
	}
}

func TestLoadCollectorConfig_MissingFileIsError(t *testing.T) {
	_, err := config.LoadCollectorConfig(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if !errors.Is(err, config.ErrNotFound) {
		t.Fatalf("LoadCollectorConfig() error = %v, want it to wrap ErrNotFound", err)
	}
}

func TestLoadCollectorConfig_ValidationFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "collector.yaml")
	if err := os.WriteFile(path, []byte("logging:\n  level: info\n  format: yaml\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := config.LoadCollectorConfig(path)
	if !errors.Is(err, config.ErrInvalidValue) {
		t.Fatalf("LoadCollectorConfig() error = %v, want it to wrap ErrInvalidValue", err)
	}
}
