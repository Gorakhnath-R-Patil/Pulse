package topology_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/storage"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/topology"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// envTestAddr mirrors internal/storage's own — these tests seed real
// rows via a real storage.BatchWriter and then query them back, so
// they need everything internal/storage's own integration tests need.
// See docs/design/service-topology.md and
// docs/design/clickhouse-storage.md.
const envTestAddr = "PULSE_TEST_CLICKHOUSE_ADDR"

func requireClickHouse(t *testing.T) []string {
	t.Helper()
	v := os.Getenv(envTestAddr)
	if v == "" {
		t.Skipf("%s not set: these tests need a real ClickHouse server; see docs/design/service-topology.md", envTestAddr)
	}
	return strings.Split(v, ",")
}

func uniqueTable(t *testing.T) string {
	t.Helper()
	return "pulse_test_" + strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_") + "_" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

// seedWriter constructs and returns a ready storage.BatchWriter along
// with the connection Query itself needs — the same
// NewBatchWriter/EnsureSchema path pulse-collector uses in production,
// so these tests exercise Query against a table shaped exactly the
// way it would be in practice, not a hand-crafted test schema.
func seedWriter(t *testing.T, addr []string, table string) *storage.BatchWriter {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := storage.DefaultConfig(addr)
	cfg.Table = table
	cfg.BatchSize = 100
	cfg.FlushInterval = time.Hour

	w, err := storage.NewBatchWriter(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewBatchWriter() returned error: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

func networkConnectEvent(command, destAddr string, destPort uint16, bytesSent, bytesReceived uint64) model.Event {
	return model.Event{
		ID:        model.NewID(),
		Type:      "network.connect",
		Timestamp: time.Now(),
		Host:      "pulse-node-1",
		Process:   &model.Process{PID: 100, Command: command},
		Network: &model.Network{
			Protocol:      "tcp",
			Source:        model.Endpoint{Address: "10.0.0.1", Port: 51000},
			Destination:   model.Endpoint{Address: destAddr, Port: destPort},
			BytesSent:     bytesSent,
			BytesReceived: bytesReceived,
		},
	}
}

func TestQuery_GroupsConnectionsBySourceAndDestination(t *testing.T) {
	addr := requireClickHouse(t)
	table := uniqueTable(t)
	w := seedWriter(t, addr, table)

	// Two connections from "curl" to the same destination (should
	// collapse into one edge, aggregated) and one from "nginx" to a
	// different destination (a distinct edge).
	w.Enqueue(networkConnectEvent("curl", "10.0.0.2", 443, 100, 200))
	w.Enqueue(networkConnectEvent("curl", "10.0.0.2", 443, 50, 75))
	w.Enqueue(networkConnectEvent("nginx", "10.0.0.3", 8080, 10, 20))

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(runCtx)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond) // let all three Enqueue calls land
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("writer did not flush within 10s of shutdown")
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer queryCancel()

	conn := storageConn(t, addr, "pulse")
	g, err := topology.Query(queryCtx, conn, table)
	if err != nil {
		t.Fatalf("Query() returned error: %v", err)
	}

	var curlEdge, nginxEdge *topology.Edge
	for i, e := range g.Edges {
		switch e.From {
		case "curl":
			curlEdge = &g.Edges[i]
		case "nginx":
			nginxEdge = &g.Edges[i]
		}
	}

	if curlEdge == nil {
		t.Fatal("no edge found from curl")
	}
	if curlEdge.To != "10.0.0.2:443" {
		t.Errorf("curl edge To = %q, want %q", curlEdge.To, "10.0.0.2:443")
	}
	if curlEdge.Connections != 2 {
		t.Errorf("curl edge Connections = %d, want 2 (two connections to the same destination collapse into one edge)", curlEdge.Connections)
	}
	if curlEdge.BytesSent != 150 || curlEdge.BytesReceived != 275 {
		t.Errorf("curl edge bytes = (%d sent, %d received), want (150, 275)", curlEdge.BytesSent, curlEdge.BytesReceived)
	}

	if nginxEdge == nil {
		t.Fatal("no edge found from nginx")
	}
	if nginxEdge.To != "10.0.0.3:8080" {
		t.Errorf("nginx edge To = %q, want %q", nginxEdge.To, "10.0.0.3:8080")
	}
}

// storageConn opens a connection the same way internal/storage does
// internally, for Query to use directly — Query takes a driver.Conn,
// not a *storage.BatchWriter, since querying is a read concern
// separate from the write path pulse-collector uses. By the time this
// is called, seedWriter has already created database via its own
// NewBatchWriter call, so connecting straight to it (rather than
// "default") is safe here.
func storageConn(t *testing.T, addr []string, database string) driver.Conn {
	t.Helper()
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: addr,
		Auth: clickhouse.Auth{Database: database},
	})
	if err != nil {
		t.Fatalf("connecting to ClickHouse: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
