package config

// CollectorConfig is the top-level configuration for pulse-collector.
// Day 01 gives it only what it needs to start up and log: the collector's
// ingestion and storage responsibilities don't exist yet (Kafka consumption
// arrives Day 14, ClickHouse storage Day 15), so this schema intentionally
// carries no fields for them yet.
type CollectorConfig struct {
	Logging LoggingConfig `yaml:"logging"`
}

// DefaultCollectorConfig returns the configuration used when no file is
// supplied.
func DefaultCollectorConfig() CollectorConfig {
	return CollectorConfig{
		Logging: DefaultLoggingConfig(),
	}
}

// Validate checks that the configuration is semantically usable.
func (c CollectorConfig) Validate() error {
	return c.Logging.Validate()
}

// LoadCollectorConfig builds a CollectorConfig from defaults, an optional
// YAML file, and environment overrides, in that order of precedence. See
// LoadAgentConfig for the precedence and missing-file semantics.
func LoadCollectorConfig(path string) (CollectorConfig, error) {
	cfg := DefaultCollectorConfig()

	if path != "" {
		if err := loadYAMLFile(path, &cfg); err != nil {
			return CollectorConfig{}, err
		}
	}

	applyLoggingEnvOverrides(&cfg.Logging)

	if err := cfg.Validate(); err != nil {
		return CollectorConfig{}, err
	}
	return cfg, nil
}
