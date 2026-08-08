package config

import (
	"fmt"
	"os"
)

// AgentConfig is the top-level configuration for pulse-agent, the
// per-host process that will eventually own eBPF-based observation
// (introduced starting Day 03). Day 01 only defines the identity and
// logging fields the agent needs to start up and log meaningfully.
type AgentConfig struct {
	// NodeName identifies the host this agent runs on. It is attached to
	// every event the agent produces once event capture exists, so
	// operators can trace telemetry back to its source node.
	NodeName string `yaml:"node_name"`

	Logging LoggingConfig `yaml:"logging"`
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

	if err := cfg.Validate(); err != nil {
		return AgentConfig{}, err
	}
	return cfg, nil
}
