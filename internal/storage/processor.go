package storage

import (
	"context"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// Processor adapts a BatchWriter to internal/pipeline's EventProcessor
// interface, so storing to ClickHouse is one more step in
// pulse-collector's consumption pipeline alongside
// pipeline.LoggingProcessor — see internal/collector's wiring.
type Processor struct {
	Writer *BatchWriter
}

// Process enqueues event for storage and always returns nil: Enqueue
// cannot fail synchronously (it only blocks under backpressure, or
// succeeds), and a write failure surfaces later, from Run, logged
// rather than reported back to whatever called Process — the same
// best-effort telemetry-storage behavior internal/otlp's exporter and
// internal/kafka's producer already establish.
func (p *Processor) Process(_ context.Context, event model.Event) error {
	p.Writer.Enqueue(event)
	return nil
}
