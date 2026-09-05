//go:build linux

package httpvis_test

import (
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/cilium/ebpf/ringbuf"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/httpvis"
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
	l := httpvis.NewLoader()
	defer l.Close()

	if err := l.Attach(); !errors.Is(err, httpvis.ErrNotLoaded) {
		t.Errorf("Attach() before Load(): error = %v, want it to wrap ErrNotLoaded", err)
	}
}

func TestLoader_ReadBeforeAttachFails(t *testing.T) {
	l := httpvis.NewLoader()
	defer l.Close()

	if _, err := l.Read(); !errors.Is(err, httpvis.ErrNotLoaded) {
		t.Errorf("Read() before Attach(): error = %v, want it to wrap ErrNotLoaded", err)
	}
}

// TestLoader_ObservesRealHTTPRequest is the integration test: it starts
// a real TCP listener, writes a real HTTP request line directly to a
// dialed connection (a plain net.Conn.Write, which on Linux is a plain
// write(2) syscall — the exact thing http_visibility.c hooks), and
// confirms the resulting event's method and path match what was
// actually sent.
func TestLoader_ObservesRealHTTPRequest(t *testing.T) {
	requireRoot(t)

	l := httpvis.NewLoader()
	defer l.Close()

	if err := l.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if err := l.Attach(); err != nil {
		t.Fatalf("Attach() error: %v", err)
	}

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err == nil {
			// Drain whatever's written so the writer isn't blocked.
			buf := make([]byte, 4096)
			_, _ = conn.Read(buf)
			conn.Close()
		}
	}()

	conn, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		t.Fatalf("net.Dial() error: %v", err)
	}
	defer conn.Close()

	wantPID := uint32(os.Getpid())
	request := "GET /pulse-integration-test HTTP/1.1\r\nHost: example.com\r\n\r\n"

	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("conn.Write() error: %v", err)
	}

	type result struct {
		event httpvis.HTTPEvent
		err   error
	}
	found := make(chan result, 1)
	go func() {
		for {
			event, err := l.Read()
			if err != nil {
				found <- result{err: err}
				return
			}
			// Some other process's HTTP-shaped write on this shared
			// runner could be read first; keep going until it's ours.
			if event.PID == wantPID && event.Path == "/pulse-integration-test" {
				found <- result{event: event}
				return
			}
		}
	}()

	select {
	case r := <-found:
		if r.err != nil {
			t.Fatalf("Read() error: %v", r.err)
		}
		if r.event.Method != "GET" {
			t.Errorf("Method = %q, want %q", r.event.Method, "GET")
		}
		if r.event.IsResponse {
			t.Error("IsResponse = true, want false for a request")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not observe our own HTTP write within 5s")
	}
}

func TestLoader_CloseInterruptsRead(t *testing.T) {
	requireRoot(t)

	l := httpvis.NewLoader()
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
