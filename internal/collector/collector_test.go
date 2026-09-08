package collector_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/collector"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestApp_Run_ReturnsWhenContextCanceled(t *testing.T) {
	app := collector.New(config.DefaultCollectorConfig(), discardLogger())

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

// freePort mirrors internal/agent's own copy — see its doc comment.
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

// A real, no-broker-needed integration test: the metrics server itself
// needs no Kafka or ClickHouse, so it starts (and answers /metrics,
// every counter at zero) even with neither configured — see Run's own
// doc comment for why.
func TestApp_Run_MetricsServerServesRealHTTP(t *testing.T) {
	addr := freePort(t)
	cfg := config.DefaultCollectorConfig()
	cfg.MetricsAddr = addr
	app := collector.New(cfg, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	// See internal/agent's identical test for why a plain Counter
	// (always present) rather than a CounterVec (only present once a
	// labeled event has actually been recorded) is what's asserted on
	// here.
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
