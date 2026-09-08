package storage_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/storage"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// envTestAddr names the ClickHouse cluster these integration tests
// connect to. Unset (the default everywhere except CI's dedicated
// clickhouse-integration job — see .github/workflows/ci.yml), they
// skip rather than fail — there is no ClickHouse, and no Docker to run
// one, on this project's Windows dev machine. Mirrors
// internal/kafka's envTestBrokers for the same reason.
const envTestAddr = "PULSE_TEST_CLICKHOUSE_ADDR"

func requireClickHouse(t *testing.T) []string {
	t.Helper()
	v := os.Getenv(envTestAddr)
	if v == "" {
		t.Skipf("%s not set: these tests need a real ClickHouse server; see docs/design/clickhouse-storage.md", envTestAddr)
	}
	return strings.Split(v, ",")
}

// uniqueTable returns a table name unlikely to collide with another
// test run — NewBatchWriter creates it via EnsureSchema, and nothing
// here ever drops it afterward.
func uniqueTable(t *testing.T) string {
	t.Helper()
	return "pulse_test_" + strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_") + "_" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestBatchWriter_WritesAndFlushesOnBatchSize(t *testing.T) {
	addr := requireClickHouse(t)
	table := uniqueTable(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := storage.DefaultConfig(addr)
	cfg.Table = table
	cfg.BatchSize = 3
	cfg.FlushInterval = time.Hour // effectively disabled for this test

	w, err := storage.NewBatchWriter(ctx, cfg, discardLogger())
	if err != nil {
		t.Fatalf("NewBatchWriter() returned error: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	runCtx, runCancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(runCtx)
		close(done)
	}()
	t.Cleanup(func() {
		runCancel()
		<-done
	})

	for i := 0; i < 3; i++ {
		w.Enqueue(model.Event{
			ID:        fmt.Sprintf("evt-%d", i),
			Type:      "process.start",
			Timestamp: time.Now(),
			Host:      "pulse-node-1",
		})
	}

	deadline := time.After(10 * time.Second)
	for {
		count, err := countRows(ctx, addr, table)
		if err != nil {
			t.Fatalf("countRows() returned error: %v", err)
		}
		if count >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("table has %d rows after 10s, want at least 3", count)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func TestBatchWriter_ShutdownFlushesRemaining(t *testing.T) {
	addr := requireClickHouse(t)
	table := uniqueTable(t)

	setupCtx, setupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer setupCancel()

	cfg := storage.DefaultConfig(addr)
	cfg.Table = table
	cfg.BatchSize = 100 // never reached
	cfg.FlushInterval = time.Hour

	w, err := storage.NewBatchWriter(setupCtx, cfg, discardLogger())
	if err != nil {
		t.Fatalf("NewBatchWriter() returned error: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	runCtx, cancel := context.WithCancel(context.Background())
	w.Enqueue(model.Event{ID: "a", Type: "network.connect", Timestamp: time.Now(), Host: "pulse-node-1"})
	w.Enqueue(model.Event{ID: "b", Type: "network.connect", Timestamp: time.Now(), Host: "pulse-node-1"})

	done := make(chan struct{})
	go func() {
		w.Run(runCtx)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond) // let both Enqueue calls land before shutdown begins
	cancel()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run() did not return within 10s of context cancellation")
	}

	count, err := countRows(setupCtx, addr, table)
	if err != nil {
		t.Fatalf("countRows() returned error: %v", err)
	}
	if count != 2 {
		t.Errorf("table has %d rows, want 2 (both flushed during shutdown)", count)
	}
}

// countRows opens its own short-lived connection rather than reusing
// the BatchWriter under test, so the assertion doesn't depend on any
// internal state of the writer being tested.
func countRows(ctx context.Context, addr []string, table string) (int, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: addr,
		Auth: clickhouse.Auth{Database: "pulse"},
	})
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	row := conn.QueryRow(ctx, fmt.Sprintf("SELECT count() FROM %s", table))
	var count uint64
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return int(count), nil
}
