package correlation_test

import (
	"sync"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/correlation"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

func eventAt(pid int32, command string, typ string, t time.Time) model.Event {
	return model.Event{
		Type:      typ,
		Timestamp: t,
		Host:      "pulse-node-1",
		Process:   &model.Process{PID: pid, Command: command},
	}
}

func TestObserve_FirstEventStartsNewTrace(t *testing.T) {
	c := correlation.New(30 * time.Second)
	span := c.Observe(eventAt(100, "curl", "network.connect", time.Now()))

	if span.TraceID == "" {
		t.Error("TraceID is empty, want a generated trace ID")
	}
	if span.ParentSpanID != "" {
		t.Errorf("ParentSpanID = %q, want empty for the first event of a new process", span.ParentSpanID)
	}
	if span.Name != "network.connect" {
		t.Errorf("Name = %q, want %q", span.Name, "network.connect")
	}
	if span.Service != "curl" {
		t.Errorf("Service = %q, want %q", span.Service, "curl")
	}
}

func TestObserve_ChainsWithinWindow(t *testing.T) {
	c := correlation.New(30 * time.Second)
	t0 := time.Now()

	first := c.Observe(eventAt(100, "curl", "dns.query", t0))
	second := c.Observe(eventAt(100, "curl", "network.connect", t0.Add(time.Second)))
	third := c.Observe(eventAt(100, "curl", "network.close", t0.Add(2*time.Second)))

	if second.TraceID != first.TraceID || third.TraceID != first.TraceID {
		t.Fatalf("TraceIDs = %v, %v, %v, want all equal", first.TraceID, second.TraceID, third.TraceID)
	}
	if second.ParentSpanID != first.SpanID {
		t.Errorf("second.ParentSpanID = %q, want %q (first.SpanID)", second.ParentSpanID, first.SpanID)
	}
	if third.ParentSpanID != second.SpanID {
		t.Errorf("third.ParentSpanID = %q, want %q (second.SpanID)", third.ParentSpanID, second.SpanID)
	}
}

func TestObserve_GapLongerThanWindowStartsNewTrace(t *testing.T) {
	c := correlation.New(30 * time.Second)
	t0 := time.Now()

	first := c.Observe(eventAt(100, "curl", "network.connect", t0))
	second := c.Observe(eventAt(100, "curl", "network.close", t0.Add(31*time.Second)))

	if second.TraceID == first.TraceID {
		t.Error("TraceID unchanged across a gap longer than the window, want a new trace")
	}
	if second.ParentSpanID != "" {
		t.Errorf("ParentSpanID = %q, want empty (this starts a new trace)", second.ParentSpanID)
	}
}

func TestObserve_DifferentPIDsDontMix(t *testing.T) {
	c := correlation.New(30 * time.Second)
	t0 := time.Now()

	a := c.Observe(eventAt(100, "curl", "network.connect", t0))
	b := c.Observe(eventAt(200, "nginx", "network.connect", t0.Add(time.Millisecond)))

	if a.TraceID == b.TraceID {
		t.Error("two different processes' first events share a TraceID, want distinct traces")
	}

	// pid 100's own second event still chains correctly despite pid
	// 200's event happening in between.
	aAgain := c.Observe(eventAt(100, "curl", "network.close", t0.Add(2*time.Millisecond)))
	if aAgain.TraceID != a.TraceID || aAgain.ParentSpanID != a.SpanID {
		t.Errorf("pid 100's second event = %+v, want it chained onto its own first event, unaffected by pid 200's", aAgain)
	}
}

func TestObserve_NoProcessGetsOwnTrace(t *testing.T) {
	c := correlation.New(30 * time.Second)
	event := model.Event{Type: "http.request", Timestamp: time.Now(), Host: "pulse-node-1"}

	span := c.Observe(event)

	if span.TraceID == "" {
		t.Error("TraceID is empty, want a generated trace ID even with no Process")
	}
	if span.ParentSpanID != "" {
		t.Errorf("ParentSpanID = %q, want empty", span.ParentSpanID)
	}
	if span.Service != "unknown" {
		t.Errorf("Service = %q, want %q", span.Service, "unknown")
	}
}

func TestObserve_ServiceLabelPrefersContainerID(t *testing.T) {
	c := correlation.New(30 * time.Second)
	event := eventAt(100, "curl", "network.connect", time.Now())
	event.Process.Container = &model.Container{ID: "abc123"}

	span := c.Observe(event)
	if span.Service != "abc123" {
		t.Errorf("Service = %q, want the container ID %q", span.Service, "abc123")
	}
}

func TestObserve_ServiceLabelFallsBackToCommand(t *testing.T) {
	c := correlation.New(30 * time.Second)
	span := c.Observe(eventAt(100, "curl", "network.connect", time.Now()))
	if span.Service != "curl" {
		t.Errorf("Service = %q, want the command name %q", span.Service, "curl")
	}
}

func TestObserve_AttributesCarriedOntoSpan(t *testing.T) {
	c := correlation.New(30 * time.Second)
	event := eventAt(100, "curl", "dns.query", time.Now())
	event.Attributes = map[string]string{"dns.name": "example.com"}

	span := c.Observe(event)
	if span.Attributes["dns.name"] != "example.com" {
		t.Errorf(`Attributes["dns.name"] = %q, want "example.com"`, span.Attributes["dns.name"])
	}
}

// TestObserve_ConcurrentUse exercises Correlator from many goroutines
// at once. It doesn't assert much about the resulting spans beyond "no
// panic and every span comes back well-formed" — the real point is
// giving the race detector (run in CI; this dev environment can't run
// -race locally, see docs/development/getting-started.md) something to
// check Correlator's internal locking against.
func TestObserve_ConcurrentUse(t *testing.T) {
	c := correlation.New(30 * time.Second)

	var wg sync.WaitGroup
	for pid := int32(0); pid < 20; pid++ {
		wg.Add(1)
		go func(pid int32) {
			defer wg.Done()
			t0 := time.Now()
			for i := 0; i < 50; i++ {
				span := c.Observe(eventAt(pid, "worker", "network.connect", t0.Add(time.Duration(i)*time.Millisecond)))
				if span.TraceID == "" || span.SpanID == "" {
					t.Errorf("Observe() returned a span with an empty TraceID or SpanID: %+v", span)
				}
			}
		}(pid)
	}
	wg.Wait()
}
