# Metrics

Package: [`internal/metrics`](../../internal/metrics). Consumes
[`pkg/model.Event`](../../pkg/model/event.go) values from the same
pipelines every other optional processor (Day 13's OTLP exporter, Day
14's Kafka producer, Day 15's ClickHouse writer) already taps into.

## Problem

Every capability this project has built produces useful aggregate
signal — how many connections succeeded vs failed, how many bytes
moved, what DNS latency looked like — that today only exists as
individual log lines and (if configured) individual spans, events, or
stored rows. None of that answers "is this healthy right now" at a
glance, which is what a metrics system is for. Prometheus is named
explicitly in this project's own design principles alongside Kafka and
ClickHouse as "boring, proven technology," and it's the obvious fit
for exactly this kind of aggregate, scrape-based operational signal.

## Design

```
pulse-agent                              pulse-collector
────────────                              ───────────────
5 pipelines ──► LoggingProcessor           kafka consumption pipeline
            ──► CorrelatingProcessor (+OTLP)       │
            ──► kafka.ProducingProcessor            ├─► LoggingProcessor
            ──► metrics.Processor                   ├─► storage.Processor
                        │                            └─► metrics.Processor
                        ▼                                     │
              metrics.Registry                                ▼
                        │                            metrics.Registry
                        ▼                                     │
              GET /metrics (this agent's                      ▼
              own capture only)                      GET /metrics (this
                                                       collector's own
                                                       consumption only)
```

**Each binary exposes its own metrics; nothing aggregates across
instances in-process.** This is the standard Prometheus multi-target
model — the same shape `node_exporter`, `cAdvisor`, and most
Prometheus-native software already use — not something this project
invented: every `pulse-agent` and `pulse-collector` instance answers
`/metrics` for *itself* only, and Prometheus's own scrape-then-`PromQL`
model is what aggregates across many instances (`sum by (...) (...)`
across every `instance` label), not code in this project. Kafka and
ClickHouse already solved cross-host aggregation for raw events (Day
14/15); metrics deliberately doesn't reach for that same path, because
Prometheus already has its own, more idiomatic answer to "aggregate
across many sources."

**One more optional processor, the same shape as every other optional
processor this project has added since Day 13.** `internal/agent.Run`
and `internal/collector.Run` each construct a `*metrics.Registry` only
if `MetricsAddr` is configured, and append a `*metrics.Processor`
wrapping it to the same `extra` processor chain OTLP export, Kafka
production, and ClickHouse storage already use. Recording a metric is
just one more thing that happens to an event already flowing through
the pipeline — no separate collection path, no polling, no additional
event type.

**The official `client_golang` library, not a hand-rolled exposition
writer.** Prometheus's text exposition format has real syntactic rules
(escaping, ordering, type declarations) that `client_golang` already
implements correctly and is the de facto standard way every Prometheus-
instrumented Go service produces it — reimplementing that by hand
would be exactly the kind of reinventing-a-solved-problem this
project's "boring, proven technology" principle argues against, not an
example of it.

**A `Registry` wraps its own `*prometheus.Registry`, never the global
default.** Using `prometheus.DefaultRegisterer` would mean every
`metrics.New()` call (including in tests) fights over the same global
registration state — the same reason `internal/pipeline` and every
other stateful type in this project takes its dependencies explicitly
rather than reading package-level globals.

**Metrics are derived from `event.Type` and `event.Attributes`
directly, mirroring how `pipeline.LoggingProcessor` already reads an
event — no new fields on `pkg/model.Event`.** A `network.connect`
event's `tcp.connect_success` attribute becomes the
`pulse_network_connect_total{success=...}` label; a `dns.response`
event's already-computed `dns.latency_ms` attribute (`internal/dns`'s
own real, transaction-ID-based latency — see
`docs/design/dns-telemetry.md`) becomes a histogram observation. Only
metrics with a genuine, already-captured signal exist — see
Limitations for what was deliberately left out.

## Tradeoffs

- **`CounterVec`/histogram label cardinality is bounded by what this
  project already captures, not audited against real-world
  cardinality risk.** `http.path` is deliberately *not* a label
  (unlike `http.method`/`http.status`) precisely because path values
  are effectively unbounded (every unique URL becomes a new label
  combination) — the one cardinality decision this design doc calls
  out explicitly. The others (event type, success/failure, HTTP
  method/status, DNS qtype/response code) are all small, bounded
  vocabularies.
- **No metrics survive a restart.** `metrics.Registry` is
  in-memory only, matching Prometheus's own model (a scraped target is
  expected to reset counters on restart; Prometheus's `rate()`/`increase()`
  functions handle that by design) rather than a gap specific to this
  project.

## Failure modes

`Processor.Process` never returns an error — a metrics update can't
meaningfully fail in a way worth reporting back to the pipeline (the
same rationale `pipeline.LoggingProcessor` already documents). If
`MetricsAddr` is already in use, `metrics.Server.Run` returns that
error immediately (not silently ignored) and — as with every other
optional capability's own startup failure — the owning `Run` logs a
warning and continues without it rather than refusing to start the
whole binary.

## Performance

Not benchmarked, for the same reason as every other subsystem this
project hasn't measured under real load yet: none exists. Each
`Processor.Process` call is a handful of map lookups and atomic
increments (`client_golang`'s own well-optimized counter/histogram
implementation) per event — cheap in isolation, the same category of
"probably fine, not measured" every other per-event processor in this
project already is.

## Security

`/metrics` is served in plaintext over HTTP with no authentication —
appropriate for a trusted network Prometheus itself reaches over (the
standard deployment assumption for Prometheus scrape targets
generally, not a gap specific to this project), not one exposed
publicly. The metric values themselves are aggregate counts and
byte/latency sums — no individual event content, no attribute values
beyond the small bounded label set above, so nothing here exposes more
than what's already visible in this project's own log output.

## Limitations

- No authentication or TLS on `/metrics`.
- No HTTP request/response *latency* metric: `internal/httpvis`
  observes a request line and a response line as two independent
  events (see `docs/design/http-visibility.md`); correlating them
  into a latency the way `internal/dns` genuinely can (via a wire
  transaction ID) isn't something this project's HTTP capture
  supports today, so this package doesn't claim a number it can't
  honestly compute.
- No per-process or per-container labels on any metric — every label
  is either an event-type discriminator or a small enum, not an
  unbounded identity dimension, per the cardinality reasoning in
  Tradeoffs.
- Metrics are per-instance only; nothing in this project provides a
  pre-aggregated, cluster-wide metrics view — that's Prometheus's own
  job once it scrapes every instance, not something built here.
