package config

import (
	"os"
	"strings"
)

// Environment variable names for the small set of settings every Pulse
// component shares. These take precedence over file-based configuration,
// matching the common container/orchestration pattern of overriding a
// mounted config file with env vars at deploy time.
const (
	envLogLevel  = "PULSE_LOG_LEVEL"
	envLogFormat = "PULSE_LOG_FORMAT"

	// envOTLPEndpoint is pulse-agent-specific, unlike the PULSE_LOG_*
	// variables above, which every component honors — see
	// applyAgentEnvOverrides.
	envOTLPEndpoint = "PULSE_OTLP_ENDPOINT"

	// envKafkaBrokers and envKafkaTopic are shared by pulse-agent
	// (producer) and pulse-collector (consumer) — see
	// applyAgentEnvOverrides and applyCollectorEnvOverrides.
	// envKafkaBrokers is a comma-separated list, matching how a
	// deploy-time env override typically has to represent a list in a
	// single string.
	envKafkaBrokers = "PULSE_KAFKA_BROKERS"
	envKafkaTopic   = "PULSE_KAFKA_TOPIC"

	// envKafkaGroupID is pulse-collector-specific: pulse-agent produces
	// and has no consumer group of its own.
	envKafkaGroupID = "PULSE_KAFKA_GROUP_ID"

	// envClickHouseAddr, envClickHouseDatabase, envClickHouseUsername,
	// and envClickHousePassword are pulse-collector-specific — see
	// applyCollectorEnvOverrides. envClickHouseAddr is comma-separated,
	// the same convention envKafkaBrokers uses.
	envClickHouseAddr     = "PULSE_CLICKHOUSE_ADDR"
	envClickHouseDatabase = "PULSE_CLICKHOUSE_DATABASE"
	envClickHouseUsername = "PULSE_CLICKHOUSE_USERNAME"
	envClickHousePassword = "PULSE_CLICKHOUSE_PASSWORD"
	envClickHouseTable    = "PULSE_CLICKHOUSE_TABLE"

	// envMetricsAddr is shared by both binaries — each has its own
	// MetricsAddr field — see applyAgentEnvOverrides and
	// applyCollectorEnvOverrides.
	envMetricsAddr = "PULSE_METRICS_ADDR"
)

// splitAddrList parses a comma-separated env var value (a broker or
// server address list) into a slice, trimming whitespace around each
// entry and dropping empty ones (so a trailing comma or extra spaces
// don't produce a spurious empty address). Shared by
// PULSE_KAFKA_BROKERS and PULSE_CLICKHOUSE_ADDR.
func splitAddrList(v string) []string {
	var addrs []string
	for _, a := range strings.Split(v, ",") {
		if a := strings.TrimSpace(a); a != "" {
			addrs = append(addrs, a)
		}
	}
	return addrs
}

// applyLoggingEnvOverrides mutates cfg in place with any of the
// PULSE_LOG_* environment variables that are set.
func applyLoggingEnvOverrides(cfg *LoggingConfig) {
	if v := os.Getenv(envLogLevel); v != "" {
		cfg.Level = v
	}
	if v := os.Getenv(envLogFormat); v != "" {
		cfg.Format = v
	}
}

// applyAgentEnvOverrides mutates cfg in place with any PULSE_* variables
// specific to pulse-agent (as opposed to the shared PULSE_LOG_*
// variables applyLoggingEnvOverrides already applies to cfg.Logging).
func applyAgentEnvOverrides(cfg *AgentConfig) {
	if v := os.Getenv(envOTLPEndpoint); v != "" {
		cfg.OTLPEndpoint = v
	}
	if v := os.Getenv(envKafkaBrokers); v != "" {
		cfg.KafkaBrokers = splitAddrList(v)
	}
	if v := os.Getenv(envKafkaTopic); v != "" {
		cfg.KafkaTopic = v
	}
	if v := os.Getenv(envMetricsAddr); v != "" {
		cfg.MetricsAddr = v
	}
}

// applyCollectorEnvOverrides mutates cfg in place with any PULSE_*
// variables specific to pulse-collector.
func applyCollectorEnvOverrides(cfg *CollectorConfig) {
	if v := os.Getenv(envKafkaBrokers); v != "" {
		cfg.KafkaBrokers = splitAddrList(v)
	}
	if v := os.Getenv(envKafkaTopic); v != "" {
		cfg.KafkaTopic = v
	}
	if v := os.Getenv(envKafkaGroupID); v != "" {
		cfg.KafkaGroupID = v
	}
	if v := os.Getenv(envClickHouseAddr); v != "" {
		cfg.ClickHouseAddr = splitAddrList(v)
	}
	if v := os.Getenv(envClickHouseDatabase); v != "" {
		cfg.ClickHouseDatabase = v
	}
	if v := os.Getenv(envClickHouseUsername); v != "" {
		cfg.ClickHouseUsername = v
	}
	if v := os.Getenv(envClickHousePassword); v != "" {
		cfg.ClickHousePassword = v
	}
	if v := os.Getenv(envClickHouseTable); v != "" {
		cfg.ClickHouseTable = v
	}
	if v := os.Getenv(envMetricsAddr); v != "" {
		cfg.MetricsAddr = v
	}
}
