package pipeline

import (
	"context"
	"log/slog"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// LoggingProcessor logs every event it's given as a single structured
// line. It replaces the per-capability logging code internal/agent used
// to hand-write for process/network/socket events separately: since
// pkg/model.Event already carries every domain's fields in structured
// form (Process, Network, Attributes), one generic implementation logs
// them all equally well.
type LoggingProcessor struct {
	Logger *slog.Logger
}

// Process logs event and always returns nil: a logging failure isn't a
// processing failure worth reporting back to the pipeline (slog itself
// doesn't surface write errors to its callers).
func (p *LoggingProcessor) Process(_ context.Context, event model.Event) error {
	attrs := make([]any, 0, 16)
	attrs = append(attrs, "type", event.Type, "id", event.ID)

	if event.Process != nil {
		attrs = append(attrs, "pid", event.Process.PID, "command", event.Process.Command)
		if event.Process.Executable != "" {
			attrs = append(attrs, "executable", event.Process.Executable)
		}
	}

	if event.Network != nil {
		attrs = append(attrs,
			"source", event.Network.Source.Address,
			"source_port", event.Network.Source.Port,
			"destination", event.Network.Destination.Address,
			"destination_port", event.Network.Destination.Port,
		)
		if event.Network.BytesSent != 0 || event.Network.BytesReceived != 0 {
			attrs = append(attrs, "bytes_sent", event.Network.BytesSent, "bytes_received", event.Network.BytesReceived)
		}
	}

	for k, v := range event.Attributes {
		attrs = append(attrs, k, v)
	}

	p.Logger.Info("telemetry event", attrs...)
	return nil
}
