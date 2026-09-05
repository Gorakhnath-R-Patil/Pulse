// Package tracing implements Pulse's trace assembly logic. Span
// identity, parent linkage, and the rest of the data model live in
// pkg/model — this package organizes a set of spans that already share
// one TraceID into a tree by their ParentSpanID relationships.
//
// Deciding which raw telemetry events (from internal/process,
// internal/network, and the rest) become which spans in the first
// place is a distinct concern — see docs/design/trace-model.md for
// where that boundary is and why.
package tracing

import "errors"

var (
	// ErrEmptyTrace is returned by AssembleTrace when given no spans.
	ErrEmptyTrace = errors.New("tracing: no spans given")

	// ErrMixedTraceIDs is returned by AssembleTrace when its spans
	// don't all share one TraceID.
	ErrMixedTraceIDs = errors.New("tracing: spans have different trace IDs")
)
