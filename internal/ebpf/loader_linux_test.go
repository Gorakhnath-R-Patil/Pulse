//go:build linux

package ebpf_test

import (
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/cilium/ebpf/ringbuf"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/ebpf"
)

// requireRoot skips tests that need to actually load a BPF program: on
// most distributions (kernel.unprivileged_bpf_disabled != 0), that
// requires root or CAP_BPF+CAP_PERFMON. CI runs these under sudo; a
// local `go test ./...` run as a normal user skips them rather than
// failing, matching cilium/ebpf's own test suite convention.
func requireRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("loading eBPF programs requires root (or equivalent capabilities); run with sudo to exercise this test")
	}
}

func TestCheckSupport_Linux(t *testing.T) {
	// CheckSupport can fail either because the kernel genuinely lacks a
	// feature, or because probing itself requires privilege this
	// process doesn't have — the two aren't reliably distinguishable
	// from here, so this only documents that the call completes
	// without panicking, not that it must succeed unprivileged.
	if err := ebpf.CheckSupport(); err != nil {
		t.Skipf("CheckSupport() failed (unsupported kernel or insufficient privilege): %v", err)
	}
}

func TestLoader_AttachBeforeLoadFails(t *testing.T) {
	l := ebpf.NewLoader()
	defer l.Close()

	if err := l.Attach(); !errors.Is(err, ebpf.ErrNotLoaded) {
		t.Errorf("Attach() before Load(): error = %v, want it to wrap ErrNotLoaded", err)
	}
}

func TestLoader_ReadBeforeAttachFails(t *testing.T) {
	l := ebpf.NewLoader()
	defer l.Close()

	if _, err := l.Read(); !errors.Is(err, ebpf.ErrNotLoaded) {
		t.Errorf("Read() before Attach(): error = %v, want it to wrap ErrNotLoaded", err)
	}
}

func TestLoader_FullLifecycle(t *testing.T) {
	requireRoot(t)

	l := ebpf.NewLoader()
	defer l.Close()

	if err := l.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if err := l.Attach(); err != nil {
		t.Fatalf("Attach() error: %v", err)
	}

	// Trigger the tracepoint: exec'ing any real binary fires
	// syscalls:sys_enter_execve.
	go func() {
		_ = exec.Command("/bin/true").Run()
	}()

	type result struct {
		event ebpf.HeartbeatEvent
		err   error
	}
	done := make(chan result, 1)
	go func() {
		event, err := l.Read()
		done <- result{event, err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Read() error: %v", r.err)
		}
		if r.event.TimestampNS == 0 {
			t.Error("TimestampNS = 0, want a real kernel timestamp")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not receive a heartbeat event within 5s of exec'ing a process")
	}
}

func TestLoader_CloseInterruptsRead(t *testing.T) {
	requireRoot(t)

	l := ebpf.NewLoader()
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

	// Give Read a moment to actually start blocking before closing.
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
