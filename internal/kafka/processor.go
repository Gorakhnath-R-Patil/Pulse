package kafka

import (
	"context"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// ProducingProcessor adapts a Producer to internal/pipeline's
// EventProcessor interface, so producing to Kafka is one more step in
// each capability's pipeline alongside pipeline.LoggingProcessor and
// correlation.CorrelatingProcessor — see internal/agent's wiring.
type ProducingProcessor struct {
	Producer *Producer
}

// Process produces event to Kafka. A non-nil error is logged by the
// pipeline that called it and never stops processing — the same
// best-effort telemetry-export behavior internal/otlp's exporter and
// internal/correlation.CorrelatingProcessor's Exporter forwarding
// already establish: telemetry transport failing is never allowed to
// be a reason capture itself breaks.
func (p *ProducingProcessor) Process(ctx context.Context, event model.Event) error {
	return p.Producer.Produce(ctx, event)
}
