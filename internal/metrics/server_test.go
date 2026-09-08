package metrics_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/metrics"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// freePort asks the OS for an unused TCP port on loopback, the same
// way internal/otlp's tests find a free port for their fake gRPC
// server — real network I/O over loopback, no mocking.
func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func TestServer_ServesMetricsOverHTTP(t *testing.T) {
	reg := metrics.New()
	p := &metrics.Processor{Registry: reg}
	_ = p.Process(context.Background(), model.Event{Type: "process.start"})

	addr := freePort(t)
	srv := metrics.NewServer(addr, reg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	body := getMetricsWithRetry(t, "http://"+addr+"/metrics")
	if !strings.Contains(body, `pulse_events_total{type="process.start"} 1`) {
		t.Errorf("response body missing expected metric line: %s", body)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned error after shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within 2s of context cancellation")
	}
}

func TestServer_RunReturnsErrorWhenAddrAlreadyInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().String()

	srv := metrics.NewServer(addr, metrics.New())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	select {
	case err := <-done:
		if err == nil {
			t.Error("Run() returned nil error, want a bind failure since the address is already in use")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within 2s of a bind failure")
	}
}

func getMetricsWithRetry(t *testing.T, url string) string {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		resp, err := http.Get(url)
		if err == nil {
			defer resp.Body.Close()
			body, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				t.Fatalf("reading response body: %v", readErr)
			}
			return string(body)
		}
		select {
		case <-deadline:
			t.Fatalf("GET %s never succeeded within 2s: %v", url, err)
		case <-time.After(5 * time.Millisecond):
		}
	}
}
