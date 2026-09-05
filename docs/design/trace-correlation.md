# Trace Correlation

Package: [`internal/correlation`](../../internal/correlation). Builds
on [`pkg/model.Span`](../../pkg/model/trace.go) and
[`internal/tracing`](../../internal/tracing) (Day 11).

## Problem, and what this honestly is not

The master specification's own example for this day is a cross-service
chain: `API → Order → Payment → PostgreSQL`. Read plainly, that's
*distributed* tracing — correlating activity across multiple services,
likely multiple hosts. This document says up front what
`internal/correlation` actually is, because it is not that, and
claiming otherwise would be exactly the "fake distributed-system
guarantees" the project's engineering standard prohibits:

- **No trace-context propagation.** A real distributed trace usually
  works because a client sends a trace ID to a server (e.g. a
  `traceparent` HTTP header) and the server continues it. Nothing this
  project has built through Day 10 reads or writes such a header —
  `internal/httpvis` parses only a request/status *line*, never headers.
- **No visibility into the other side of a connection.** `internal/network`
  captures outbound `connect()` only — see its Limitations — so this
  agent never observes *inbound* `accept()` on the server side of a
  call it made. Even if it did, that's a different agent's process,
  observed by a different agent instance.
- **No cross-host aggregation.** Each `pulse-agent` runs its own
  pipelines and its own `Correlator`, in-process. Nothing yet ships
  events off-host (Kafka is Day 14) or aggregates multiple agents'
  output anywhere (a collector doing that is later still).

Given all three are missing, `API → Order → Payment → PostgreSQL` is
not achievable today, by anyone, however it's implemented — and
`internal/correlation` doesn't pretend otherwise. What it does instead
is real and directly useful: it's this day's phrase, "when sufficient
metadata exists," taken seriously. The strongest, most universally
available signal across every capability this project has built —
process discovery, network, socket, HTTP, DNS — is which process (PID)
produced an event and when. So that's what it correlates by.

## Design

```
5 pipelines, one shared Correlator
process ──┐
network ──┤
socket  ──┼──► CorrelatingProcessor.Process(event) ──► Correlator.Observe(event) ──► Span
httpvis ──┤        (per pipeline, added alongside                │
dns     ──┘         LoggingProcessor)                             ├─ same PID, within window  → chain (same TraceID, ParentSpanID = last span)
                                                                    └─ new PID, or gap > window → new trace (new TraceID, no parent)
```

**One `Correlator`, shared across every pipeline.** `internal/agent.Run`
constructs exactly one `*correlation.Correlator` and passes it into
every `newXPipeline` call. That's what lets, say, a process's DNS query
(`internal/dns`) and its following TCP connect (`internal/network`) —
two different capabilities, two different pipelines, two different
goroutines — land in the same trace: they share nothing except the PID
the correlator keys on.

**Correlation, not decoration: it's a `pipeline.EventProcessor`, added
alongside `LoggingProcessor`, not instead of it.** Every pipeline
already logs each raw event (Day 07); `CorrelatingProcessor` runs as a
second processor on the same event, logging the span it was turned
into. Nothing about how each capability captures or normalizes events
changed for this day.

**Concurrency-safe, unlike this project's other per-capability
types.** Every `Loader` in this project is explicitly documented as
unsafe for concurrent use, because exactly one goroutine (one pipeline)
ever calls `Read` on it. `Correlator` is different on purpose: five
pipelines' worker goroutines call `Observe` on the *same* instance
concurrently, so it holds a `sync.Mutex` around its session map. This
is the first concurrency-safety requirement this project has actually
needed, not a defensive habit applied everywhere.

**Time-windowed session chaining, with the same eviction discipline as
Day 10's `queryCorrelator`.** A process's events chain into one trace
as long as consecutive events are no more than `window` apart (30s by
default); a longer gap starts a new trace, treating the process as
having moved on to unrelated work. A separate, much larger
`sessionTTL` (10 minutes) bounds memory for processes that go
permanently quiet, the same shape Day 10 already established for
exactly this reason.

## What's deliberately not here yet

- **True cross-service/cross-host correlation** — see Problem above.
  This needs trace-context propagation, inbound-connection capture, or
  cross-agent aggregation, none of which exist yet.
- **Persisted or queryable traces.** `CorrelatingProcessor` logs each
  span as it's produced; nothing accumulates a trace's full span set
  anywhere, and `internal/tracing.AssembleTrace` (Day 11) is not called
  from this pipeline at all — there is no in-memory or stored place
  holding "all spans for trace X" to assemble. A human (or a future
  Day 14+ pipeline) reading the logs could reconstruct one by grouping
  on `trace_id` and feeding the result to `AssembleTrace`, but this
  package doesn't do that step itself.

## Tradeoffs

- **Correlating by (PID, time proximity) instead of anything
  content-aware** is a real, acknowledged imprecision, not just a
  simplification. A busy, long-running server process handling many
  genuinely unrelated concurrent connections within the same 30-second
  window will have all of them chained into one trace, misleadingly
  implying they're related when they're only contemporaneous. This is
  the direct, honest cost of correlating by the one signal every
  capability actually provides — see Problem.
- **A flat chain, not a real call tree, within one process's window.**
  Each new event becomes the child of the *immediately preceding* one,
  not of whichever earlier span actually "caused" it — `Correlator` has
  no notion of causality beyond sequence. `internal/tracing.AssembleTrace`
  (Day 11) can build a real tree from spans that already carry correct
  parent links; `Correlator` doesn't have enough information to assign
  those links more precisely than "whatever came right before."

## Failure modes

`Observe` never returns an error. An event with no `Process` becomes
the root of its own singleton trace rather than being rejected — the
graceful-degradation behavior this day's "handle incomplete traces
gracefully" instruction calls for, applied at the point where metadata
actually runs out, rather than deferred to some later validation step.

## Performance

Not benchmarked. `Observe` does a mutex-guarded map lookup/insert and a
map sweep (bounded by session count, itself bounded by `sessionTTL`)
per call — cheap in isolation, but "per call, from up to five
concurrent pipelines" is exactly the kind of contention worth measuring
under real load, which doesn't exist yet to measure against.

## Security

Spans carry the same category of metadata `pkg/model.Event` already
does (process/service identity, timing) — see this project's
established "prefer metadata over sensitive application contents"
principle, which nothing about correlation changes.

## Limitations

- Correlates within one agent (one host) only — see Problem.
- Chains by temporal sequence, not causal accuracy — see Tradeoffs.
- No output beyond per-span log lines; no assembled trace is ever
  produced by this package itself.
