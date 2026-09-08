package topology

import (
	"fmt"
	"io"
	"time"
)

// WriteDOT renders g in Graphviz DOT format — the standard, widely
// supported text format for describing a graph, renderable by
// `dot -Tpng`, countless online viewers, and editor extensions,
// without this project needing to draw anything itself. A "service"
// node renders as an ellipse (Graphviz's default), an "endpoint" node
// as a box, so the two kinds this graph honestly distinguishes (see
// docs/design/service-topology.md) stay visually distinguishable too.
func WriteDOT(w io.Writer, g *Graph) error {
	if _, err := fmt.Fprintln(w, "digraph pulse_topology {"); err != nil {
		return err
	}
	for _, n := range g.Nodes {
		shape := "ellipse"
		if n.Kind == "endpoint" {
			shape = "box"
		}
		if _, err := fmt.Fprintf(w, "  %q [shape=%s];\n", n.ID, shape); err != nil {
			return err
		}
	}
	for _, e := range g.Edges {
		label := fmt.Sprintf("%d conn", e.Connections)
		if _, err := fmt.Fprintf(w, "  %q -> %q [label=%q];\n", e.From, e.To, label); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w, "}")
	return err
}

// WriteText renders g as a plain tab-separated line per edge —
// simpler than DOT for a quick terminal look, at the cost of no
// visualization.
func WriteText(w io.Writer, g *Graph) error {
	for _, e := range g.Edges {
		_, err := fmt.Fprintf(w, "%s -> %s\tconnections=%d\tbytes_sent=%d\tbytes_received=%d\tfirst_seen=%s\tlast_seen=%s\n",
			e.From, e.To, e.Connections, e.BytesSent, e.BytesReceived,
			e.FirstSeen.Format(time.RFC3339), e.LastSeen.Format(time.RFC3339))
		if err != nil {
			return err
		}
	}
	return nil
}
