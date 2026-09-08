# OTLP Export

Package: [`internal/otlp`](../../internal/otlp). Consumes
[`pkg/model.Span`](../../pkg/model/trace.go) values produced by
[`internal/correlation`](../../internal/correlation) (Day 12).

## Problem

Through Day 12, a correlated span's only destination is a log line —
useful for a human reading `pulse-agent`'s own output, useless to any
existing tracing backend (Jaeger, Tempo, an OTel Collector, a vendor
SaaS). This day makes spans leave the process, in a format something
else already knows how to consume, without inventing a bespoke wire
format for it.

OTLP (the OpenTelemetry Protocol) is the boring, proven choice here —
it's what "boring, proven technology" means in a tracing context: every
mainstream backend either speaks OTLP natively or has a well-maintained
receiver for it, so exporting OTLP buys interoperability this project
would otherwise have to build one integration at a time.

## What "OTLP-compatible" means here, and what it doesn't

`internal/otlp` implements exactly one thing: a `TraceServiceClient`
that calls `Export` with a batch of spans shaped as OTLP's
`ExportTraceServiceRequest`. It does not implement:

- **Metrics or logs export.** OTLP defines three signals; this package
  only ever touches the trace one, because that's the only signal this
  project produces (Day 17's metrics engine is a separate, later
  concern with its own export path, likely Prometheus-shaped rather
  than OTLP-shaped — not decided yet).
- **The OTLP/HTTP transport, or any OTLP receiver.** Only the gRPC
  client side exists. Nothing in this project accepts OTLP from
  elsewhere.
- **Span events, links, or status.** `model.Span` doesn't carry them
  (see `docs/design/trace-model.md`), so there's nothing to convert —
  `spanToProto` sets exactly the fields `model.Span` actually has:
  IDs, name, kind (always `INTERNAL` — `model.Span` has no notion of
  server/client/producer/consumer), timestamps, and attributes.
- **Semantic-convention attribute names beyond `service.name` and
  `host.name`.** OTel defines a large, evolving vocabulary of standard
  attribute keys (`http.method`, `net.peer.name`, etc.); `model.Span`'s
  attribute map (see `internal/correlation`) uses whatever keys the
  correlator happened to record, unchanged. A backend that expects
  strict semantic-convention names may not recognize them specially —
  they still arrive as valid OTLP attributes, just not necessarily
  ones a UI has a built-in icon for.

Given all that, "OTLP-compatible" here means: **a real OTLP collector
listening on gRPC will accept these requests and store real,
correctly-shaped spans.** It does not mean full OTLP SDK parity.

## Design

```
CorrelatingProcessor.Process(event)
        │
        ├─► Correlator.Observe(event) ──► span
        │
        ├─► log span                              (unconditional, Day 12 behavior unchanged)
        │
        └─► Exporter.Enqueue(span)                 (only if an Exporter is configured)
                    │
                    ▼
              BatchExporter.queue (buffered channel, size = QueueSize)
                    │
                    ▼  (Run, one goroutine, started by internal/agent)
              batch accumulates until BatchSize spans queued, or FlushInterval elapses
                    │
                    ▼
              exportWithRetry: export, retry with exponential backoff (100ms, ×2) up to MaxRetries
                    │
                    ▼
              gRPC Export() ──► OTLP collector
```

**Export is optional and off by default.** `AgentConfig.OTLPEndpoint`
(YAML `otlp_endpoint`, env `PULSE_OTLP_ENDPOINT`) is empty unless an
operator sets it. `internal/agent.Run` only constructs a
`BatchExporter` when it's non-empty — pulse-agent never dials a
made-up default collector address, matching this project's existing
"never reach for an invented default for something with real
infrastructure implications" stance (see `internal/config`'s handling
of `--config` pointing at a missing file).

**One shared `CorrelatingProcessor`, not one per pipeline.** Through
Day 12, each of the five pipelines (`internal/agent`'s
`newXPipeline` constructors) built its *own*
`&correlation.CorrelatingProcessor{Correlator: corr, ...}` wrapping a
shared `*Correlator` — logging worked identically either way, since
each processor was a thin, stateless wrapper. Adding `Exporter` broke
that equivalence: five separately-constructed processors would mean
either five copies of the same `Exporter` field (harmless but
redundant) or, worse, an easy place for one pipeline's wiring to
silently omit it. `internal/agent.Run` now constructs exactly one
`CorrelatingProcessor` and passes the same pointer to every pipeline,
the same way it already did for the `Correlator` itself.

**Batching, not one-span-per-RPC.** A single gRPC call per span would
mean this project's telemetry volume (potentially every process
start/exit, every TCP connect, every socket close, every HTTP line,
every DNS query, now emitted as spans) turns into an equal volume of
network round-trips. `BatchExporter` instead queues spans and flushes
a batch whenever it reaches `BatchSize` (default 512) or
`FlushInterval` elapses (default 5s) — whichever comes first, so a
quiet period doesn't hold spans indefinitely and a busy period doesn't
grow a batch without bound.

