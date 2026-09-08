package config

import "fmt"

// CollectorConfig is the top-level configuration for pulse-collector.
// Day 01 gave it only what it needs to start up and log; Day 14 adds
// Kafka consumption. Storage (ClickHouse, Day 15) still doesn't exist,
// so a consumed event today is only logged — see
// docs/design/kafka-transport.md.
type CollectorConfig struct {
	Logging LoggingConfig `yaml:"logging"`

	// KafkaBrokers is a list of Kafka bootstrap addresses (e.g.
	// ["localhost:9092"]) to consume events from. Empty (the default)
	// disables Kafka consumption entirely.
	KafkaBrokers []string `yaml:"kafka_brokers,omitempty"`

	// KafkaTopic is the topic to consume from. Required if
	// KafkaBrokers is set; ignored otherwise.
	KafkaTopic string `yaml:"kafka_topic,omitempty"`

	// KafkaGroupID is the Kafka consumer group this collector joins.
	// Defaults to "pulse-collector" if KafkaBrokers is set and this is
	// left empty, so a single-collector deployment works without
	// naming one explicitly.
	KafkaGroupID string `yaml:"kafka_group_id,omitempty"`
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

// defaultKafkaGroupID is used when KafkaBrokers is set but KafkaGroupID
// is left empty, so a single-collector deployment works without naming
// a consumer group explicitly.
const defaultKafkaGroupID = "pulse-collector"

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
	applyCollectorEnvOverrides(&cfg)

	if len(cfg.KafkaBrokers) > 0 && cfg.KafkaGroupID == "" {
		cfg.KafkaGroupID = defaultKafkaGroupID
	}

	if err := cfg.Validate(); err != nil {
		return CollectorConfig{}, err
	}
	return cfg, nil
}
