package agent

import (
	"context"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/process"
)

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
