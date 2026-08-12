package agent

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/process"
)

// fakeProcessLoader is a processLoader test double: it never touches a
// real kernel, so these tests exercise App's wiring logic (does it call
// Load/Attach/Read correctly, does it log what it should, does it treat
// a shutdown-triggered read failure differently from an unexpected one)
// without needing Linux or root, unlike internal/process's own loader
// tests.
//
// block, if non-nil, makes Read block forever once events is exhausted
// instead of returning terminalErr — used to test the goroutine
// startProcessDiscovery spawns without racing on what it logs
// afterward: a Read that never returns never writes to the test's log
// buffer, so asserting on that buffer immediately is safe.
type fakeProcessLoader struct {
	loadErr     error
	attachErr   error
	events      []process.ProcessEvent
	terminalErr error
	block       chan struct{}

	i int
}

func (f *fakeProcessLoader) Load() error   { return f.loadErr }
func (f *fakeProcessLoader) Attach() error { return f.attachErr }

func (f *fakeProcessLoader) Read() (process.ProcessEvent, error) {
	if f.i < len(f.events) {
		e := f.events[f.i]
		f.i++
		return e, nil
	}
	if f.block != nil {
		<-f.block // never closed by these tests: blocks forever
	}
	return process.ProcessEvent{}, f.terminalErr
}

func testApp() (*App, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	return New(config.AgentConfig{NodeName: "pulse-node-1"}, logger), &buf
}

func TestStartProcessDiscovery_LoadFails(t *testing.T) {
	app, _ := testApp()
	wantErr := errors.New("load failed")
	fake := &fakeProcessLoader{loadErr: wantErr}

	err := app.startProcessDiscovery(context.Background(), fake)
	if !errors.Is(err, wantErr) {
		t.Fatalf("startProcessDiscovery() error = %v, want %v", err, wantErr)
	}
}

func TestStartProcessDiscovery_AttachFails(t *testing.T) {
	app, _ := testApp()
	wantErr := errors.New("attach failed")
	fake := &fakeProcessLoader{attachErr: wantErr}

	err := app.startProcessDiscovery(context.Background(), fake)
	if !errors.Is(err, wantErr) {
		t.Fatalf("startProcessDiscovery() error = %v, want %v", err, wantErr)
	}
}

func TestStartProcessDiscovery_SuccessLogsActiveAndStartsWatcher(t *testing.T) {
	app, buf := testApp()
	// Read blocks forever on the very first call: the spawned watcher
	// goroutine never reaches a log call, so it can never race with
	// this test's buf.String() below.
	fake := &fakeProcessLoader{block: make(chan struct{})}

	if err := app.startProcessDiscovery(context.Background(), fake); err != nil {
		t.Fatalf("startProcessDiscovery() returned error: %v", err)
	}

	if !strings.Contains(buf.String(), "process discovery active") {
		t.Errorf("log output missing startup confirmation: %s", buf.String())
	}
}

func TestWatchProcessEvents_LogsEachEvent(t *testing.T) {
	app, buf := testApp()
	fake := &fakeProcessLoader{
		events: []process.ProcessEvent{
			{PID: 100, PPID: 1, Comm: "sh", Type: process.EventStart},
		},
		terminalErr: errors.New("simulated read failure"),
	}

	// Called synchronously (no `go`), so there is nothing to
	// synchronize on: watchProcessEvents only returns after it has
	// finished writing everything it's going to write.
	app.watchProcessEvents(context.Background(), fake)

	out := buf.String()
	if !strings.Contains(out, `"pid":100`) {
		t.Errorf("log output missing the observed event's pid: %s", out)
	}
	if !strings.Contains(out, "process.start") {
		t.Errorf("log output missing the event type: %s", out)
	}
}

func TestWatchProcessEvents_UnexpectedReadFailureIsWarned(t *testing.T) {
	app, buf := testApp()
	fake := &fakeProcessLoader{terminalErr: errors.New("boom")}

	app.watchProcessEvents(context.Background(), fake)

	if !strings.Contains(buf.String(), "process discovery read failed") {
		t.Errorf("log output missing the read-failure warning: %s", buf.String())
	}
}

func TestWatchProcessEvents_ShutdownReadFailureIsSilent(t *testing.T) {
	app, buf := testApp()
	fake := &fakeProcessLoader{terminalErr: errors.New("reader closed")}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // simulate Run's shutdown path: ctx is already done when Read fails

	app.watchProcessEvents(ctx, fake)

	if strings.Contains(buf.String(), "read failed") {
		t.Errorf("expected no warning for a shutdown-triggered read failure, got: %s", buf.String())
	}
}
