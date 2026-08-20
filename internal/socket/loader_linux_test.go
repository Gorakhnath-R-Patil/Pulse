//go:build linux

package socket_test

import (
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/cilium/ebpf/ringbuf"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/socket"
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
	l := socket.NewLoader()
	defer l.Close()

	if err := l.Attach(); !errors.Is(err, socket.ErrNotLoaded) {
		t.Errorf("Attach() before Load(): error = %v, want it to wrap ErrNotLoaded", err)
	}
}

func TestLoader_ReadBeforeAttachFails(t *testing.T) {
	l := socket.NewLoader()
	defer l.Close()

	if _, err := l.Read(); !errors.Is(err, socket.ErrNotLoaded) {
		t.Errorf("Read() before Attach(): error = %v, want it to wrap ErrNotLoaded", err)
	}
}

// TestLoader_ObservesRealTCPClose is the integration test Day 06 calls
// for: it opens a real TCP connection, writes a known number of bytes,
// closes it, and confirms the resulting close event's endpoints and
// byte count match reality — not just internal self-consistency. See
// internal/network's identical-in-spirit test for the same rationale
// applied to connect events.
func TestLoader_ObservesRealTCPClose(t *testing.T) {
	requireRoot(t)

	l := socket.NewLoader()
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
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 64)
		conn.Read(buf) //nolint:errcheck // best-effort drain; the test only cares about the client side's own close event
	}()

	conn, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		t.Fatalf("net.Dial() error: %v", err)
	}

	const payload = "hello"
	if _, err := conn.Write([]byte(payload)); err != nil {
		t.Fatalf("conn.Write() error: %v", err)
	}

	wantLocalPort := conn.LocalAddr().(*net.TCPAddr).Port
	wantRemotePort := conn.RemoteAddr().(*net.TCPAddr).Port
	wantPID := uint32(os.Getpid())

	if err := conn.Close(); err != nil {
		t.Fatalf("conn.Close() error: %v", err)
	}

	type result struct {
		event socket.CloseEvent
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
			// tcp_close fires for every TCP socket closed on the host;
			// keep going until it's ours.
			if event.PID == wantPID && int(event.SourcePort) == wantLocalPort {
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
		if r.event.DestAddr.String() != "127.0.0.1" {
			t.Errorf("DestAddr = %v, want 127.0.0.1", r.event.DestAddr)
		}
		if int(r.event.DestPort) != wantRemotePort {
			t.Errorf("DestPort = %d, want %d (the real listener port)", r.event.DestPort, wantRemotePort)
		}
		if r.event.BytesSent != uint64(len(payload)) {
			t.Errorf("BytesSent = %d, want %d (the real payload length)", r.event.BytesSent, len(payload))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not observe our own connection's close within 5s")
	}
}

func TestLoader_CloseInterruptsRead(t *testing.T) {
	requireRoot(t)

	l := socket.NewLoader()
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
