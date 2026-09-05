package correlation

import (
	"context"
	"log/slog"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// CorrelatingProcessor is a pipeline.EventProcessor (see
// internal/pipeline) that correlates every event it's given through a
// shared Correlator and logs the resulting span. Sharing one
// Correlator — and hence one CorrelatingProcessor built from it —
// across every capability's pipeline is what lets events from
// different capabilities (process discovery, network, DNS, ...) on the
// same process end up in the same trace; see internal/agent's wiring.
//
// Logging is kept separate from Correlator itself so Correlator stays
// pure and testable without a logger — see correlator_test.go.
type CorrelatingProcessor struct {
	Correlator *Correlator
	Logger     *slog.Logger
}

// Process correlates event and logs the resulting span. It always
// returns nil: Correlator.Observe never fails, and a logging failure
// isn't a processing failure worth reporting back to the pipeline —
// the same rationale pipeline.LoggingProcessor already documents.
func (p *CorrelatingProcessor) Process(_ context.Context, event model.Event) error {
	span := p.Correlator.Observe(event)

	p.Logger.Info("span",
		"trace_id", span.TraceID,
		"span_id", span.SpanID,
		"parent_span_id", span.ParentSpanID,
		"name", span.Name,
		"service", span.Service,
	)
	return nil
}
