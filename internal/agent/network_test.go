package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/network"
)

// fakeNetworkLoader mirrors fakeProcessLoader in process_test.go — see
// its doc comment for the block-channel rationale.
type fakeNetworkLoader struct {
	loadErr     error
	attachErr   error
	events      []network.ConnectEvent
	terminalErr error
	block       chan struct{}

	i int
}

func (f *fakeNetworkLoader) Load() error   { return f.loadErr }
func (f *fakeNetworkLoader) Attach() error { return f.attachErr }

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

func TestStartNetworkTelemetry_LoadFails(t *testing.T) {
	app, _ := testApp()
	wantErr := errors.New("load failed")
	fake := &fakeNetworkLoader{loadErr: wantErr}

	err := app.startNetworkTelemetry(context.Background(), fake)
	if !errors.Is(err, wantErr) {
		t.Fatalf("startNetworkTelemetry() error = %v, want %v", err, wantErr)
	}
}

func TestStartNetworkTelemetry_AttachFails(t *testing.T) {
	app, _ := testApp()
	wantErr := errors.New("attach failed")
	fake := &fakeNetworkLoader{attachErr: wantErr}

	err := app.startNetworkTelemetry(context.Background(), fake)
	if !errors.Is(err, wantErr) {
		t.Fatalf("startNetworkTelemetry() error = %v, want %v", err, wantErr)
	}
}

func TestStartNetworkTelemetry_SuccessLogsActiveAndStartsWatcher(t *testing.T) {
	app, buf := testApp()
	fake := &fakeNetworkLoader{block: make(chan struct{})}

	if err := app.startNetworkTelemetry(context.Background(), fake); err != nil {
		t.Fatalf("startNetworkTelemetry() returned error: %v", err)
	}

	if !strings.Contains(buf.String(), "network connection telemetry active") {
		t.Errorf("log output missing startup confirmation: %s", buf.String())
	}
}

func TestWatchNetworkEvents_LogsEachEvent(t *testing.T) {
	app, buf := testApp()
	fake := &fakeNetworkLoader{
		events: []network.ConnectEvent{
			{PID: 100, Comm: "curl", SourcePort: 51000, DestPort: 443, Success: true},
		},
		terminalErr: errors.New("simulated read failure"),
	}

	app.watchNetworkEvents(context.Background(), fake)

	out := buf.String()
	if !strings.Contains(out, `"pid":100`) {
		t.Errorf("log output missing the observed event's pid: %s", out)
	}
	if !strings.Contains(out, "network.connect") {
		t.Errorf("log output missing the event type: %s", out)
	}
	if !strings.Contains(out, `"success":true`) {
		t.Errorf("log output missing the success flag: %s", out)
	}
}

func TestWatchNetworkEvents_UnexpectedReadFailureIsWarned(t *testing.T) {
	app, buf := testApp()
	fake := &fakeNetworkLoader{terminalErr: errors.New("boom")}

	app.watchNetworkEvents(context.Background(), fake)

	if !strings.Contains(buf.String(), "network connection telemetry read failed") {
		t.Errorf("log output missing the read-failure warning: %s", buf.String())
	}
}

func TestWatchNetworkEvents_ShutdownReadFailureIsSilent(t *testing.T) {
	app, buf := testApp()
	fake := &fakeNetworkLoader{terminalErr: errors.New("reader closed")}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	app.watchNetworkEvents(ctx, fake)

	if strings.Contains(buf.String(), "read failed") {
		t.Errorf("expected no warning for a shutdown-triggered read failure, got: %s", buf.String())
	}
}
