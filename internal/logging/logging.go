// Package logging builds structured loggers for Pulse components.
//
// Pulse uses the standard library's log/slog rather than a third-party
// logging library: it is dependency-free, structured by default, and fast
// enough for a telemetry system that will itself emit high volumes of log
// output. There is deliberately no package-level global logger — every
// component constructs its own *slog.Logger and receives it via
// dependency injection, so tests can capture output and nothing in this
// codebase depends on hidden global state.
package logging

import (
	"fmt"
	"io"
	"log/slog"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
)

// New builds a *slog.Logger from cfg, writing to w. It returns an error if
// cfg fails validation rather than silently falling back to a default,
// since a misconfigured logger should never be the reason misconfiguration
// goes unnoticed.
func New(cfg config.LoggingConfig, w io.Writer) (*slog.Logger, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	level, err := parseLevel(cfg.Level)
	if err != nil {
		// Validate() above already rejects unknown levels, so this
		// path only triggers if the two checks ever drift apart.
		return nil, err
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	switch cfg.Format {
	case "text":
		handler = slog.NewTextHandler(w, opts)
	default: // "json", enforced by cfg.Validate above
		handler = slog.NewJSONHandler(w, opts)
	}

	return slog.New(handler), nil
}

func parseLevel(level string) (slog.Level, error) {
	switch level {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("logging: unrecognized level %q", level)
	}
}
