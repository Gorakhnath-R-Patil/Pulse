# ClickHouse Storage

Package: [`internal/storage`](../../internal/storage). Consumes
[`pkg/model.Event`](../../pkg/model/event.go) values
[`internal/collector`](../../internal/collector) reads off Kafka (Day
14).

## Problem

Day 14 gave `pulse-collector` something to do with a consumed event —
log it — but nothing durable. A restart loses everything; nothing can
be queried after the fact. The architecture diagram in the root
`README.md` puts storage right after Kafka in the chain
(`Kafka → Storage/Metrics`), and ClickHouse specifically — named in
this project's own design principles alongside Kafka, Prometheus, and
Grafana as "boring, proven technology" — is the obvious choice for
this shape of data: high-volume, append-only, time-ordered telemetry
rows, queried later by time range and a handful of columns. That's
precisely ClickHouse's designed use case (it's a column store built
for exactly this workload), not a generic OLTP problem a general-
purpose database would also solve adequately.

## Design

```
pulse-collector
────────────────
kafka consumption pipeline
        │
        ├─► LoggingProcessor          (Day 14, unconditional)
        │
        └─► storage.Processor          (new, optional)
                    │
                    ▼
              BatchWriter.Enqueue(event)
                    │
                    ▼  (Run, one goroutine, started by internal/collector)
              batch accumulates until BatchSize rows queued, or FlushInterval elapses
                    │
                    ▼
              writeWithRetry: PrepareBatch, append every row, Send, retry with backoff
                    │
                    ▼
              ClickHouse (events table, MergeTree, ORDER BY (timestamp, id))
```

**One more optional processor in the same pipeline, not a separate
path.** Exactly the shape Day 14 already established for Kafka
production in `internal/agent`: `internal/collector.Run` constructs a
`*storage.BatchWriter` only if `ClickHouseAddr` is configured, and
appends a `*storage.Processor` wrapping it to the Kafka consumption
pipeline's processor chain (see `internal/collector/kafka.go`'s
`newKafkaPipeline`), alongside — not instead of — the existing
`LoggingProcessor`.

**A hand-rolled batch/retry layer here, unlike `internal/kafka`'s
choice not to build one — and that's a deliberate, explained
inconsistency, not an oversight.** `docs/design/kafka-transport.md`
argues building `internal/otlp.BatchExporter`-style batching for Kafka
production would be pure duplication, because `kafka-go`'s `Writer`
already batches and retries internally. ClickHouse's own Go client
does the opposite: `driver.Conn.PrepareBatch` builds exactly the batch
you construct row by row and sends exactly when you call `Send` — no
implicit batching underneath. Combined with ClickHouse's own
well-documented preference for large, infrequent inserts over small,
frequent ones (its canonical anti-pattern is exactly what naive
one-row-per-event writing would produce), `storage.BatchWriter` needs
the same size/interval-triggered batching, retry-with-backoff, and
shutdown-drain shape `internal/otlp.BatchExporter` already has — for a
different, ClickHouse-specific reason than gRPC's lack of batching.

**Schema is created at connection time, not managed by a separate
migration tool.** `NewBatchWriter` runs `CREATE DATABASE IF NOT
EXISTS` and `CREATE TABLE IF NOT EXISTS` itself before returning,
rather than requiring an operator to run a migration step first, or
this project reaching for a dedicated schema-migration framework. This
is deliberately minimal — there is exactly one table, its shape is
fixed by `pkg/model.Event`'s own fields, and nothing here has evolved
it yet (see Limitations for what a real migration story would need
once it does).

**Connecting is two-step because a session can't set a database that
doesn't exist yet as its own default.** `NewBatchWriter` first opens a
short-lived connection against ClickHouse's own always-present
`default` database, creates `cfg.Database` if needed, closes that
connection, then opens the real, long-lived one with `cfg.Database` as
its default — see the doc comment on `NewBatchWriter` for exactly why.

**Columns mirror `pkg/model.Event`'s fields directly, flattened.**
`internal/storage/schema.go`'s `eventsDDL` declares one column per
field `pkg/model.Event` (and its `Process`/`Network`/`Container`
sub-structures) actually has — no JSON blob column holding the whole
event. `Attributes` is the one exception: it's ClickHouse's native
`Map(String, String)` type, not a serialized string, so it stays
queryable (map access, `arrayJoin`, etc.) rather than opaque text. A
field absent on a given event (e.g. `Process` on a non-process event)
writes as that column's zero value — the same convention
`pkg/model.Marshal`'s JSON encoding already uses for an absent
optional field (an omitted key, which decodes back to a zero value),
so this isn't a new convention specific to storage.

