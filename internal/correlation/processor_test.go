package correlation_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/correlation"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

func TestCorrelatingProcessor_LogsSpan(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	p := &correlation.CorrelatingProcessor{Correlator: correlation.New(30 * time.Second), Logger: logger}

	event := model.Event{
		Type:      "network.connect",
		Timestamp: time.Now(),
		Host:      "pulse-node-1",
		Process:   &model.Process{PID: 100, Command: "curl"},
	}

	if err := p.Process(context.Background(), event); err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `"trace_id"`) {
		t.Errorf("log output missing trace_id: %s", out)
	}
	if !strings.Contains(out, `"service":"curl"`) {
		t.Errorf("log output missing the service label: %s", out)
	}
	if !strings.Contains(out, `"name":"network.connect"`) {
		t.Errorf("log output missing the span name: %s", out)
	}
}

func TestCorrelatingProcessor_ChainsAcrossCalls(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	corr := correlation.New(30 * time.Second)
	p := &correlation.CorrelatingProcessor{Correlator: corr, Logger: logger}

	t0 := time.Now()
	first := model.Event{Type: "dns.query", Timestamp: t0, Process: &model.Process{PID: 100, Command: "curl"}}
	second := model.Event{Type: "network.connect", Timestamp: t0.Add(time.Second), Process: &model.Process{PID: 100, Command: "curl"}}

	// Exercise through the shared Correlator directly to get the two
	// spans' IDs, confirming the *same* Correlator instance a
	// CorrelatingProcessor wraps is what makes chaining across
	// capabilities possible (see internal/agent's wiring, which shares
	// one Correlator across every pipeline's processor).
	firstSpan := corr.Observe(first)
	if err := p.Process(context.Background(), second); err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `"parent_span_id":"`+string(firstSpan.SpanID)+`"`) {
		t.Errorf("log output missing the expected parent_span_id (%s): %s", firstSpan.SpanID, out)
	}
}

type fakeExporter struct {
	spans []model.Span
}

func (f *fakeExporter) Enqueue(span model.Span) {
	f.spans = append(f.spans, span)
}

func TestCorrelatingProcessor_ForwardsToExporterWhenSet(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	exporter := &fakeExporter{}
	p := &correlation.CorrelatingProcessor{Correlator: correlation.New(30 * time.Second), Logger: logger, Exporter: exporter}

	event := model.Event{Type: "network.connect", Timestamp: time.Now(), Process: &model.Process{PID: 100, Command: "curl"}}
	if err := p.Process(context.Background(), event); err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}

	if len(exporter.spans) != 1 {
		t.Fatalf("len(exporter.spans) = %d, want 1", len(exporter.spans))
	}
	if exporter.spans[0].Name != "network.connect" {
		t.Errorf("exported span Name = %q, want %q", exporter.spans[0].Name, "network.connect")
	}
}

func TestCorrelatingProcessor_NilExporterIsSkippedSafely(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	p := &correlation.CorrelatingProcessor{Correlator: correlation.New(30 * time.Second), Logger: logger} // Exporter left nil

	event := model.Event{Type: "network.connect", Timestamp: time.Now(), Process: &model.Process{PID: 100, Command: "curl"}}
	if err := p.Process(context.Background(), event); err != nil {
		t.Fatalf("Process() with a nil Exporter returned error: %v", err)
	}
}
