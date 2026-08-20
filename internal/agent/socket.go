package agent

import (
	"context"
	"sync/atomic"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/socket"
)

// socketEventBufferSize bounds the queue between reading socket close
// events off the ring buffer and logging them. It exists so a slow or
// stalled consumer degrades to dropped (and counted) events instead of
// blocking the read loop indefinitely — the read loop's job is to drain
// the kernel ring buffer promptly, not to buffer for a slow consumer.
// This is deliberately the simplest possible bounded buffer, not
// Day 07's fuller pipeline (worker pools, multi-stage backpressure);
// see docs/design/socket-data.md.
const socketEventBufferSize = 256

// socketLoader is the subset of *socket.Loader's method set
// startSocketTelemetry needs, letting tests substitute a fake without
// touching a real kernel. Mirrors processLoader/networkLoader.
type socketLoader interface {
	Load() error
	Attach() error
	Read() (socket.CloseEvent, error)
}

// startSocketTelemetry loads and attaches loader, then starts the
// bounded read/log pipeline described on socketEventBufferSize running
// until ctx is canceled. On failure, loader has already been left in a
// state safe for the caller to Close() harmlessly, and nothing is
// started.
func (a *App) startSocketTelemetry(ctx context.Context, loader socketLoader) error {
	if err := loader.Load(); err != nil {
		return err
	}
	if err := loader.Attach(); err != nil {
		return err
	}

	a.logger.Info("socket data telemetry active")

	events := make(chan socket.CloseEvent, socketEventBufferSize)
	go a.readSocketEvents(ctx, loader, events)
	go a.logSocketEvents(events)
	return nil
}

// readSocketEvents drains loader as fast as the kernel delivers events,
// forwarding each one to out without blocking: if out is full, the
// event is dropped and counted rather than backing up the read loop.
// Returns (closing out) once Read fails — see watchProcessEvents in
// process.go for why a shutdown-triggered failure is silent and an
// unexpected one is warned.
func (a *App) readSocketEvents(ctx context.Context, loader socketLoader, out chan<- socket.CloseEvent) {
	defer close(out)

	var dropped atomic.Uint64
	for {
		event, err := loader.Read()
		if err != nil {
			if ctx.Err() == nil {
				a.logger.Warn("socket telemetry read failed", "error", err)
			}
			return
		}

		select {
		case out <- event:
		default:
			n := dropped.Add(1)
			// Logged on the first drop and every 100th after, so a
			// sustained overload doesn't itself become a log-volume
			// problem while still surfacing that it's happening.
			if n == 1 || n%100 == 0 {
				a.logger.Warn("socket telemetry buffer full, dropping event", "dropped_total", n)
			}
		}
	}
}

// logSocketEvents normalizes and logs every event sent to in, until in
// is closed (by readSocketEvents, once its Read loop ends).
func (a *App) logSocketEvents(in <-chan socket.CloseEvent) {
	for event := range in {
		modelEvent := socket.ToEvent(event, a.cfg.NodeName)
		a.logger.Info("socket close event",
			"type", modelEvent.Type,
			"pid", event.PID,
			"command", event.Comm,
			"source", modelEvent.Network.Source.Address,
			"source_port", event.SourcePort,
			"destination", modelEvent.Network.Destination.Address,
			"destination_port", event.DestPort,
			"bytes_sent", event.BytesSent,
			"bytes_received", event.BytesReceived,
			"sock_error", event.SockError,
		)
	}
}
