// Package agent contains the pulse-agent application: startup,
// structured logging of its identity, best-effort telemetry capture
// (process discovery, network connection telemetry, socket data
// telemetry, HTTP visibility, DNS telemetry) run through a shared
// internal/pipeline per capability with a shared internal/correlation
// stage across all of them and optional internal/otlp export, and
// graceful shutdown on context cancellation.
package agent

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/correlation"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/dns"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/httpvis"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/network"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/otlp"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/pipeline"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/process"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/socket"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/version"
)

// correlationWindow is how much time may pass between two events from
// the same process before internal/correlation starts a new trace
// rather than chaining onto the previous one. See
// docs/design/trace-correlation.md.
const correlationWindow = 30 * time.Second

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
// shares — process.Loader, network.Loader, socket.Loader,
// httpvis.Loader, and dns.Loader all satisfy this structurally, without
// declaring it themselves.
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

	// One Correlator shared across every capability below is what lets
	// e.g. a process's DNS query and its subsequent TCP connect end up
	// in the same trace — see docs/design/trace-correlation.md. One
	// CorrelatingProcessor built from it, also shared, is what lets
	// every pipeline log (and, if OTLPEndpoint is set, export) through
	// the exact same correlation step rather than each computing its
	// own — see internal/correlation's doc comment on why correlating
	// twice would produce two different spans for one event.
	corrProcessor := &correlation.CorrelatingProcessor{
		Correlator: correlation.New(correlationWindow),
		Logger:     a.logger,
	}

	var exporter *otlp.BatchExporter
	var exporterDone sync.WaitGroup
	if a.cfg.OTLPEndpoint != "" {
		var err error
		exporter, err = otlp.NewBatchExporter(otlp.DefaultConfig(a.cfg.OTLPEndpoint), a.cfg.NodeName, a.logger)
		if err != nil {
			a.logger.Warn("otlp export unavailable", "error", err)
		} else {
			corrProcessor.Exporter = exporter
			a.logger.Info("otlp export active", "endpoint", a.cfg.OTLPEndpoint)
			exporterDone.Add(1)
			go func() {
				defer exporterDone.Done()
				exporter.Run(ctx)
			}()
		}
	}

	processLoader := process.NewLoader()
	networkLoader := network.NewLoader()
	socketLoader := socket.NewLoader()
	httpvisLoader := httpvis.NewLoader()
	dnsLoader := dns.NewLoader()

	candidates := []capability{
		{"process discovery", processLoader, a.newProcessPipeline(processLoader, corrProcessor)},
		{"network connection telemetry", networkLoader, a.newNetworkPipeline(networkLoader, corrProcessor)},
		{"socket data telemetry", socketLoader, a.newSocketPipeline(socketLoader, corrProcessor)},
		{"http visibility", httpvisLoader, a.newHTTPVisPipeline(httpvisLoader, corrProcessor)},
		{"dns telemetry", dnsLoader, a.newDNSPipeline(dnsLoader, corrProcessor)},
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

	// The exporter's own Run goroutine already stops (and flushes
	// whatever was queued) on ctx cancellation; wait for it to actually
	// finish before closing its connection, or a still-in-flight final
	// export could be cut off mid-call.
	if exporter != nil {
		exporterDone.Wait()
		exporter.Close()
	}

	return nil
}
