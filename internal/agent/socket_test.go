package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/socket"
)

// fakeSocketLoader mirrors fakeProcessLoader/fakeNetworkLoader — see
// fakeProcessLoader's doc comment in process_test.go for the block-
// channel rationale.
type fakeSocketLoader struct {
	loadErr     error
	attachErr   error
	events      []socket.CloseEvent
	terminalErr error
	block       chan struct{}

	i int
}

func (f *fakeSocketLoader) Load() error   { return f.loadErr }
func (f *fakeSocketLoader) Attach() error { return f.attachErr }

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

func TestStartSocketTelemetry_LoadFails(t *testing.T) {
	app, _ := testApp()
	wantErr := errors.New("load failed")
	fake := &fakeSocketLoader{loadErr: wantErr}

	err := app.startSocketTelemetry(context.Background(), fake)
	if !errors.Is(err, wantErr) {
		t.Fatalf("startSocketTelemetry() error = %v, want %v", err, wantErr)
	}
}

func TestStartSocketTelemetry_AttachFails(t *testing.T) {
	app, _ := testApp()
	wantErr := errors.New("attach failed")
	fake := &fakeSocketLoader{attachErr: wantErr}

	err := app.startSocketTelemetry(context.Background(), fake)
	if !errors.Is(err, wantErr) {
		t.Fatalf("startSocketTelemetry() error = %v, want %v", err, wantErr)
	}
}

func TestStartSocketTelemetry_SuccessLogsActive(t *testing.T) {
	app, buf := testApp()
	fake := &fakeSocketLoader{block: make(chan struct{})}

	if err := app.startSocketTelemetry(context.Background(), fake); err != nil {
		t.Fatalf("startSocketTelemetry() returned error: %v", err)
	}

	if !strings.Contains(buf.String(), "socket data telemetry active") {
		t.Errorf("log output missing startup confirmation: %s", buf.String())
	}
}

func TestLogSocketEvents_LogsEachEvent(t *testing.T) {
	app, buf := testApp()
	in := make(chan socket.CloseEvent, 1)
	in <- socket.CloseEvent{PID: 100, Comm: "curl", SourcePort: 51000, DestPort: 443, BytesSent: 5}
	close(in)

	app.logSocketEvents(in)

	out := buf.String()
	if !strings.Contains(out, `"pid":100`) {
		t.Errorf("log output missing the observed event's pid: %s", out)
	}
	if !strings.Contains(out, `"bytes_sent":5`) {
		t.Errorf("log output missing the byte count: %s", out)
	}
}

// TestReadSocketEvents_DropsWhenBufferFull is the test for this day's
// "introduce bounded buffering" requirement: called synchronously
// (no consumer draining concurrently), so a channel smaller than the
// event count deterministically forces a drop, without needing to
// approach the real socketEventBufferSize constant.
func TestReadSocketEvents_DropsWhenBufferFull(t *testing.T) {
	app, buf := testApp()
	fake := &fakeSocketLoader{
		events:      []socket.CloseEvent{{PID: 1}, {PID: 2}, {PID: 3}},
		terminalErr: errors.New("simulated read failure"),
	}

	out := make(chan socket.CloseEvent, 1) // smaller than len(fake.events): forces a drop
	app.readSocketEvents(context.Background(), fake, out)

	var received int
	for range out { // readSocketEvents closes out when done
		received++
	}
	if received == 0 {
		t.Error("received 0 events, want at least 1 to have made it through the buffer")
	}
	if received >= len(fake.events) {
		t.Errorf("received %d of %d events, want fewer (some should have been dropped)", received, len(fake.events))
	}

	if !strings.Contains(buf.String(), "socket telemetry buffer full, dropping event") {
		t.Errorf("log output missing the drop warning: %s", buf.String())
	}
}

func TestReadSocketEvents_UnexpectedReadFailureIsWarned(t *testing.T) {
	app, buf := testApp()
	fake := &fakeSocketLoader{terminalErr: errors.New("boom")}

	out := make(chan socket.CloseEvent, 1)
	app.readSocketEvents(context.Background(), fake, out)

	if !strings.Contains(buf.String(), "socket telemetry read failed") {
		t.Errorf("log output missing the read-failure warning: %s", buf.String())
	}
}

func TestReadSocketEvents_ShutdownReadFailureIsSilent(t *testing.T) {
	app, buf := testApp()
	fake := &fakeSocketLoader{terminalErr: errors.New("reader closed")}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	out := make(chan socket.CloseEvent, 1)
	app.readSocketEvents(ctx, fake, out)

	if strings.Contains(buf.String(), "read failed") {
		t.Errorf("expected no warning for a shutdown-triggered read failure, got: %s", buf.String())
	}
}
