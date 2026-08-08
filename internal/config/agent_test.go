package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
)

func TestLoadAgentConfig_DefaultsWhenNoPath(t *testing.T) {
	cfg, err := config.LoadAgentConfig("")
	if err != nil {
		t.Fatalf("LoadAgentConfig(\"\") returned error: %v", err)
	}
	if cfg.NodeName == "" {
		t.Error("expected a non-empty default NodeName")
	}
	if cfg.Logging != config.DefaultLoggingConfig() {
		t.Errorf("Logging = %+v, want defaults %+v", cfg.Logging, config.DefaultLoggingConfig())
	}
}

func TestLoadAgentConfig_FromValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	yaml := "node_name: pulse-node-42\nlogging:\n  level: debug\n  format: text\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := config.LoadAgentConfig(path)
	if err != nil {
		t.Fatalf("LoadAgentConfig(%q) returned error: %v", path, err)
	}
	if cfg.NodeName != "pulse-node-42" {
		t.Errorf("NodeName = %q, want %q", cfg.NodeName, "pulse-node-42")
	}
	if cfg.Logging.Level != "debug" || cfg.Logging.Format != "text" {
		t.Errorf("Logging = %+v, want {debug text}", cfg.Logging)
	}
}

func TestLoadAgentConfig_MissingFileIsError(t *testing.T) {
	_, err := config.LoadAgentConfig(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if !errors.Is(err, config.ErrNotFound) {
		t.Fatalf("LoadAgentConfig() error = %v, want it to wrap ErrNotFound", err)
	}
}

func TestLoadAgentConfig_InvalidYAMLIsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte("node_name: [this is not valid: yaml"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := config.LoadAgentConfig(path)
	if !errors.Is(err, config.ErrInvalidSyntax) {
		t.Fatalf("LoadAgentConfig() error = %v, want it to wrap ErrInvalidSyntax", err)
	}
}

func TestLoadAgentConfig_UnknownFieldIsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte("node_nmae: typo\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := config.LoadAgentConfig(path)
	if !errors.Is(err, config.ErrInvalidSyntax) {
		t.Fatalf("LoadAgentConfig() error = %v, want it to wrap ErrInvalidSyntax (strict decoding should catch the typo)", err)
	}
}

func TestLoadAgentConfig_ValidationFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte("node_name: n1\nlogging:\n  level: loud\n  format: json\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := config.LoadAgentConfig(path)
	if !errors.Is(err, config.ErrInvalidValue) {
		t.Fatalf("LoadAgentConfig() error = %v, want it to wrap ErrInvalidValue", err)
	}
}

func TestLoadAgentConfig_EnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte("node_name: n1\nlogging:\n  level: info\n  format: json\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("PULSE_LOG_LEVEL", "debug")
	t.Setenv("PULSE_LOG_FORMAT", "text")

	cfg, err := config.LoadAgentConfig(path)
	if err != nil {
		t.Fatalf("LoadAgentConfig(%q) returned error: %v", path, err)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Logging.Level = %q, want env override %q", cfg.Logging.Level, "debug")
	}
	if cfg.Logging.Format != "text" {
		t.Errorf("Logging.Format = %q, want env override %q", cfg.Logging.Format, "text")
	}
}
