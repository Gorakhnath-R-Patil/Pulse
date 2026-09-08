// Package otlp exports pkg/model.Span values to an OTLP/gRPC collector
// (the OpenTelemetry Protocol's trace export API) — batching, retrying,
// and applying backpressure, but not attempting broader OTLP
// compatibility than that. See docs/design/otlp-export.md for exactly
// what "OTLP-compatible" means here and what it doesn't.
package otlp

import (
	"encoding/hex"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// spansToResourceSpans converts spans into OTLP's ResourceSpans shape,
// one ResourceSpans per distinct Service: OTLP's Resource identifies a
// single service/process, so a batch spanning multiple services needs
// to say so explicitly rather than attributing every span to whichever
// service happened to appear first in the batch.
func spansToResourceSpans(host string, spans []model.Span) []*tracev1.ResourceSpans {
	var order []string
	bySvc := make(map[string][]*tracev1.Span)
	for _, s := range spans {
		if _, ok := bySvc[s.Service]; !ok {
			order = append(order, s.Service)
		}
		bySvc[s.Service] = append(bySvc[s.Service], spanToProto(s))
	}

	result := make([]*tracev1.ResourceSpans, 0, len(order))
	for _, svc := range order {
		result = append(result, &tracev1.ResourceSpans{
			Resource: &resourcev1.Resource{
				Attributes: []*commonv1.KeyValue{
					stringAttr("service.name", svc),
					stringAttr("host.name", host),
				},
			},
			ScopeSpans: []*tracev1.ScopeSpans{
				{Spans: bySvc[svc]},
			},
		})
	}
	return result
}

// spanToProto converts one model.Span to OTLP's Span message. TraceID
// and SpanID convert cleanly with no loss: they were deliberately
// chosen (Day 11) to already be OTLP's own 32-hex/16-hex ID format, so
// this is a plain hex decode, not a reinterpretation.
func spanToProto(s model.Span) *tracev1.Span {
	span := &tracev1.Span{
		TraceId:           traceIDBytes(s.TraceID),
		SpanId:            spanIDBytes(s.SpanID),
		Name:              s.Name,
		Kind:              tracev1.Span_SPAN_KIND_INTERNAL,
		StartTimeUnixNano: uint64(s.StartTime.UnixNano()),
	}
	if s.ParentSpanID != "" {
		span.ParentSpanId = spanIDBytes(s.ParentSpanID)
	}
	if !s.EndTime.IsZero() {
		span.EndTimeUnixNano = uint64(s.EndTime.UnixNano())
	}
	for k, v := range s.Attributes {
		span.Attributes = append(span.Attributes, stringAttr(k, v))
	}
	return span
}

func stringAttr(key, value string) *commonv1.KeyValue {
	return &commonv1.KeyValue{
		Key:   key,
		Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: value}},
	}
}

// traceIDBytes and spanIDBytes decode a model.TraceID/SpanID's hex text
// into the raw bytes OTLP's wire format expects. A decode failure
// (which model.NewTraceID/NewSpanID's own output can never actually
// produce — this only guards against a Span constructed by hand with a
// malformed ID) yields nil, which OTLP itself treats as an invalid ID
// rather than something spanToProto needs to reject up front.
func traceIDBytes(id model.TraceID) []byte {
	b, err := hex.DecodeString(string(id))
	if err != nil {
		return nil
	}
	return b
}

func spanIDBytes(id model.SpanID) []byte {
	b, err := hex.DecodeString(string(id))
	if err != nil {
		return nil
	}
	return b
}
