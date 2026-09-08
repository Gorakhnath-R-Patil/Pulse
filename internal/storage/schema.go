package storage

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// eventsDDL creates the table a BatchWriter inserts into, if it
// doesn't already exist. Columns mirror pkg/model.Event's own fields
// directly — see convert.go's eventToRow, which must stay in sync with
// this list. Attributes (a free-form map on model.Event) is stored as
// a native ClickHouse Map(String, String) rather than a serialized
// JSON string, so it stays queryable (map access, arrayJoin, etc.)
// instead of opaque text.
//
// ENGINE = MergeTree, ordered by (timestamp, id): MergeTree is
// ClickHouse's standard general-purpose engine, and ordering by time
// first is the access pattern every telemetry query this project can
// currently imagine needs (a time range, optionally narrowed further)
// — see docs/design/clickhouse-storage.md.
const eventsDDL = `
CREATE TABLE IF NOT EXISTS %s (
	id String,
	type String,
	timestamp DateTime64(9),
	host String,
	pid UInt32,
	command String,
	executable String,
	container_id String,
	pod_uid String,
	source_address String,
	source_port UInt16,
	destination_address String,
	destination_port UInt16,
	bytes_sent UInt64,
	bytes_received UInt64,
	attributes Map(String, String)
) ENGINE = MergeTree
ORDER BY (timestamp, id)
`

// EnsureSchema creates table (in database cfg.Database, via conn) if it
// doesn't already exist. Called once by NewBatchWriter — see its doc
// comment for why that, rather than a separate migration step/tool, is
// enough for what this project needs today.
func EnsureSchema(ctx context.Context, conn driver.Conn, table string) error {
	if err := conn.Exec(ctx, fmt.Sprintf(eventsDDL, table)); err != nil {
		return fmt.Errorf("storage: create table %s: %w", table, err)
	}
	return nil
}
