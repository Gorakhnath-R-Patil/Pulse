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

func TestLoadCollectorConfig_KafkaDefaultsDisabled(t *testing.T) {
	cfg, err := config.LoadCollectorConfig("")
	if err != nil {
		t.Fatalf("LoadCollectorConfig(\"\") returned error: %v", err)
	}
	if len(cfg.KafkaBrokers) != 0 {
		t.Errorf("KafkaBrokers = %v, want empty by default (consumption disabled)", cfg.KafkaBrokers)
	}
}

func TestLoadCollectorConfig_KafkaEnvOverride(t *testing.T) {
	t.Setenv("PULSE_KAFKA_BROKERS", "localhost:9092")
	t.Setenv("PULSE_KAFKA_TOPIC", "pulse-events")

	cfg, err := config.LoadCollectorConfig("")
	if err != nil {
		t.Fatalf("LoadCollectorConfig(\"\") returned error: %v", err)
	}
	if len(cfg.KafkaBrokers) != 1 || cfg.KafkaBrokers[0] != "localhost:9092" {
		t.Errorf("KafkaBrokers = %v, want [localhost:9092]", cfg.KafkaBrokers)
	}
	if cfg.KafkaTopic != "pulse-events" {
		t.Errorf("KafkaTopic = %q, want %q", cfg.KafkaTopic, "pulse-events")
	}
}

func TestLoadCollectorConfig_KafkaGroupIDDefaultsWhenBrokersSet(t *testing.T) {
	t.Setenv("PULSE_KAFKA_BROKERS", "localhost:9092")
	t.Setenv("PULSE_KAFKA_TOPIC", "pulse-events")

	cfg, err := config.LoadCollectorConfig("")
	if err != nil {
		t.Fatalf("LoadCollectorConfig(\"\") returned error: %v", err)
	}
	if cfg.KafkaGroupID != "pulse-collector" {
		t.Errorf("KafkaGroupID = %q, want default %q", cfg.KafkaGroupID, "pulse-collector")
	}
}

func TestLoadCollectorConfig_KafkaGroupIDEnvOverride(t *testing.T) {
	t.Setenv("PULSE_KAFKA_BROKERS", "localhost:9092")
	t.Setenv("PULSE_KAFKA_TOPIC", "pulse-events")
	t.Setenv("PULSE_KAFKA_GROUP_ID", "custom-group")

	cfg, err := config.LoadCollectorConfig("")
	if err != nil {
		t.Fatalf("LoadCollectorConfig(\"\") returned error: %v", err)
	}
	if cfg.KafkaGroupID != "custom-group" {
		t.Errorf("KafkaGroupID = %q, want env override %q", cfg.KafkaGroupID, "custom-group")
	}
}

func TestLoadCollectorConfig_KafkaTopicWithoutBrokersIsError(t *testing.T) {
	t.Setenv("PULSE_KAFKA_TOPIC", "pulse-events")

	_, err := config.LoadCollectorConfig("")
	if !errors.Is(err, config.ErrInvalidValue) {
		t.Fatalf("LoadCollectorConfig() error = %v, want it to wrap ErrInvalidValue (kafka_topic without kafka_brokers)", err)
	}
}