## Tradeoffs

- **A fixed, hand-written schema instead of a real migration
  tool.** Fine for one table that hasn't changed shape yet; a second
  schema change would need this project to decide on an actual
  migration story (versioned `.sql` files, a tool like `golang-
  migrate`, or continuing to hand-roll `IF NOT EXISTS`-only changes)
  rather than something `internal/storage` provides today.
- **`MergeTree` ordered by `(timestamp, id)`, not partitioned.** A
  real production ClickHouse deployment usually partitions a
  time-series table (e.g. by month) so old data can be dropped cheaply
  and queries against a narrow time range skip whole partitions. This
  schema doesn't do that yet — not because it's wrong, but because
  there's no retention policy or query pattern established yet to
  design a partition scheme against; adding one is a schema
  migration this project doesn't have a story for regardless (see
  above).
- **Building a second batch/retry implementation (see Design) is real,
  accepted duplication of shape with `internal/otlp.BatchExporter`,
  not logic** — the two share no code, only a pattern, because Go has
  no generic "batch this queue with backoff" abstraction in this
  project yet and building one prematurely, before a third real user
  of the pattern exists, would be exactly the kind of speculative
  abstraction this project's engineering standard warns against.

## Failure modes

- **ClickHouse unreachable when `pulse-collector` starts:**
  `storage.NewBatchWriter`'s eager connect-and-verify-schema means
  this fails immediately and loudly (logged, not fatal — matching
  every other best-effort capability in this project) rather than
  silently accepting events it can never store. Unlike
  `otlp.NewBatchExporter`/`kafka.NewProducer`'s lazy dial, there's no
  equivalent "works until the first real write" grace period here.
- **ClickHouse unreachable for longer than `MaxRetries` allows:**
  batches are dropped (logged at `Warn`), never buffered beyond the
  current batch — the identical policy `internal/otlp.BatchExporter`
  already established, for the identical reason (bounded memory over
  unbounded retry).
- **`QueueSize` exhausted:** `Enqueue` blocks, which — since it's
  called synchronously from the Kafka consumption pipeline's worker —
  eventually applies backpressure back to `kafkaSource.Read`, and from
  there to the Kafka consumer itself (messages accumulate on the
  broker instead of being consumed, rather than being read and
  discarded). Consistent with this project's established
  backpressure-over-silent-drop stance.

## Performance

Not benchmarked, for the same reason as `internal/otlp` and
`internal/kafka`: no representative production load exists yet.
`BatchSize` defaults to 1000 (higher than `internal/otlp`'s 512),
reflecting ClickHouse's stronger preference for larger batches — a
reasonable starting point given the client library's own
documentation, not a measured number.

## Security

No new sensitive data is computed here that wasn't already produced —
a stored row carries exactly the same metadata-only shape
`pkg/model.Event` already has. As with `internal/kafka` and
`internal/otlp`'s own defaults, this package uses `clickhouse-go`'s
plain TCP native protocol with no TLS configuration surface —
appropriate only for a ClickHouse server on infrastructure this
deployment already trusts. `ClickHouseUsername`/`ClickHousePassword`
exist (unlike Kafka/OTLP, which have no auth surface at all yet)
because ClickHouse's own out-of-the-box setup already expects a
username, so wiring it through cost little; that's authentication, not
encryption — credentials and every row still cross the network in
cleartext.

## Limitations

- No TLS to the ClickHouse server.
- No real schema migration story beyond `IF NOT EXISTS` DDL run at
  connect time — see Tradeoffs.
- No partitioning or retention policy on the `events` table.
- A batch that exhausts its retries is dropped, not persisted or
  retried later — there is no durable queue in this path, the same
  limitation `internal/otlp` already documents for the identical
  reason.
- Integration tests (`internal/storage/integration_test.go`) need a
  real ClickHouse server and skip themselves everywhere one isn't
  configured via `PULSE_TEST_CLICKHOUSE_ADDR` — including this
  project's own Windows dev machine, which has neither ClickHouse nor
  Docker to run it. CI's dedicated `clickhouse-integration` job (see
  `.github/workflows/ci.yml`) is what actually exercises them, against
  a real single-node server running as a GitHub Actions service
  container.
