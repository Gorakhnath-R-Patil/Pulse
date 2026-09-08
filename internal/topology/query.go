package topology

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Query builds a Graph from every network.connect event stored in
// table, grouped by (source service, destination address:port) and
// aggregated across every connection sharing that pair.
//
// The source service label uses the same fallback precedence
// internal/correlation's serviceLabel already establishes for spans —
// container ID if known, else the process command, else "unknown" —
// replicated here as SQL rather than reused directly, since this
// package queries ClickHouse's stored columns, not Go model.Event
// values.
func Query(ctx context.Context, conn driver.Conn, table string) (*Graph, error) {
	rows, err := conn.Query(ctx, fmt.Sprintf(`
		SELECT
			coalesce(nullIf(container_id, ''), nullIf(command, ''), 'unknown') AS source,
			destination_address,
			destination_port,
			count() AS connections,
			sum(bytes_sent) AS bytes_sent,
			sum(bytes_received) AS bytes_received,
			min(timestamp) AS first_seen,
			max(timestamp) AS last_seen
		FROM %s
		WHERE type = 'network.connect'
		GROUP BY source, destination_address, destination_port
		ORDER BY source, destination_address, destination_port
	`, table))
	if err != nil {
		return nil, fmt.Errorf("topology: query: %w", err)
	}
	defer rows.Close()

	g := &Graph{}
	seen := make(map[string]bool)
	for rows.Next() {
		var source, destAddr string
		var destPort uint16
		var connections, bytesSent, bytesReceived uint64
		var firstSeen, lastSeen time.Time

		if err := rows.Scan(&source, &destAddr, &destPort, &connections, &bytesSent, &bytesReceived, &firstSeen, &lastSeen); err != nil {
			return nil, fmt.Errorf("topology: scan row: %w", err)
		}

		dest := fmt.Sprintf("%s:%d", destAddr, destPort)
		if !seen[source] {
			g.Nodes = append(g.Nodes, Node{ID: source, Kind: "service"})
			seen[source] = true
		}
		if !seen[dest] {
			g.Nodes = append(g.Nodes, Node{ID: dest, Kind: "endpoint"})
			seen[dest] = true
		}
		g.Edges = append(g.Edges, Edge{
			From:          source,
			To:            dest,
			Connections:   connections,
			BytesSent:     bytesSent,
			BytesReceived: bytesReceived,
			FirstSeen:     firstSeen,
			LastSeen:      lastSeen,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("topology: rows: %w", err)
	}
	return g, nil
}
