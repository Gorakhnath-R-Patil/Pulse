package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/socket"
)

// fakeSocketLoader mirrors fakeProcessLoader in process_test.go — see
// its doc comment for the block-channel rationale.
type fakeSocketLoader struct {
	loadErr     error
	attachErr   error
	closeErr    error
	events      []socket.CloseEvent
	terminalErr error
	block       chan struct{}

	i int
}

func (f *fakeSocketLoader) Load() error   { return f.loadErr }
func (f *fakeSocketLoader) Attach() error { return f.attachErr }

func (f *fakeSocketLoader) Close() error {
	if f.block != nil {
		select {
		case <-f.block:
		default:
			close(f.block)
		}
	}
	return f.closeErr
}

func (f *fakeSocketLoader) Read() (socket.CloseEvent, error) {
	if f.i < len(f.events) {
		e := f.events[f.i]
		f.i++
		return e, nil
	}
	if f.block != nil {
		<-f.block
	}
	return socket.CloseEvent{}, f.terminalErr
}

func TestSocketSource_Read_NormalizesEvent(t *testing.T) {
	fake := &fakeSocketLoader{events: []socket.CloseEvent{
		{PID: 100, Comm: "curl", SourcePort: 51000, DestPort: 443, BytesSent: 5, BytesReceived: 1024},
	}}
	src := socketSource{loader: fake, nodeName: "pulse-node-1"}

	event, err := src.Read()
	if err != nil {
		t.Fatalf("Read() returned error: %v", err)
	}
	if event.Type != "network.close" {
		t.Errorf("Type = %q, want %q", event.Type, "network.close")
	}
	if event.Network == nil || event.Network.BytesSent != 5 || event.Network.BytesReceived != 1024 {
		t.Errorf("Network = %+v, want BytesSent=5 BytesReceived=1024", event.Network)
	}
}

func TestSocketSource_Read_PropagatesLoaderError(t *testing.T) {
	wantErr := errors.New("read failed")
	src := socketSource{loader: &fakeSocketLoader{terminalErr: wantErr}, nodeName: "pulse-node-1"}

	_, err := src.Read()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Read() error = %v, want %v", err, wantErr)
	}
}

func TestSocketPipeline_LogsEventsEndToEnd(t *testing.T) {
	app, buf := testApp()
	fake := &fakeSocketLoader{
		events: []socket.CloseEvent{
			{PID: 100, Comm: "curl", SourcePort: 51000, DestPort: 443, BytesSent: 5},
		},
		block: make(chan struct{}),
	}

	p := app.newSocketPipeline(fake)

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

	if !strings.Contains(buf.String(), "network.close") {
		t.Errorf("log output missing the event type: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"bytes_sent":5`) {
		t.Errorf("log output missing the byte count: %s", buf.String())
	}

	cancel()
	fake.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pipeline did not shut down after the loader was closed")
	}
}
