package agent_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/agent"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestApp_Run_ReturnsWhenContextCanceled(t *testing.T) {
	app := agent.New(config.DefaultAgentConfig(), discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // simulate an immediate shutdown signal

	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within 2s of context cancellation")
	}
}

// freePort finds an unused loopback port the same way
// internal/metrics/server_test.go's own helper does: listen once to
// let the OS pick one, close it, and reuse the address — an accepted
// TOCTOU risk this project already relies on elsewhere (see
// internal/otlp/exporter_test.go's startFakeCollector).
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

// This is a real, no-eBPF-needed integration test: MetricsAddr's
// server needs no Linux, kernel privilege, or external infrastructure
// (unlike every telemetry capability itself), so unlike those, this
// runs everywhere, including this project's own Windows dev machine.
func TestApp_Run_MetricsServerServesRealHTTP(t *testing.T) {
	addr := freePort(t)
	cfg := config.DefaultAgentConfig()
	cfg.MetricsAddr = addr
	app := agent.New(cfg, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	// pulse_network_bytes_sent_total is a plain Counter, not a
	// CounterVec: unlike pulse_events_total and friends (which only
	// appear once at least one labeled event has actually been
	// recorded — never true here, since this test runs on a platform
	// with no real eBPF capture), a Counter always exposes its one
	// zero-valued series immediately on registration, so it's the
	// right thing to assert on without needing a real captured event.
	body := getWithRetry(t, "http://"+addr+"/metrics")
	if !strings.Contains(body, "pulse_network_bytes_sent_total") {
		t.Errorf("response body missing expected metric family: %s", body)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within 2s of context cancellation")
	}
}

func getWithRetry(t *testing.T, url string) string {
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
