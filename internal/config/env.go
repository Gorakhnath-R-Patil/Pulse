package config

import "os"

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
)

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
}
