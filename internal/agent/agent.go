// Package agent contains the pulse-agent application: startup,
// structured logging of its identity, best-effort telemetry capture
// (process discovery, network connection telemetry, socket data
// telemetry, HTTP visibility, DNS telemetry) run through a shared
// internal/pipeline per capability with a shared internal/correlation
// stage across all of them and optional internal/otlp export,
// internal/kafka production, and internal/metrics exposition, and
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
	"github.com/Gorakhnath-R-Patil/Pulse/internal/kafka"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/metrics"
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

	// Kafka production runs alongside (not instead of) logging and
	// correlation, one more processor in every pipeline's chain — see
	// docs/design/kafka-transport.md. Unlike the OTLP exporter, a
	// kafka.Producer has no background Run loop of its own to wait on:
	// kafka-go's Writer batches internally and Close flushes it
	// synchronously, so shutdown only needs to call Close once every
	// pipeline has stopped producing to it.
	var extra []pipeline.EventProcessor

	var kafkaProducer *kafka.Producer
	if len(a.cfg.KafkaBrokers) > 0 {
		kafkaProducer = kafka.NewProducer(kafka.ProducerConfig{Brokers: a.cfg.KafkaBrokers, Topic: a.cfg.KafkaTopic})
		extra = append(extra, &kafka.ProducingProcessor{Producer: kafkaProducer})
		a.logger.Info("kafka production active", "brokers", a.cfg.KafkaBrokers, "topic", a.cfg.KafkaTopic)
	}

	// Metrics recording is the same shape again: one more optional
	// processor, plus a background HTTP server (like the OTLP
	// exporter's Run, not like Kafka's Producer, since serving
	// /metrics is itself an ongoing job, not a batched write) that
	// Run waits on during shutdown. See docs/design/metrics.md.
	var metricsServer *metrics.Server
	var metricsServerDone sync.WaitGroup
	if a.cfg.MetricsAddr != "" {
		registry := metrics.New()
		extra = append(extra, &metrics.Processor{Registry: registry})
		metricsServer = metrics.NewServer(a.cfg.MetricsAddr, registry)
		a.logger.Info("metrics server active", "addr", a.cfg.MetricsAddr)
		metricsServerDone.Add(1)
		go func() {
			defer metricsServerDone.Done()
			if err := metricsServer.Run(ctx); err != nil {
				a.logger.Warn("metrics server stopped", "error", err)
			}
		}()
	}

	processLoader := process.NewLoader()
	networkLoader := network.NewLoader()
	socketLoader := socket.NewLoader()
	httpvisLoader := httpvis.NewLoader()
	dnsLoader := dns.NewLoader()

	candidates := []capability{
		{"process discovery", processLoader, a.newProcessPipeline(processLoader, corrProcessor, extra...)},
		{"network connection telemetry", networkLoader, a.newNetworkPipeline(networkLoader, corrProcessor, extra...)},
		{"socket data telemetry", socketLoader, a.newSocketPipeline(socketLoader, corrProcessor, extra...)},
		{"http visibility", httpvisLoader, a.newHTTPVisPipeline(httpvisLoader, corrProcessor, extra...)},
		{"dns telemetry", dnsLoader, a.newDNSPipeline(dnsLoader, corrProcessor, extra...)},
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

	// Every pipeline that could still call kafkaProducer.Produce has
	// already stopped (running.Wait() above), so it's safe to close it
	// now — unlike the exporter, there's no separate goroutine left to
	// wait for.
	if kafkaProducer != nil {
		if err := kafkaProducer.Close(); err != nil {
			a.logger.Warn("kafka producer close failed", "error", err)
		}
	}

	// metricsServer.Run already began its own graceful shutdown the
	// moment ctx was canceled above (it watches the same ctx); wait
	// for that to actually finish before returning.
	metricsServerDone.Wait()

	return nil
}
