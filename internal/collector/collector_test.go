package collector_test

import (
	"context"
	"io"
	"log/slog"
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
