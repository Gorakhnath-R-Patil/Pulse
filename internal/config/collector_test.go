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

func TestLoadCollectorConfig_ClickHouseDefaultsDisabled(t *testing.T) {
	cfg, err := config.LoadCollectorConfig("")
	if err != nil {
		t.Fatalf("LoadCollectorConfig(\"\") returned error: %v", err)
	}
	if len(cfg.ClickHouseAddr) != 0 {
		t.Errorf("ClickHouseAddr = %v, want empty by default (storage disabled)", cfg.ClickHouseAddr)
	}
}

func TestLoadCollectorConfig_ClickHouseEnvOverride(t *testing.T) {
	t.Setenv("PULSE_CLICKHOUSE_ADDR", "localhost:9000, localhost:9001")
	t.Setenv("PULSE_CLICKHOUSE_USERNAME", "pulse")
	t.Setenv("PULSE_CLICKHOUSE_PASSWORD", "secret")

	cfg, err := config.LoadCollectorConfig("")
	if err != nil {
		t.Fatalf("LoadCollectorConfig(\"\") returned error: %v", err)
	}
	wantAddr := []string{"localhost:9000", "localhost:9001"}
	if len(cfg.ClickHouseAddr) != len(wantAddr) || cfg.ClickHouseAddr[0] != wantAddr[0] || cfg.ClickHouseAddr[1] != wantAddr[1] {
		t.Errorf("ClickHouseAddr = %v, want %v (whitespace trimmed)", cfg.ClickHouseAddr, wantAddr)
	}
	if cfg.ClickHouseUsername != "pulse" {
		t.Errorf("ClickHouseUsername = %q, want %q", cfg.ClickHouseUsername, "pulse")
	}
	if cfg.ClickHousePassword != "secret" {
		t.Errorf("ClickHousePassword = %q, want %q", cfg.ClickHousePassword, "secret")
	}
}

func TestLoadCollectorConfig_ClickHouseDatabaseAndTableDefaultWhenAddrSet(t *testing.T) {
	t.Setenv("PULSE_CLICKHOUSE_ADDR", "localhost:9000")

	cfg, err := config.LoadCollectorConfig("")
	if err != nil {
		t.Fatalf("LoadCollectorConfig(\"\") returned error: %v", err)
	}
	if cfg.ClickHouseDatabase != "pulse" {
		t.Errorf("ClickHouseDatabase = %q, want default %q", cfg.ClickHouseDatabase, "pulse")
	}
	if cfg.ClickHouseTable != "events" {
		t.Errorf("ClickHouseTable = %q, want default %q", cfg.ClickHouseTable, "events")
	}
}

func TestLoadCollectorConfig_ClickHouseDatabaseAndTableEnvOverride(t *testing.T) {
	t.Setenv("PULSE_CLICKHOUSE_ADDR", "localhost:9000")
	t.Setenv("PULSE_CLICKHOUSE_DATABASE", "custom_db")
	t.Setenv("PULSE_CLICKHOUSE_TABLE", "custom_events")

	cfg, err := config.LoadCollectorConfig("")
	if err != nil {
		t.Fatalf("LoadCollectorConfig(\"\") returned error: %v", err)
	}
	if cfg.ClickHouseDatabase != "custom_db" {
		t.Errorf("ClickHouseDatabase = %q, want env override %q", cfg.ClickHouseDatabase, "custom_db")
	}
	if cfg.ClickHouseTable != "custom_events" {
		t.Errorf("ClickHouseTable = %q, want env override %q", cfg.ClickHouseTable, "custom_events")
	}
}

func TestLoadCollectorConfig_MetricsAddrDefaultsEmpty(t *testing.T) {
	cfg, err := config.LoadCollectorConfig("")
	if err != nil {
		t.Fatalf("LoadCollectorConfig(\"\") returned error: %v", err)
	}
	if cfg.MetricsAddr != "" {
		t.Errorf("MetricsAddr = %q, want empty by default (metrics server disabled)", cfg.MetricsAddr)
	}
}

func TestLoadCollectorConfig_MetricsAddrEnvOverride(t *testing.T) {
	t.Setenv("PULSE_METRICS_ADDR", ":9091")

	cfg, err := config.LoadCollectorConfig("")
	if err != nil {
		t.Fatalf("LoadCollectorConfig(\"\") returned error: %v", err)
	}
	if cfg.MetricsAddr != ":9091" {
		t.Errorf("MetricsAddr = %q, want env override %q", cfg.MetricsAddr, ":9091")
	}
}
