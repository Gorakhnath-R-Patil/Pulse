package config

import "fmt"

// LoggingConfig controls the structured logger shared by every Pulse
// component. It is embedded in each binary's top-level configuration
// instead of duplicated, since logging behavior should be configured
// consistently across the agent, collector, and CLI.
type LoggingConfig struct {
	// Level is the minimum severity that will be emitted: one of
	// "debug", "info", "warn", "error".
	Level string `yaml:"level"`

	// Format selects the log encoding: "json" for machine-readable
	// output (the production default) or "text" for human-readable
	// output during local development.
	Format string `yaml:"format"`
}

// DefaultLoggingConfig returns the secure, production-appropriate default:
// info level, JSON encoding.
func DefaultLoggingConfig() LoggingConfig {
	return LoggingConfig{
		Level:  "info",
		Format: "json",
	}
}

var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

var validLogFormats = map[string]bool{
	"json": true,
	"text": true,
}

// Validate reports whether the configuration holds a recognized level and
// format. It returns an error wrapping ErrInvalidValue on failure.
func (c LoggingConfig) Validate() error {
	if !validLogLevels[c.Level] {
		return fmt.Errorf("%w: logging.level %q (want one of debug, info, warn, error)", ErrInvalidValue, c.Level)
	}
	if !validLogFormats[c.Format] {
		return fmt.Errorf("%w: logging.format %q (want one of json, text)", ErrInvalidValue, c.Format)
	}
	return nil
}
