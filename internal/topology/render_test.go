package topology_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/topology"
)

func testGraph() *topology.Graph {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return &topology.Graph{
		Nodes: []topology.Node{
			{ID: "curl", Kind: "service"},
			{ID: "10.0.0.2:443", Kind: "endpoint"},
		},
		Edges: []topology.Edge{
			{
				From: "curl", To: "10.0.0.2:443",
				Connections: 3, BytesSent: 100, BytesReceived: 200,
				FirstSeen: ts, LastSeen: ts.Add(time.Minute),
			},
		},
	}
}

func TestWriteDOT_RendersNodesAndEdges(t *testing.T) {
	var buf bytes.Buffer
	if err := topology.WriteDOT(&buf, testGraph()); err != nil {
		t.Fatalf("WriteDOT() returned error: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "digraph pulse_topology {") {
		t.Errorf("output missing digraph header: %s", out)
	}
	if !strings.Contains(out, `"curl" [shape=ellipse];`) {
		t.Errorf("output missing service node: %s", out)
	}
	if !strings.Contains(out, `"10.0.0.2:443" [shape=box];`) {
		t.Errorf("output missing endpoint node: %s", out)
	}
	if !strings.Contains(out, `"curl" -> "10.0.0.2:443" [label="3 conn"];`) {
		t.Errorf("output missing edge: %s", out)
	}
}

func TestWriteText_RendersOneLinePerEdge(t *testing.T) {
	var buf bytes.Buffer
	if err := topology.WriteText(&buf, testGraph()); err != nil {
		t.Fatalf("WriteText() returned error: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "curl -> 10.0.0.2:443") {
		t.Errorf("output missing edge summary: %s", out)
	}
	if !strings.Contains(out, "connections=3") {
		t.Errorf("output missing connection count: %s", out)
	}
	if !strings.Contains(out, "bytes_sent=100") || !strings.Contains(out, "bytes_received=200") {
		t.Errorf("output missing byte counts: %s", out)
	}
}

func TestWriteDOT_EmptyGraph(t *testing.T) {
	var buf bytes.Buffer
	if err := topology.WriteDOT(&buf, &topology.Graph{}); err != nil {
		t.Fatalf("WriteDOT() returned error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "digraph pulse_topology {") || !strings.Contains(out, "}") {
		t.Errorf("empty graph should still render valid DOT wrapper: %s", out)
	}
}
