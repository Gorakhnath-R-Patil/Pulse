// Package agent contains the pulse-agent application skeleton: startup,
// structured logging of its identity, and graceful shutdown on context
// cancellation. It intentionally does not yet observe anything — eBPF
// program loading begins Day 03, process discovery Day 04, and so on.
package agent

import (
	"context"
	"log/slog"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/version"
)

// App is the pulse-agent application. Its dependencies (config, logger)
// are passed in explicitly rather than read from globals, so it can be
// constructed and tested without touching the environment or filesystem.
type App struct {
	cfg    config.AgentConfig
	logger *slog.Logger
}

// New constructs an App from its dependencies.
func New(cfg config.AgentConfig, logger *slog.Logger) *App {
	return &App{cfg: cfg, logger: logger}
}

// Run starts the agent and blocks until ctx is canceled, then shuts down
// cleanly. It returns nil on a normal, context-driven shutdown.
func (a *App) Run(ctx context.Context) error {
	a.logger.Info("pulse-agent starting",
		"node_name", a.cfg.NodeName,
		"version", version.Version,
		"commit", version.Commit,
	)

	<-ctx.Done()

	a.logger.Info("pulse-agent stopping", "reason", ctx.Err())
	return nil
}
