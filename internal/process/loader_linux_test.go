//go:build linux

package process_test

import (
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/cilium/ebpf/ringbuf"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/process"
)

// requireRoot skips tests that need to actually load a BPF program —
// see internal/ebpf/loader_linux_test.go's identical helper for why.
func requireRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("loading eBPF programs requires root (or equivalent capabilities); run with sudo to exercise this test")
	}
}

func TestLoader_AttachBeforeLoadFails(t *testing.T) {
	l := process.NewLoader()
	defer l.Close()

	if err := l.Attach(); !errors.Is(err, process.ErrNotLoaded) {
		t.Errorf("Attach() before Load(): error = %v, want it to wrap ErrNotLoaded", err)
	}
}

func TestLoader_ReadBeforeAttachFails(t *testing.T) {
	l := process.NewLoader()
	defer l.Close()

	if _, err := l.Read(); !errors.Is(err, process.ErrNotLoaded) {
		t.Errorf("Read() before Attach(): error = %v, want it to wrap ErrNotLoaded", err)
	}
}

func TestLoader_ObservesRealProcessStart(t *testing.T) {
	requireRoot(t)

	l := process.NewLoader()
	defer l.Close()

	if err := l.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if err := l.Attach(); err != nil {
		t.Fatalf("Attach() error: %v", err)
	}

	before := time.Now()

	go func() {
		_ = exec.Command("/bin/true").Run()
	}()

	type result struct {
		event process.ProcessEvent
		err   error
	}
	done := make(chan result, 1)
	go func() {
		// /bin/true execs and exits almost immediately, so the first
		// event read back could be either kind depending on scheduling;
		// either is proof the pipeline works end to end.
		event, err := l.Read()
		done <- result{event, err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Read() error: %v", r.err)
		}
		if r.event.PID == 0 {
			t.Error("PID = 0, want a real process ID")
		}
		if r.event.Timestamp.Before(before.Add(-time.Second)) || r.event.Timestamp.After(time.Now().Add(time.Second)) {
			t.Errorf("Timestamp = %v, want it close to now (%v)", r.event.Timestamp, before)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not receive a process event within 5s of exec'ing a process")
	}
}

func TestLoader_CloseInterruptsRead(t *testing.T) {
	requireRoot(t)

	l := process.NewLoader()
	if err := l.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if err := l.Attach(); err != nil {
		t.Fatalf("Attach() error: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := l.Read()
		done <- err
	}()

	time.Sleep(100 * time.Millisecond)
	if err := l.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, ringbuf.ErrClosed) {
			t.Errorf("Read() error = %v, want it to wrap ringbuf.ErrClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Read() did not return within 2s of Close()")
	}
}
