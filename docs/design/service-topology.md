# Service Topology

Package: [`internal/topology`](../../internal/topology). Queries
events [`internal/storage`](../../internal/storage) has written to
ClickHouse (Day 15). Exposed via `pulse-cli topology` (see
[`internal/cli/topology.go`](../../internal/cli/topology.go)).

## Problem, and what this honestly is not

A "service dependency graph" — which services call which other
services, how often, how much data — is the kind of artifact this
project's own `README.md` names as a goal. This document says up
front what `internal/topology` actually produces, for the same reason
`docs/design/trace-correlation.md` did for Day 12's correlation work:
it would be easy to call this a "service map" and imply more than
what's actually captured.

**Every edge in this graph has a real, service-identified source and
an unresolved endpoint destination — never a service-identified
destination.** `internal/network` (Day 5) captures outbound `connect()`
calls only; nothing in this project's kernel capture has ever observed
the *receiving* side of a connection (`accept()`). `docs/design/trace-
correlation.md` already documented this gap for trace correlation; it
applies identically here, and Day 14/15's Kafka+ClickHouse pipeline
doesn't change it — aggregating every agent's events in one place
doesn't manufacture a signal that was never captured on any of them.
So a graph edge always reads `service → address:port`, never
`service → service`. That's a real limitation, not a rendering choice:
this project genuinely does not know what's listening on the other
end of an observed connection, only that something was.

**DNS telemetry (Day 10) can't fill that gap either, today.**
`internal/dns.ToEvent` (see `docs/design/dns-telemetry.md`) carries the
queried name, but not the resolved IP address from the answer — the
wire-parsing this project does for DNS responses stops at response
code and answer *count*, not the answer records themselves. Without a
captured IP, there is no honest way to map a `network.connect`
destination address back to the hostname that was likely queried for
it, so `internal/topology` doesn't attempt that correlation. A
destination is always rendered as a bare `address:port`.

Given both gaps, what this package actually delivers: **an
egress-only, source-service-identified view of what each observed
process talks to, aggregated across every agent that has produced
into the same ClickHouse table** — genuinely more than any single
agent could show on its own (Day 12's correlation is explicitly
single-host; this is not, because it queries storage every agent's
Kafka-produced events already landed in), but still a fan-out view,
not a call graph.

## Design

```
pulse-cli topology
        │
        ▼
topology.Query(ctx, conn, table)
        │
        ▼
SELECT source, destination_address, destination_port,
       count(), sum(bytes_sent), sum(bytes_received),
       min(timestamp), max(timestamp)
FROM <table>
WHERE type = 'network.connect'
GROUP BY source, destination_address, destination_port
        │
        ▼
Graph{Nodes, Edges}
        │
        ▼
WriteDOT / WriteText
```

**A read-only query against already-stored data, not a new capture
mechanism or a running service.** Every byte `internal/topology` uses
was already captured by `internal/network` and already written by
`internal/storage`; this package adds no new kernel-level observation,
no new event type, and nothing running continuously — `pulse-cli
topology` connects, queries, prints, and exits, the same shape
`pulse-cli config validate` already has.

**The source service label matches `internal/correlation`'s own
precedence, replicated as SQL.** `internal/correlation`'s
`serviceLabel` prefers a process's container ID, falls back to its
command, and falls back to `"unknown"` — the same three-step fallback
`topology.Query`'s `coalesce(nullIf(container_id, ''), nullIf(command,
''), 'unknown')` expression encodes. This isn't shared code (the two
packages have no dependency on each other — one operates on Go
`model.Event` values, the other on SQL over stored columns) but it is
a deliberately shared *convention*, so a service labeled one way in a
trace and a different way in a topology graph would be a bug, not an
accepted inconsistency.

**Aggregated server-side, not row-by-row in Go.** The `GROUP BY` and
`sum()`/`count()`/`min()`/`max()` calls happen in ClickHouse's own
query engine — exactly the workload a column store is built for — so
`Query` only ever materializes one row per distinct (source,
destination) pair into Go, however many raw `network.connect` events
that represents.

**Two render formats, no dependency for either.** `WriteDOT` emits
Graphviz DOT — a plain, well-known text format countless existing
tools can already turn into an image, so this project doesn't need to
draw anything itself or add a graphics dependency. `WriteText` emits a
simpler tab-separated line per edge for a quick terminal look. Both
are pure functions over an already-built `Graph`, independent of
`Query` and independently tested without ClickHouse.

## Tradeoffs

- **No time-windowing.** `Query` aggregates every matching row in the
  table, unbounded — a connection observed a year ago and one observed
  a second ago count equally toward the same edge. There's no
  retention policy on the underlying table yet either (see
  `docs/design/clickhouse-storage.md`'s Limitations), so this is
  consistent with, not worse than, what's already true of the data
  it's reading. A time-range filter is a natural, small extension once
  there's a reason to want one (e.g. "topology in the last hour" for
  an incident) — not added preemptively.
- **`GROUP BY` on three columns, no index tuning.** `events` is
  ordered by `(timestamp, id)` (see `docs/design/clickhouse-storage.md`),
  not by anything topology-specific — a full scan of every
  `network.connect` row is what this query actually does today. Fine
  at today's (nonexistent) production volume; a real deployment would
  likely want a materialized view or a secondary ordering once this
  query runs against real data at real scale, which doesn't exist yet
  to design against.

## Failure modes

`Query` returns an error (wrapping ClickHouse's own) on a connection
failure, a malformed table name, or a scan failure — `pulse-cli
topology` reports it to stderr and exits `1`, the same
error-reporting shape `config validate` already has. There is no
best-effort/degraded mode here the way `internal/agent`'s capability
loading has: querying is a one-shot, explicit operator action, not a
background capability that should keep running without a subsystem
it depends on.

## Performance

Not benchmarked, for the same reason as every other new subsystem this
week: no representative production data volume exists yet to
benchmark a full-table `GROUP BY` against.

## Security

The graph itself carries the same metadata-only shape (service labels,
IP addresses, ports, byte counts) this project's stored events already
have — no new data is derived or exposed beyond what's already stored.
`pulse-cli topology` inherits `internal/storage`'s own connection
security posture (see `docs/design/clickhouse-storage.md`'s Security
section): no TLS, ClickHouse's own username/password only.

## Limitations

- Every edge's destination is `address:port`, never a resolved
  service name — see Problem.
- No DNS-based hostname enrichment — the data to do it honestly
  doesn't exist yet (`internal/dns` doesn't capture answer addresses).
- No time-windowing or filtering — see Tradeoffs.
- Read-only, on-demand via `pulse-cli`; nothing renders this
  continuously or exposes it as a live service yet.
