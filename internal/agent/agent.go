// Package agent contains the pulse-agent application: startup,
// structured logging of its identity, best-effort telemetry capture
// (process discovery, network connection telemetry), and graceful
// shutdown on context cancellation.
package agent

import (
	"context"
	"log/slog"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/network"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/process"
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
//
// Each telemetry capability (process.go, network.go) is started on a
// best-effort basis: on a platform or kernel that doesn't support it,
// or without sufficient privilege, Run logs why and continues running
// without it rather than failing to start. Telemetry capture is never
// allowed to be a reason pulse-agent itself won't run.
func (a *App) Run(ctx context.Context) error {
	a.logger.Info("pulse-agent starting",
		"node_name", a.cfg.NodeName,
		"version", version.Version,
		"commit", version.Commit,
	)

	processLoader := process.NewLoader()
	if err := a.startProcessDiscovery(ctx, processLoader); err != nil {
		a.logger.Warn("process discovery unavailable", "error", err)
	} else {
		defer processLoader.Close()
	}

	networkLoader := network.NewLoader()
	if err := a.startNetworkTelemetry(ctx, networkLoader); err != nil {
		a.logger.Warn("network connection telemetry unavailable", "error", err)
	} else {
		defer networkLoader.Close()
	}

	<-ctx.Done()

	a.logger.Info("pulse-agent stopping", "reason", ctx.Err())
	return nil
}
