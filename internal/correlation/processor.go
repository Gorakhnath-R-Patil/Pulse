package correlation

import (
	"context"
	"log/slog"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// SpanExporter is anything CorrelatingProcessor can forward newly
// correlated spans to for export, in addition to logging them.
// *internal/otlp.BatchExporter satisfies this; nothing in this package
// depends on internal/otlp itself, keeping the dependency direction
// the other way around — see docs/design/otlp-export.md.
type SpanExporter interface {
	Enqueue(span model.Span)
}

// CorrelatingProcessor is a pipeline.EventProcessor (see
// internal/pipeline) that correlates every event it's given through a
// shared Correlator, logs the resulting span, and — if Exporter is set
// — forwards it for export. Sharing one Correlator — and hence one
// CorrelatingProcessor built from it — across every capability's
// pipeline is what lets events from different capabilities (process
// discovery, network, DNS, ...) on the same process end up in the same
// trace; see internal/agent's wiring.
//
// Correlation happens exactly once per event here, regardless of
// whether Exporter is set: Observe is called a single time and its
// result is both logged and (optionally) exported, rather than each
// concern calling Observe separately and producing two different spans
// (different SpanIDs) for what should be one.
//
// Logging is kept separate from Correlator itself so Correlator stays
// pure and testable without a logger — see correlator_test.go.
type CorrelatingProcessor struct {
	Correlator *Correlator
	Logger     *slog.Logger

	// Exporter is optional. A nil Exporter means spans are logged only
	// — see internal/agent's wiring for how an unset OTLP endpoint
	// leaves this nil rather than pointing at a made-up default.
	Exporter SpanExporter
}

// Process correlates event, logs the resulting span, and forwards it to
// Exporter if one is set. It always returns nil: Correlator.Observe
// never fails, and neither a logging nor an export-enqueue failure is a
// processing failure worth reporting back to the pipeline — the same
// rationale pipeline.LoggingProcessor already documents.
func (p *CorrelatingProcessor) Process(_ context.Context, event model.Event) error {
	span := p.Correlator.Observe(event)

	p.Logger.Info("span",
		"trace_id", span.TraceID,
		"span_id", span.SpanID,
		"parent_span_id", span.ParentSpanID,
		"name", span.Name,
		"service", span.Service,
	)

	if p.Exporter != nil {
		p.Exporter.Enqueue(span)
	}

	return nil
}
