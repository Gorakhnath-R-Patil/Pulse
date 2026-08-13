package agent

import (
	"context"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/network"
)

// networkLoader is the subset of *network.Loader's method set
// startNetworkTelemetry needs, letting tests substitute a fake without
// touching a real kernel. Mirrors processLoader in process.go — see
// docs/design/network-connect.md for why this is a second, near-
// identical instance of the pattern rather than a shared abstraction.
type networkLoader interface {
	Load() error
	Attach() error
	Read() (network.ConnectEvent, error)
}

// startNetworkTelemetry loads and attaches loader, then starts a
// background goroutine logging every event it reports until ctx is
// canceled. On failure, loader has already been left in a state safe
// for the caller to Close() harmlessly, and no goroutine is started.
func (a *App) startNetworkTelemetry(ctx context.Context, loader networkLoader) error {
	if err := loader.Load(); err != nil {
		return err
	}
	if err := loader.Attach(); err != nil {
		return err
	}

	a.logger.Info("network connection telemetry active")
	go a.watchNetworkEvents(ctx, loader)
	return nil
}

// watchNetworkEvents logs every TCP connect event loader reports until
// Read fails — which happens once, by design, when ctx is canceled and
// the caller closes loader (see Run). See watchProcessEvents in
// process.go for why every event is logged at info level for now.
func (a *App) watchNetworkEvents(ctx context.Context, loader networkLoader) {
	for {
		event, err := loader.Read()
		if err != nil {
			if ctx.Err() == nil {
				a.logger.Warn("network connection telemetry read failed", "error", err)
			}
			return
		}

		modelEvent := network.ToEvent(event, a.cfg.NodeName)
		a.logger.Info("network connect event",
			"type", modelEvent.Type,
			"pid", event.PID,
			"command", event.Comm,
			"source", modelEvent.Network.Source.Address,
			"source_port", event.SourcePort,
			"destination", modelEvent.Network.Destination.Address,
			"destination_port", event.DestPort,
			"success", event.Success,
		)
	}
}
