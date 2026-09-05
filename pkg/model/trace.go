package model

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// TraceID uniquely identifies one distributed trace: a chain of
// causally-related operations, possibly spanning multiple services.
// Represented as 32 lowercase hex characters (a 128-bit value) to
// match the W3C Trace Context / OTLP convention Day 13's export will
// need to interoperate with, even though nothing generates or parses
// that specific wire format outside this package yet.
type TraceID string

// SpanID identifies one span within a trace: a 64-bit value as 16
// lowercase hex characters, matching the same convention.
type SpanID string

// NewTraceID generates a random TraceID.
func NewTraceID() TraceID {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("model: failed to read random bytes for NewTraceID: %v", err))
	}
	return TraceID(hex.EncodeToString(b[:]))
}

// NewSpanID generates a random SpanID.
func NewSpanID() SpanID {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("model: failed to read random bytes for NewSpanID: %v", err))
	}
	return SpanID(hex.EncodeToString(b[:]))
}

// Span is one operation within a distributed trace. Unlike an Event,
// which describes something that happened at an instant, a Span
// describes something with a start and — usually — an end.
//
// This is Pulse's own minimal span shape, not OpenTelemetry's:
// docs/design/event-model.md's Alternatives section already rejected
// adopting OTel's data model wholesale as more machinery than was
// needed then, and that reasoning still applies here. What's here is
// only what this day actually needs: identity, parent linkage, timing,
// service identity, and attributes. OTLP compatibility remains a later,
// explicit goal (Day 13) this may converge toward, not something this
// type commits to today.
type Span struct {
	TraceID TraceID `json:"trace_id"`
	SpanID  SpanID  `json:"span_id"`

	// ParentSpanID is empty for a trace's root span.
	ParentSpanID SpanID `json:"parent_span_id,omitempty"`

	// Name identifies the operation this span represents, e.g.
	// "network.connect" or "http.request" — typically an Event.Type
	// value, since correlating raw events into spans is expected to
	// build them this way (see docs/design/trace-model.md).
	Name string `json:"name"`

	// Service identifies which service performed this operation. This
	// package doesn't resolve one from raw telemetry — deciding that is
	// a correlation concern, not a data-model one — it only holds
	// whatever value the caller assigns.
	Service string `json:"service"`

	StartTime time.Time `json:"start_time"`

	// EndTime is the zero value for a span that hasn't ended yet.
	EndTime time.Time `json:"end_time,omitempty"`

	Attributes map[string]string `json:"attributes,omitempty"`
}

// Duration reports how long the span ran. It returns 0 for a span
// without an EndTime — a still-running span, not one that took no time
// at all.
func (s Span) Duration() time.Duration {
	if s.EndTime.IsZero() {
		return 0
	}
	return s.EndTime.Sub(s.StartTime)
}

// Validate reports whether s is well-formed enough to assemble into a
// trace.
func (s Span) Validate() error {
	if s.TraceID == "" {
		return fmt.Errorf("%w: trace_id", ErrMissingField)
	}
	if s.SpanID == "" {
		return fmt.Errorf("%w: span_id", ErrMissingField)
	}
	if s.Name == "" {
		return fmt.Errorf("%w: name", ErrMissingField)
	}
	if s.Service == "" {
		return fmt.Errorf("%w: service", ErrMissingField)
	}
	if s.StartTime.IsZero() {
		return fmt.Errorf("%w: start_time", ErrMissingField)
	}
	return nil
}
