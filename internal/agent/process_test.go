package agent

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/process"
)

// syncBuffer is a mutex-protected bytes.Buffer. The *EndToEnd tests in
// this package (and in network_test.go, socket_test.go) run a pipeline
// in a background goroutine and poll its log output from the test
// goroutine while it's still writing — a plain bytes.Buffer is not
// safe for that, so every test capturing a running pipeline's log
// output uses this instead.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// fakeProcessLoader is a processLoader test double: it never touches a
// real kernel, so these tests exercise this package's wiring logic
// (does processSource normalize correctly, does the resulting pipeline
// actually log what it reads) without needing Linux or root, unlike
// internal/process's own loader tests.
//
// block, if non-nil, makes Read block forever once events is exhausted
// instead of returning terminalErr — used so a pipeline built around
// this fake can be left running without ever reaching a log call that
// would race a test's later buf.String() read.
type fakeProcessLoader struct {
	loadErr     error
	attachErr   error
	closeErr    error
	events      []process.ProcessEvent
	terminalErr error
	block       chan struct{}

	i int
}

func (f *fakeProcessLoader) Load() error   { return f.loadErr }
func (f *fakeProcessLoader) Attach() error { return f.attachErr }

// Close mirrors a real Loader: it unblocks a Read call parked on block,
// the same way closing the underlying ring buffer reader would.
func (f *fakeProcessLoader) Close() error {
	if f.block != nil {
		select {
		case <-f.block:
		default:
			close(f.block)
		}
	}
	return f.closeErr
}

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

func testApp() (*App, *syncBuffer) {
	buf := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(buf, nil))
	return New(config.AgentConfig{NodeName: "pulse-node-1"}, logger), buf
}

func TestProcessSource_Read_NormalizesEvent(t *testing.T) {
	fake := &fakeProcessLoader{events: []process.ProcessEvent{
		{PID: 100, PPID: 1, Comm: "sh", Type: process.EventStart},
	}}
	src := processSource{loader: fake, nodeName: "pulse-node-1"}

	event, err := src.Read()
	if err != nil {
		t.Fatalf("Read() returned error: %v", err)
	}
	if event.Type != "process.start" {
		t.Errorf("Type = %q, want %q", event.Type, "process.start")
	}
	if event.Host != "pulse-node-1" {
		t.Errorf("Host = %q, want %q", event.Host, "pulse-node-1")
	}
	if event.Process == nil || event.Process.PID != 100 || event.Process.Command != "sh" {
		t.Errorf("Process = %+v, want PID=100 Command=sh", event.Process)
	}
}

func TestProcessSource_Read_ExitEventNeverResolvesExecutable(t *testing.T) {
	// Deterministic and platform-independent, unlike asserting that
	// resolution *succeeds* for a start event (which depends on a real,
	// currently-running PID and is already covered by
	// internal/process's own ResolveExecutable tests): an exit event's
	// process is gone by definition, so Read must not even attempt it.
	fake := &fakeProcessLoader{events: []process.ProcessEvent{
		{PID: 100, Comm: "sh", Type: process.EventExit},
	}}
	src := processSource{loader: fake, nodeName: "pulse-node-1"}

	event, err := src.Read()
	if err != nil {
		t.Fatalf("Read() returned error: %v", err)
	}
	if event.Process.Executable != "" {
		t.Errorf("Process.Executable = %q, want empty for an exit event", event.Process.Executable)
	}
}

func TestProcessSource_Read_PropagatesLoaderError(t *testing.T) {
	wantErr := errors.New("read failed")
	src := processSource{loader: &fakeProcessLoader{terminalErr: wantErr}, nodeName: "pulse-node-1"}

	_, err := src.Read()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Read() error = %v, want %v", err, wantErr)
	}
}

func TestProcessPipeline_LogsEventsEndToEnd(t *testing.T) {
	app, buf := testApp()
	fake := &fakeProcessLoader{
		events: []process.ProcessEvent{
			{PID: 100, PPID: 1, Comm: "sh", Type: process.EventStart},
		},
		block: make(chan struct{}), // keep the pipeline alive without racing buf after the one event
	}

	p := app.newProcessPipeline(fake)

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

	if !strings.Contains(buf.String(), "process.start") {
		t.Errorf("log output missing the event type: %s", buf.String())
	}

	// Mirrors App.Run's real shutdown sequence: cancel ctx, then close
	// the loader so its blocked Read unblocks — ctx cancellation alone
	// does not interrupt a Read call already in progress, matching the
	// real Loader's contract (see internal/ebpf's Loader.Close).
	cancel()
	fake.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pipeline did not shut down after the loader was closed")
	}
}
