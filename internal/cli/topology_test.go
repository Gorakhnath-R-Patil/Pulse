package cli_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/cli"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/storage"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

func TestExecute_Topology_MissingAddrIsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli.Execute([]string{"topology"}, &stdout, &stderr)

	if code != cli.ExitUsage {
		t.Errorf("code = %d, want %d", code, cli.ExitUsage)
	}
	if !strings.Contains(stderr.String(), "-clickhouse-addr") {
		t.Errorf("stderr = %q, want it to mention -clickhouse-addr", stderr.String())
	}
}

func TestExecute_Topology_UnknownFormatIsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli.Execute([]string{"topology", "-clickhouse-addr", "localhost:9000", "-format", "yaml"}, &stdout, &stderr)

	if code != cli.ExitUsage {
		t.Errorf("code = %d, want %d", code, cli.ExitUsage)
	}
	if !strings.Contains(stderr.String(), "yaml") {
		t.Errorf("stderr = %q, want it to name the bad format", stderr.String())
	}
}

func TestExecute_Topology_UnreachableClickHouseIsFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	// A real address, syntactically valid, that nothing is listening
	// on — this project's usual "unreachable, not malformed" case,
	// same as internal/otlp and internal/kafka's own error-path tests.
	code := cli.Execute([]string{"topology", "-clickhouse-addr", "127.0.0.1:1"}, &stdout, &stderr)

	if code != cli.ExitFailure {
		t.Errorf("code = %d, want %d", code, cli.ExitFailure)
	}
	if !strings.Contains(stderr.String(), "pulse-cli") {
		t.Errorf("stderr = %q, want an error message", stderr.String())
	}
}

// The happy path — a real query against a real ClickHouse server —
// needs PULSE_TEST_CLICKHOUSE_ADDR, the same env var
// internal/storage's and internal/topology's own integration tests
// use; skips itself everywhere one isn't configured, including this
// project's own Windows dev machine. See docs/design/service-topology.md.
func TestExecute_Topology_RealQuery(t *testing.T) {
	addrEnv := os.Getenv("PULSE_TEST_CLICKHOUSE_ADDR")
	if addrEnv == "" {
		t.Skip("PULSE_TEST_CLICKHOUSE_ADDR not set: this test needs a real ClickHouse server")
	}
	addr := strings.Split(addrEnv, ",")
	table := "pulse_test_cli_topology_" + strconv.FormatInt(time.Now().UnixNano(), 10)

	setupCtx, setupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer setupCancel()

	cfg := storage.DefaultConfig(addr)
	cfg.Table = table
	cfg.BatchSize = 1
	cfg.FlushInterval = time.Hour

	w, err := storage.NewBatchWriter(setupCtx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
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

	w.Enqueue(model.Event{
		ID:        model.NewID(),
		Type:      "network.connect",
		Timestamp: time.Now(),
		Host:      "pulse-node-1",
		Process:   &model.Process{PID: 100, Command: "curl"},
		Network: &model.Network{
			Source:      model.Endpoint{Address: "10.0.0.1", Port: 51000},
			Destination: model.Endpoint{Address: "10.0.0.2", Port: 443},
		},
	})

	time.Sleep(50 * time.Millisecond)
	runCancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("writer did not flush within 10s of shutdown")
	}

	var stdout, stderr bytes.Buffer
	code := cli.Execute([]string{"topology", "-clickhouse-addr", addrEnv, "-table", table}, &stdout, &stderr)

	if code != cli.ExitSuccess {
		t.Fatalf("code = %d, want %d; stderr: %s", code, cli.ExitSuccess, stderr.String())
	}
	if !strings.Contains(stdout.String(), "curl -> 10.0.0.2:443") {
		t.Errorf("stdout = %q, want it to contain the observed edge", stdout.String())
	}
}
