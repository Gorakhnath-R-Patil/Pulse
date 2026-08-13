//go:build linux

package network_test

import (
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/cilium/ebpf/ringbuf"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/network"
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
	l := network.NewLoader()
	defer l.Close()

	if err := l.Attach(); !errors.Is(err, network.ErrNotLoaded) {
		t.Errorf("Attach() before Load(): error = %v, want it to wrap ErrNotLoaded", err)
	}
}

func TestLoader_ReadBeforeAttachFails(t *testing.T) {
	l := network.NewLoader()
	defer l.Close()

	if _, err := l.Read(); !errors.Is(err, network.ErrNotLoaded) {
		t.Errorf("Read() before Attach(): error = %v, want it to wrap ErrNotLoaded", err)
	}
}

// TestLoader_ObservesRealTCPConnect is the integration test Day 05
// calls for: it starts a real TCP listener, dials it from this
// process, and confirms the resulting event's source/destination
// address and port match what the standard library itself reports for
// the connection. This is the strongest available check that
// decodeRawEvent's mixed-endianness field decoding (see event.go) is
// actually correct against a real kernel, not just internally
// consistent with itself.
func TestLoader_ObservesRealTCPConnect(t *testing.T) {
	requireRoot(t)

	l := network.NewLoader()
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
			conn.Close()
		}
	}()

	conn, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		t.Fatalf("net.Dial() error: %v", err)
	}
	defer conn.Close()

	wantLocalPort := conn.LocalAddr().(*net.TCPAddr).Port
	wantRemotePort := conn.RemoteAddr().(*net.TCPAddr).Port
	wantPID := uint32(os.Getpid())

	type result struct {
		event network.ConnectEvent
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
			// tcp_v4_connect fires for every IPv4 TCP connect on the
			// host, so on a shared runner an unrelated process's
			// connect could be read first; keep going until it's ours.
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
		if !r.event.Success {
			t.Error("Success = false, want true for a connect to an accepting listener")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not observe our own connect within 5s")
	}
}

func TestLoader_CloseInterruptsRead(t *testing.T) {
	requireRoot(t)

	l := network.NewLoader()
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
