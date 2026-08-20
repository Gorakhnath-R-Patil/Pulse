// Package agent contains the pulse-agent application: startup,
// structured logging of its identity, best-effort telemetry capture
// (process discovery, network connection telemetry, socket data
// telemetry) run through a shared internal/pipeline per capability, and
// graceful shutdown on context cancellation.
package agent

import (
	"context"
	"log/slog"
	"sync"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/network"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/pipeline"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/process"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/socket"
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

// capabilityLoader is the lifecycle every telemetry capability's loader
// shares — process.Loader, network.Loader, and socket.Loader all
// satisfy this structurally, without declaring it themselves.
type capabilityLoader interface {
	Load() error
	Attach() error
	Close() error
}

// capability pairs a telemetry capability's loader with the pipeline
// that reads from it, so Run can start, log, and shut all of them down
// uniformly regardless of what domain each one covers.
type capability struct {
	name     string
	loader   capabilityLoader
	pipeline *pipeline.Pipeline
}

// Run starts the agent and blocks until ctx is canceled, then shuts down
// cleanly. It returns nil on a normal, context-driven shutdown.
//
// Each telemetry capability is started on a best-effort basis: on a
// platform or kernel that doesn't support it, or without sufficient
// privilege, Run logs why and continues running without it rather than
// failing to start. Telemetry capture is never allowed to be a reason
// pulse-agent itself won't run. Once started, a capability's pipeline
// runs until Run closes its loader (unblocking the pipeline's read
// loop) and waits for it to finish draining in-flight work — see
// internal/pipeline's Run for the graceful shutdown contract this
// relies on.
func (a *App) Run(ctx context.Context) error {
	a.logger.Info("pulse-agent starting",
		"node_name", a.cfg.NodeName,
		"version", version.Version,
		"commit", version.Commit,
	)

	processLoader := process.NewLoader()
	networkLoader := network.NewLoader()
	socketLoader := socket.NewLoader()

	candidates := []capability{
		{"process discovery", processLoader, a.newProcessPipeline(processLoader)},
		{"network connection telemetry", networkLoader, a.newNetworkPipeline(networkLoader)},
		{"socket data telemetry", socketLoader, a.newSocketPipeline(socketLoader)},
	}

	var active []capability
	var running sync.WaitGroup
	for _, c := range candidates {
		if err := c.loader.Load(); err != nil {
			a.logger.Warn(c.name+" unavailable", "error", err)
			continue
		}
		if err := c.loader.Attach(); err != nil {
			a.logger.Warn(c.name+" unavailable", "error", err)
			c.loader.Close()
			continue
		}

		a.logger.Info(c.name + " active")
		active = append(active, c)
		running.Add(1)
		go func(p *pipeline.Pipeline) {
			defer running.Done()
			p.Run(ctx)
		}(c.pipeline)
	}

	<-ctx.Done()

	a.logger.Info("pulse-agent stopping", "reason", ctx.Err())

	for _, c := range active {
		c.loader.Close()
	}
	running.Wait()

	return nil
}
