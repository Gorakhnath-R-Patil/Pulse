package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/httpvis"
)

// fakeHTTPVisLoader mirrors fakeProcessLoader in process_test.go — see
// its doc comment for the block-channel rationale.
type fakeHTTPVisLoader struct {
	loadErr     error
	attachErr   error
	closeErr    error
	events      []httpvis.HTTPEvent
	terminalErr error
	block       chan struct{}

	i int
}

func (f *fakeHTTPVisLoader) Load() error   { return f.loadErr }
func (f *fakeHTTPVisLoader) Attach() error { return f.attachErr }

func (f *fakeHTTPVisLoader) Close() error {
	if f.block != nil {
		select {
		case <-f.block:
		default:
			close(f.block)
		}
	}
	return f.closeErr
}

func (f *fakeHTTPVisLoader) Read() (httpvis.HTTPEvent, error) {
	if f.i < len(f.events) {
		e := f.events[f.i]
		f.i++
		return e, nil
	}
	if f.block != nil {
		<-f.block
		if f.terminalErr != nil {
			return httpvis.HTTPEvent{}, f.terminalErr
		}
		return httpvis.HTTPEvent{}, errFakeLoaderClosed
	}
	return httpvis.HTTPEvent{}, f.terminalErr
}

func TestHTTPVisSource_Read_NormalizesEvent(t *testing.T) {
	fake := &fakeHTTPVisLoader{events: []httpvis.HTTPEvent{
		{PID: 100, Comm: "curl", Size: 42, Method: "GET", Path: "/foo"},
	}}
	src := httpvisSource{loader: fake, nodeName: "pulse-node-1"}

	event, err := src.Read()
	if err != nil {
		t.Fatalf("Read() returned error: %v", err)
	}
	if event.Type != "http.request" {
		t.Errorf("Type = %q, want %q", event.Type, "http.request")
	}
	if event.Attributes["http.method"] != "GET" || event.Attributes["http.path"] != "/foo" {
		t.Errorf("Attributes = %+v, want method=GET path=/foo", event.Attributes)
	}
}

func TestHTTPVisSource_Read_PropagatesLoaderError(t *testing.T) {
	wantErr := errors.New("read failed")
	src := httpvisSource{loader: &fakeHTTPVisLoader{terminalErr: wantErr}, nodeName: "pulse-node-1"}

	_, err := src.Read()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Read() error = %v, want %v", err, wantErr)
	}
}

func TestHTTPVisPipeline_LogsEventsEndToEnd(t *testing.T) {
	app, buf := testApp()
	fake := &fakeHTTPVisLoader{
		events: []httpvis.HTTPEvent{
			{PID: 100, Comm: "curl", Size: 42, Method: "GET", Path: "/foo"},
		},
		block: make(chan struct{}),
	}

	p := app.newHTTPVisPipeline(fake, testCorrelatingProcessor())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	deadline := time.After(2 * time.Second)
	for !strings.Contains(buf.String(), `"pid":100`) {
		select {
		case <-deadline:
			t.Fatalf("log output never contained the observed event's pid: %s", buf.String())
		case <-time.After(time.Millisecond):
		}
	}

	if !strings.Contains(buf.String(), "http.request") {
		t.Errorf("log output missing the event type: %s", buf.String())
	}

	cancel()
	fake.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pipeline did not shut down after the loader was closed")
	}
}