**Backpressure via a bounded channel, the same discipline as
`internal/pipeline`.** `Enqueue` blocks when the queue (default 4096)
is full rather than dropping spans silently — consistent with this
project's established stance (see `docs/design/event-pipeline.md`)
that silent data loss under load is worse than backpressure, even
though backpressure here means a slow collector can eventually stall
`CorrelatingProcessor.Process`, and therefore every pipeline feeding
it. This is a real, accepted tradeoff — see Failure modes.

**Retry with exponential backoff, then drop.** A transient collector
hiccup (a restart, a brief network blip) shouldn't drop a batch on the
first failed attempt, but a genuinely unreachable collector can't be
allowed to retry forever either — that would let a batch's spans hold
memory indefinitely and starve the queue behind it. `exportWithRetry`
tries the initial attempt plus `MaxRetries` more (default 3),
doubling its backoff from 100ms, then logs a warning and drops the
batch. Export failures are logged, never propagated — telemetry
export breaking is never allowed to be a reason the agent itself
breaks, the same principle `internal/agent.Run`'s best-effort
capability loading already applies.

**Shutdown drains the queue once, then exits.** `Run`'s `ctx.Done()`
branch drains whatever is already queued (non-blocking, since nothing
new is added to the queue once shutdown begins in practice — the
producers stop first, see below) and flushes it one last time, using
a *fresh* `context.Background()`-derived timeout rather than the
already-canceled `ctx`, since a canceled context can't be used to
bound a new call. `internal/agent.Run` waits (via a dedicated
`sync.WaitGroup`) for this goroutine to actually finish — not just for
`ctx` to be canceled — before calling `Close()` on the gRPC
connection, so a still-in-flight final export is never cut off by its
own transport closing underneath it.

## Tradeoffs

- **Grouping by `Service` into separate `ResourceSpans` reprocesses
  every batch's spans into a map on every flush.** Simple and correct,
  and batches are bounded (`BatchSize`), so this is not a scaling
  concern at today's volumes — not benchmarked, see Performance.
- **`Insecure: true` by default.** `DefaultConfig` connects without
  TLS, appropriate for a local or sidecar collector — the deployment
  shape this project's own architecture diagram assumes for Day 1-13
  (agent and collector on related, trusted infrastructure). This is
  documented on `Config.Insecure` as something to turn off for any
  collector reached over an untrusted network; nothing yet in this
  project's configuration surface (`AgentConfig`) exposes a way to
  enable TLS, since no deployment built so far needs it. A real gap if
  and when a deployment does.
- **No authentication (API key, mTLS) support at all.** Same reasoning
  as TLS above — nothing in this project's current deployment model
  needs it yet, and adding it before there's a concrete need would be
  exactly the "premature architecture" the project's engineering
  standard warns against.

## Failure modes

- **Collector unreachable for longer than `MaxRetries` allows:**
  batches are dropped (logged at `Warn`), never buffered beyond the
  current batch — a stuck collector loses telemetry for its downtime,
  by design, rather than growing unbounded memory trying to hold onto
  it.
- **Collector unreachable and the queue fills up:** `Enqueue` blocks.
  Since `Enqueue` is called synchronously from
  `CorrelatingProcessor.Process`, which every pipeline's worker
  goroutines call, a sustained outage eventually applies backpressure
  all the way back to each pipeline's own bounded queue
  (`internal/pipeline.Config.QueueSize`), and from there to each
  loader's `Read` loop. This is consistent with — not a new
  exception to — this project's backpressure-over-silent-drop stance,
  but it is a real, sharper consequence once a slow *external* system
  (a collector, possibly over a real network) is what pipelines can
  now stall on, rather than only this process's own queues.
- **`NewBatchExporter` fails (e.g. malformed endpoint address):**
  `internal/agent.Run` logs a warning and continues running without
  export — matching the best-effort startup behavior every other
  capability already has. `grpc.NewClient` does not dial eagerly, so
  this failure mode only actually triggers on a malformed target
  string, not on "collector not listening yet"; the latter is instead
  handled by the retry/drop behavior above once real export attempts
  begin.

## Performance

Not benchmarked, for the same reason as `internal/correlation`: no
representative production load exists yet to benchmark against, and a
number produced against an ad-hoc synthetic load would be exactly the
kind of unearned precision this project's "no fake engineering"
standard prohibits. What's structurally true: batching bounds RPC
count, not total bytes transferred; a busy agent under a slow
collector will accumulate queued spans up to `QueueSize` before
blocking pipelines.

## Security

No new sensitive data leaves the process here that wasn't already
computed — spans carry the same metadata-only shape
`internal/correlation` already produces (see its own Security
section). What's new is that this metadata now leaves the host over
the network, to whatever `OTLPEndpoint` names. Combined with
`Insecure: true` by default (see Tradeoffs), this means: **do not
point `otlp_endpoint` at a collector reachable outside a trusted
network without adding TLS support first** — a real, documented
limitation, not a hidden one.

## Limitations

- Trace signal only — no metrics or logs export (see above).
- gRPC transport only — no OTLP/HTTP.
- No TLS or authentication configuration surface yet.
- Attribute keys are whatever `internal/correlation` produced, not
  necessarily OTel semantic-convention names.
- A batch that exhausts its retries is dropped, not persisted or
  retried later — there is no durable queue anywhere in this path.
