// Package agent contains the pulse-agent application: startup,
// structured logging of its identity, best-effort process discovery,
// and graceful shutdown on context cancellation.
package agent

import (
	"context"
	"log/slog"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
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
// Process discovery (internal/process) is started on a best-effort
// basis: on a platform or kernel that doesn't support it, or without
// sufficient privilege, Run logs why and continues running without it
// rather than failing to start. Telemetry capture is never allowed to
// be a reason pulse-agent itself won't run.
func (a *App) Run(ctx context.Context) error {
	a.logger.Info("pulse-agent starting",
		"node_name", a.cfg.NodeName,
		"version", version.Version,
		"commit", version.Commit,
	)

	loader := process.NewLoader()
	if err := a.startProcessDiscovery(ctx, loader); err != nil {
		a.logger.Warn("process discovery unavailable", "error", err)
	} else {
		defer loader.Close()
	}

	<-ctx.Done()

	a.logger.Info("pulse-agent stopping", "reason", ctx.Err())
	return nil
}

// processLoader is the subset of *process.Loader's method set
// startProcessDiscovery needs, letting tests substitute a fake without
// touching a real kernel.
type processLoader interface {
	Load() error
	Attach() error
	Read() (process.ProcessEvent, error)
}

// startProcessDiscovery loads and attaches loader, then starts a
// background goroutine logging every event it reports until ctx is
// canceled. On failure, loader has already been left in a state safe
// for the caller to Close() harmlessly (Load/Attach clean up after
// themselves on error), and no goroutine is started.
func (a *App) startProcessDiscovery(ctx context.Context, loader processLoader) error {
	if err := loader.Load(); err != nil {
		return err
	}
	if err := loader.Attach(); err != nil {
		return err
	}

	a.logger.Info("process discovery active")
	go a.watchProcessEvents(ctx, loader)
	return nil
}

// watchProcessEvents logs every process lifecycle event loader reports
// until Read fails — which happens once, by design, when ctx is
// canceled and the caller closes loader (see Run).
//
// Logging every event at info level is a deliberate, temporary choice
// for this foundational stage: it makes the capability visible when
// running pulse-agent directly. Log volume under real load is a later
// concern (aggregation lands with the pipeline in Day 07 onward, not
// here).
func (a *App) watchProcessEvents(ctx context.Context, loader processLoader) {
	for {
		event, err := loader.Read()
		if err != nil {
			if ctx.Err() == nil {
				// Not a shutdown in progress: this is a genuine,
				// unexpected read failure.
				a.logger.Warn("process discovery read failed", "error", err)
			}
			return
		}

		executable := ""
		if event.Type == process.EventStart {
			// Best-effort only, and only for start events — see
			// process.ResolveExecutable's doc comment for why exit
			// events can't reliably resolve this.
			if exe, err := process.ResolveExecutable(event.PID); err == nil {
				executable = exe
			}
		}

		modelEvent := process.ToEvent(event, a.cfg.NodeName, executable)
		a.logger.Info("process event",
			"type", modelEvent.Type,
			"pid", event.PID,
			"ppid", event.PPID,
			"command", event.Comm,
			"executable", executable,
		)
	}
}
