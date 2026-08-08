// Package collector contains the pulse-collector application skeleton:
// startup, structured logging, and graceful shutdown on context
// cancellation. It does not yet receive telemetry from anywhere — Kafka
// consumption arrives Day 14 and persistent storage Day 15.
package collector

import (
	"context"
	"log/slog"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/version"
)

// App is the pulse-collector application.
type App struct {
	cfg    config.CollectorConfig
	logger *slog.Logger
}

// New constructs an App from its dependencies.
func New(cfg config.CollectorConfig, logger *slog.Logger) *App {
	return &App{cfg: cfg, logger: logger}
}

// Run starts the collector and blocks until ctx is canceled, then shuts
// down cleanly. It returns nil on a normal, context-driven shutdown.
func (a *App) Run(ctx context.Context) error {
	a.logger.Info("pulse-collector starting",
		"version", version.Version,
		"commit", version.Commit,
	)

	<-ctx.Done()

	a.logger.Info("pulse-collector stopping", "reason", ctx.Err())
	return nil
}
