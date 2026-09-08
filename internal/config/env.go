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
)

// splitKafkaBrokers parses a comma-separated PULSE_KAFKA_BROKERS value
// into a broker list, trimming whitespace around each entry and
// dropping empty ones (so a trailing comma or extra spaces don't
// produce a spurious empty broker address).
func splitKafkaBrokers(v string) []string {
	var brokers []string
	for _, b := range strings.Split(v, ",") {
		if b := strings.TrimSpace(b); b != "" {
			brokers = append(brokers, b)
		}
	}
	return brokers
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
		cfg.KafkaBrokers = splitKafkaBrokers(v)
	}
	if v := os.Getenv(envKafkaTopic); v != "" {
		cfg.KafkaTopic = v
	}
}

// applyCollectorEnvOverrides mutates cfg in place with any PULSE_*
// variables specific to pulse-collector.
func applyCollectorEnvOverrides(cfg *CollectorConfig) {
	if v := os.Getenv(envKafkaBrokers); v != "" {
		cfg.KafkaBrokers = splitKafkaBrokers(v)
	}
	if v := os.Getenv(envKafkaTopic); v != "" {
		cfg.KafkaTopic = v
	}
	if v := os.Getenv(envKafkaGroupID); v != "" {
		cfg.KafkaGroupID = v
	}
}
