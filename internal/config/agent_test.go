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

func TestLoadAgentConfig_OTLPEndpointDefaultsEmpty(t *testing.T) {
	cfg, err := config.LoadAgentConfig("")
	if err != nil {
		t.Fatalf("LoadAgentConfig(\"\") returned error: %v", err)
	}
	if cfg.OTLPEndpoint != "" {
		t.Errorf("OTLPEndpoint = %q, want empty by default (export disabled)", cfg.OTLPEndpoint)
	}
}

func TestLoadAgentConfig_OTLPEndpointEnvOverride(t *testing.T) {
	t.Setenv("PULSE_OTLP_ENDPOINT", "localhost:4317")

	cfg, err := config.LoadAgentConfig("")
	if err != nil {
		t.Fatalf("LoadAgentConfig(\"\") returned error: %v", err)
	}
	if cfg.OTLPEndpoint != "localhost:4317" {
		t.Errorf("OTLPEndpoint = %q, want env override %q", cfg.OTLPEndpoint, "localhost:4317")
	}
}

func TestLoadAgentConfig_KafkaDefaultsDisabled(t *testing.T) {
	cfg, err := config.LoadAgentConfig("")
	if err != nil {
		t.Fatalf("LoadAgentConfig(\"\") returned error: %v", err)
	}
	if len(cfg.KafkaBrokers) != 0 {
		t.Errorf("KafkaBrokers = %v, want empty by default (production disabled)", cfg.KafkaBrokers)
	}
}

func TestLoadAgentConfig_KafkaEnvOverride(t *testing.T) {
	t.Setenv("PULSE_KAFKA_BROKERS", "localhost:9092, localhost:9093")
	t.Setenv("PULSE_KAFKA_TOPIC", "pulse-events")

	cfg, err := config.LoadAgentConfig("")
	if err != nil {
		t.Fatalf("LoadAgentConfig(\"\") returned error: %v", err)
	}
	wantBrokers := []string{"localhost:9092", "localhost:9093"}
	if len(cfg.KafkaBrokers) != len(wantBrokers) || cfg.KafkaBrokers[0] != wantBrokers[0] || cfg.KafkaBrokers[1] != wantBrokers[1] {
		t.Errorf("KafkaBrokers = %v, want %v (whitespace trimmed)", cfg.KafkaBrokers, wantBrokers)
	}
	if cfg.KafkaTopic != "pulse-events" {
		t.Errorf("KafkaTopic = %q, want %q", cfg.KafkaTopic, "pulse-events")
	}
}

func TestLoadAgentConfig_KafkaTopicWithoutBrokersIsError(t *testing.T) {
	t.Setenv("PULSE_KAFKA_TOPIC", "pulse-events")

	_, err := config.LoadAgentConfig("")
	if !errors.Is(err, config.ErrInvalidValue) {
		t.Fatalf("LoadAgentConfig() error = %v, want it to wrap ErrInvalidValue (kafka_topic without kafka_brokers)", err)
	}
}

func TestLoadAgentConfig_KafkaBrokersWithoutTopicIsError(t *testing.T) {
	t.Setenv("PULSE_KAFKA_BROKERS", "localhost:9092")

	_, err := config.LoadAgentConfig("")
	if !errors.Is(err, config.ErrInvalidValue) {
		t.Fatalf("LoadAgentConfig() error = %v, want it to wrap ErrInvalidValue (kafka_brokers without kafka_topic)", err)
	}
}

func TestLoadAgentConfig_MetricsAddrDefaultsEmpty(t *testing.T) {
	cfg, err := config.LoadAgentConfig("")
	if err != nil {
		t.Fatalf("LoadAgentConfig(\"\") returned error: %v", err)
	}
	if cfg.MetricsAddr != "" {
		t.Errorf("MetricsAddr = %q, want empty by default (metrics server disabled)", cfg.MetricsAddr)
	}
}

func TestLoadAgentConfig_MetricsAddrEnvOverride(t *testing.T) {
	t.Setenv("PULSE_METRICS_ADDR", ":9090")

	cfg, err := config.LoadAgentConfig("")
	if err != nil {
		t.Fatalf("LoadAgentConfig(\"\") returned error: %v", err)
	}
	if cfg.MetricsAddr != ":9090" {
		t.Errorf("MetricsAddr = %q, want env override %q", cfg.MetricsAddr, ":9090")
	}
}
