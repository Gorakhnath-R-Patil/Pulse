// Package topology builds a service dependency graph from
// network.connect events already stored by internal/storage — see
// docs/design/service-topology.md for what this graph actually shows,
// and, just as importantly, what it honestly can't.
package topology

import "time"

// Node is one participant in a Graph: either a source service (Kind
// "service") or a destination endpoint (Kind "endpoint") — see Edge.
type Node struct {
	// ID identifies the node: a service label (container ID, or
	// process command, or "unknown" — see Query's doc comment) for a
	// "service" node, or an "address:port" string for an "endpoint"
	// node.
	ID   string
	Kind string
}

// Edge records that From (a service) made at least one outbound TCP
// connection to To (an endpoint) during the queried data, aggregated
// across every such connection observed.
type Edge struct {
	From string
	To   string

	Connections   uint64
	BytesSent     uint64
	BytesReceived uint64
	FirstSeen     time.Time
	LastSeen      time.Time
}

// Graph is a service dependency view: every distinct (source service,
// destination endpoint) pair observed, with Nodes deduplicated across
// every Edge that mentions them.
type Graph struct {
	Nodes []Node
	Edges []Edge
}
