package agent

import (
	"errors"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// fakeEventSource is a pipeline.EventSource test double returning a
// fixed sequence of events, then a terminal error.
type fakeEventSource struct {
	events      []model.Event
	terminalErr error
	i           int
}

func (f *fakeEventSource) Read() (model.Event, error) {
	if f.i < len(f.events) {
		e := f.events[f.i]
		f.i++
		return e, nil
	}
	return model.Event{}, f.terminalErr
}

func TestContainerEnrichingSource_PropagatesInnerError(t *testing.T) {
	wantErr := errors.New("read failed")
	src := containerEnrichingSource{inner: &fakeEventSource{terminalErr: wantErr}}

	_, err := src.Read()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Read() error = %v, want %v", err, wantErr)
	}
}

func TestContainerEnrichingSource_SkipsEnrichmentWithoutProcess(t *testing.T) {
	// No Process set at all: enrichment must not panic trying to read
	// a PID that doesn't exist, and must leave the event otherwise
	// unchanged.
	event := model.Event{ID: "abc", Type: "test.event", Timestamp: time.Now(), Host: "n1"}
	src := containerEnrichingSource{inner: &fakeEventSource{events: []model.Event{event}}}

	got, err := src.Read()
	if err != nil {
		t.Fatalf("Read() returned error: %v", err)
	}
	if got.Process != nil {
		t.Errorf("Process = %+v, want nil (unchanged from input)", got.Process)
	}
}

func TestContainerEnrichingSource_UnresolvableProcessLeavesContainerUnset(t *testing.T) {
	// PID 0 never resolves to a real container on any platform this
	// runs on (see internal/discovery's identical-reasoning test) — the
	// meaningful assertion is that a resolution failure is silent, not
	// that resolution succeeds, which would require a real container
	// this test can't assume exists.
	event := model.Event{
		ID: "abc", Type: "test.event", Timestamp: time.Now(), Host: "n1",
		Process: &model.Process{PID: 0},
	}
	src := containerEnrichingSource{inner: &fakeEventSource{events: []model.Event{event}}}

	got, err := src.Read()
	if err != nil {
		t.Fatalf("Read() returned error: %v", err)
	}
	if got.Process.Container != nil {
		t.Errorf("Process.Container = %+v, want nil for an unresolvable PID", got.Process.Container)
	}
}
