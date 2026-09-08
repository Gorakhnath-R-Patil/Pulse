package config

import (
	"fmt"
	"os"
)

// AgentConfig is the top-level configuration for pulse-agent, the
// per-host process that owns eBPF-based observation.
type AgentConfig struct {
	// NodeName identifies the host this agent runs on. It is attached to
	// every event the agent produces once event capture exists, so
	// operators can trace telemetry back to its source node.
	NodeName string `yaml:"node_name"`

	Logging LoggingConfig `yaml:"logging"`

	// OTLPEndpoint is an OTLP/gRPC collector address (e.g.
	// "localhost:4317") to export correlated spans to. Empty (the
	// default) disables export entirely — pulse-agent never tries to
	// reach a made-up default endpoint. See docs/design/otlp-export.md.
	OTLPEndpoint string `yaml:"otlp_endpoint,omitempty"`

	// KafkaBrokers is a list of Kafka bootstrap addresses (e.g.
	// ["localhost:9092"]) to produce every captured event to, in
	// addition to — not instead of — each pipeline's existing logging
	// and correlation steps. Empty (the default) disables Kafka
	// production entirely. See docs/design/kafka-transport.md.
	KafkaBrokers []string `yaml:"kafka_brokers,omitempty"`

	// KafkaTopic is the topic events are produced to. Required if
	// KafkaBrokers is set; ignored otherwise.
	KafkaTopic string `yaml:"kafka_topic,omitempty"`
}

// DefaultAgentConfig returns the configuration used when no file is
// supplied. NodeName falls back to the OS hostname, then "unknown" if the
// hostname cannot be determined.
func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		NodeName: defaultNodeName(),
		Logging:  DefaultLoggingConfig(),
	}
}

func defaultNodeName() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "unknown"
}

// Validate checks that the configuration is semantically usable.
func (c AgentConfig) Validate() error {
	if c.NodeName == "" {
		return fmt.Errorf("%w: node_name must not be empty", ErrInvalidValue)
	}
	if err := c.Logging.Validate(); err != nil {
		return err
	}
	if len(c.KafkaBrokers) > 0 && c.KafkaTopic == "" {
		return fmt.Errorf("%w: kafka_topic must be set when kafka_brokers is set", ErrInvalidValue)
	}
	if len(c.KafkaBrokers) == 0 && c.KafkaTopic != "" {
		return fmt.Errorf("%w: kafka_topic is set but kafka_brokers is empty", ErrInvalidValue)
	}
	return nil
}

// LoadAgentConfig builds an AgentConfig from defaults, an optional YAML
// file, and environment overrides, in that order of precedence.
//
// path may be empty, meaning "no file requested" — defaults and env
// overrides still apply. If path is non-empty and does not exist, that is
// treated as a configuration error (ErrNotFound) rather than silently
// falling back, since an operator who names a file expects it to be read.
func LoadAgentConfig(path string) (AgentConfig, error) {
	cfg := DefaultAgentConfig()

	if path != "" {
		if err := loadYAMLFile(path, &cfg); err != nil {
			return AgentConfig{}, err
		}
	}

	applyLoggingEnvOverrides(&cfg.Logging)
	applyAgentEnvOverrides(&cfg)

	if err := cfg.Validate(); err != nil {
		return AgentConfig{}, err
	}
	return cfg, nil
}
