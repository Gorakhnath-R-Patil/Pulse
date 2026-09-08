package otlp

import (
	"encoding/hex"
	"testing"
	"time"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

func TestSpanToProto_IDsRoundTripAsRawBytes(t *testing.T) {
	traceID := model.NewTraceID()
	spanID := model.NewSpanID()
	s := model.Span{
		TraceID:   traceID,
		SpanID:    spanID,
		Name:      "network.connect",
		Service:   "order-service",
		StartTime: time.Now(),
	}

	got := spanToProto(s)

	wantTraceBytes, _ := hex.DecodeString(string(traceID))
	wantSpanBytes, _ := hex.DecodeString(string(spanID))
	if string(got.TraceId) != string(wantTraceBytes) {
		t.Errorf("TraceId = %x, want %x", got.TraceId, wantTraceBytes)
	}
	if string(got.SpanId) != string(wantSpanBytes) {
		t.Errorf("SpanId = %x, want %x", got.SpanId, wantSpanBytes)
	}
	if len(got.TraceId) != 16 {
		t.Errorf("len(TraceId) = %d, want 16 (OTLP's required trace ID length)", len(got.TraceId))
	}
	if len(got.SpanId) != 8 {
		t.Errorf("len(SpanId) = %d, want 8 (OTLP's required span ID length)", len(got.SpanId))
	}
}

func TestSpanToProto_RootSpanHasNoParentID(t *testing.T) {
	s := model.Span{TraceID: model.NewTraceID(), SpanID: model.NewSpanID(), Name: "a", Service: "svc", StartTime: time.Now()}

	got := spanToProto(s)
	if len(got.ParentSpanId) != 0 {
		t.Errorf("ParentSpanId = %x, want empty for a root span", got.ParentSpanId)
	}
}

func TestSpanToProto_ParentSpanIDConverted(t *testing.T) {
	parentID := model.NewSpanID()
	s := model.Span{
		TraceID: model.NewTraceID(), SpanID: model.NewSpanID(), ParentSpanID: parentID,
		Name: "a", Service: "svc", StartTime: time.Now(),
	}

	got := spanToProto(s)
	want, _ := hex.DecodeString(string(parentID))
	if string(got.ParentSpanId) != string(want) {
		t.Errorf("ParentSpanId = %x, want %x", got.ParentSpanId, want)
	}
}

func TestSpanToProto_TimestampsConverted(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(150 * time.Millisecond)
	s := model.Span{TraceID: model.NewTraceID(), SpanID: model.NewSpanID(), Name: "a", Service: "svc", StartTime: start, EndTime: end}

	got := spanToProto(s)
	if got.StartTimeUnixNano != uint64(start.UnixNano()) {
		t.Errorf("StartTimeUnixNano = %d, want %d", got.StartTimeUnixNano, start.UnixNano())
	}
	if got.EndTimeUnixNano != uint64(end.UnixNano()) {
		t.Errorf("EndTimeUnixNano = %d, want %d", got.EndTimeUnixNano, end.UnixNano())
	}
}

func TestSpanToProto_NoEndTimeLeavesEndTimeUnixNanoZero(t *testing.T) {
	s := model.Span{TraceID: model.NewTraceID(), SpanID: model.NewSpanID(), Name: "a", Service: "svc", StartTime: time.Now()}

	got := spanToProto(s)
	if got.EndTimeUnixNano != 0 {
		t.Errorf("EndTimeUnixNano = %d, want 0 for a span with no EndTime", got.EndTimeUnixNano)
	}
}

func TestSpanToProto_AttributesConverted(t *testing.T) {
	s := model.Span{
		TraceID: model.NewTraceID(), SpanID: model.NewSpanID(), Name: "dns.query", Service: "svc",
		StartTime:  time.Now(),
		Attributes: map[string]string{"dns.name": "example.com"},
	}

	got := spanToProto(s)
	if len(got.Attributes) != 1 {
		t.Fatalf("len(Attributes) = %d, want 1", len(got.Attributes))
	}
	if got.Attributes[0].Key != "dns.name" {
		t.Errorf("Attributes[0].Key = %q, want %q", got.Attributes[0].Key, "dns.name")
	}
	if got.Attributes[0].Value.GetStringValue() != "example.com" {
		t.Errorf("Attributes[0].Value = %q, want %q", got.Attributes[0].Value.GetStringValue(), "example.com")
	}
}

func TestSpansToResourceSpans_GroupsByService(t *testing.T) {
	t0 := time.Now()
	orderSpan := model.Span{TraceID: model.NewTraceID(), SpanID: model.NewSpanID(), Name: "a", Service: "order-service", StartTime: t0}
	paymentSpan := model.Span{TraceID: model.NewTraceID(), SpanID: model.NewSpanID(), Name: "b", Service: "payment-service", StartTime: t0}
	orderSpan2 := model.Span{TraceID: model.NewTraceID(), SpanID: model.NewSpanID(), Name: "c", Service: "order-service", StartTime: t0}

	got := spansToResourceSpans("pulse-node-1", []model.Span{orderSpan, paymentSpan, orderSpan2})

	if len(got) != 2 {
		t.Fatalf("len(ResourceSpans) = %d, want 2 (one per distinct service)", len(got))
	}

	total := 0
	for _, rs := range got {
		for _, ss := range rs.ScopeSpans {
			total += len(ss.Spans)
		}
	}
	if total != 3 {
		t.Errorf("total spans across all ResourceSpans = %d, want 3", total)
	}

	// The order-service ResourceSpans should carry both of its spans.
	foundOrderService := false
	for _, rs := range got {
		for _, attr := range rs.Resource.Attributes {
			if attr.Key == "service.name" && attr.Value.GetStringValue() == "order-service" {
				foundOrderService = true
				var count int
				for _, ss := range rs.ScopeSpans {
					count += len(ss.Spans)
				}
				if count != 2 {
					t.Errorf("order-service ResourceSpans has %d spans, want 2", count)
				}
			}
		}
	}
	if !foundOrderService {
		t.Error("no ResourceSpans found with service.name = order-service")
	}
}

func TestSpansToResourceSpans_SetsHostAttribute(t *testing.T) {
	s := model.Span{TraceID: model.NewTraceID(), SpanID: model.NewSpanID(), Name: "a", Service: "svc", StartTime: time.Now()}

	got := spansToResourceSpans("pulse-node-1", []model.Span{s})

	if len(got) != 1 {
		t.Fatalf("len(ResourceSpans) = %d, want 1", len(got))
	}
	var foundHost bool
	for _, attr := range got[0].Resource.Attributes {
		if attr.Key == "host.name" && attr.Value.GetStringValue() == "pulse-node-1" {
			foundHost = true
		}
	}
	if !foundHost {
		t.Error("Resource.Attributes missing host.name = pulse-node-1")
	}
}

// sanity check that spanToProto's Kind is always set to something
// meaningful rather than left at the zero value (SPAN_KIND_UNSPECIFIED),
// which OTLP consumers may treat as a signal something is wrong.
func TestSpanToProto_KindIsSet(t *testing.T) {
	s := model.Span{TraceID: model.NewTraceID(), SpanID: model.NewSpanID(), Name: "a", Service: "svc", StartTime: time.Now()}
	if got := spanToProto(s).Kind; got != tracev1.Span_SPAN_KIND_INTERNAL {
		t.Errorf("Kind = %v, want SPAN_KIND_INTERNAL", got)
	}
}
