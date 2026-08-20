package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/network"
)

// fakeNetworkLoader mirrors fakeProcessLoader in process_test.go — see
// its doc comment for the block-channel rationale.
type fakeNetworkLoader struct {
	loadErr     error
	attachErr   error
	closeErr    error
	events      []network.ConnectEvent
	terminalErr error
	block       chan struct{}

	i int
}

func (f *fakeNetworkLoader) Load() error   { return f.loadErr }
func (f *fakeNetworkLoader) Attach() error { return f.attachErr }

func (f *fakeNetworkLoader) Close() error {
	if f.block != nil {
		select {
		case <-f.block:
		default:
			close(f.block)
		}
	}
	return f.closeErr
}

func (f *fakeNetworkLoader) Read() (network.ConnectEvent, error) {
	if f.i < len(f.events) {
		e := f.events[f.i]
		f.i++
		return e, nil
	}
	if f.block != nil {
		<-f.block
	}
	return network.ConnectEvent{}, f.terminalErr
}

func TestNetworkSource_Read_NormalizesEvent(t *testing.T) {
	fake := &fakeNetworkLoader{events: []network.ConnectEvent{
		{PID: 100, Comm: "curl", SourcePort: 51000, DestPort: 443, Success: true},
	}}
	src := networkSource{loader: fake, nodeName: "pulse-node-1"}

	event, err := src.Read()
	if err != nil {
		t.Fatalf("Read() returned error: %v", err)
	}
	if event.Type != "network.connect" {
		t.Errorf("Type = %q, want %q", event.Type, "network.connect")
	}
	if event.Network == nil || event.Network.Source.Port != 51000 || event.Network.Destination.Port != 443 {
		t.Errorf("Network = %+v, want source port 51000, destination port 443", event.Network)
	}
	if event.Attributes["tcp.connect_success"] != "true" {
		t.Errorf(`Attributes["tcp.connect_success"] = %q, want "true"`, event.Attributes["tcp.connect_success"])
	}
}

func TestNetworkSource_Read_PropagatesLoaderError(t *testing.T) {
	wantErr := errors.New("read failed")
	src := networkSource{loader: &fakeNetworkLoader{terminalErr: wantErr}, nodeName: "pulse-node-1"}

	_, err := src.Read()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Read() error = %v, want %v", err, wantErr)
	}
}

func TestNetworkPipeline_LogsEventsEndToEnd(t *testing.T) {
	app, buf := testApp()
	fake := &fakeNetworkLoader{
		events: []network.ConnectEvent{
			{PID: 100, Comm: "curl", SourcePort: 51000, DestPort: 443, Success: true},
		},
		block: make(chan struct{}),
	}

	p := app.newNetworkPipeline(fake)

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

	if !strings.Contains(buf.String(), "network.connect") {
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
