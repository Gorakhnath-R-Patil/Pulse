# Telemetry Event Model

Package: [`pkg/model`](../../pkg/model). Wire contract:
[`proto/pulse/v1/event.proto`](../../proto/pulse/v1/event.proto).

## Problem

Pulse will eventually capture kernel-level activity from several
independent sources — process lifecycle (Day 04), TCP connections
(Day 05), socket byte counters (Day 06), HTTP (Day 09), DNS (Day 10) —
each via its own eBPF program and its own decoding logic. Every later
pipeline stage (enrichment, correlation, aggregation, export — Day 07
onward) needs to operate on those disparate sources through one stable
shape, or every stage would need to know about every source's raw
format. `pkg/model.Event` is that shape: the single record type
everything downstream of "decode" is written against.

## Design

`Event` is a flat envelope plus two optional, focused sub-structures:

```
Event
├── ID          string              — unique per event instance (model.NewID())
├── Type        string              — "<domain>.<action>", e.g. "process.start"
├── Timestamp   time.Time           — when the activity occurred
├── Host        string              — machine the activity was observed on
├── Process     *Process (optional) — which process, if known
│   ├── PID         int32
│   ├── Executable  string
│   └── Command     string
├── Network     *Network (optional) — network endpoints, if applicable
│   ├── Protocol     string
│   ├── Source        Endpoint{Address, Port}
│   └── Destination   Endpoint{Address, Port}
└── Attributes  map[string]string (optional) — domain-specific detail
```

`Process` and `Network` are pointers so "not applicable" (e.g. a process
event has no network endpoints) is representable as `nil` rather than an
ambiguous zero-valued struct. Each has its own `Validate()`, called by
`Event.Validate()` only when non-nil, and independently unit-tested.

`Attributes` is a deliberate escape hatch: as HTTP, DNS, and other
protocol-specific detail gets captured in later days, most of it will
live here (`attributes["http.method"] = "GET"`) rather than as new
top-level `Event` fields. This is what keeps the envelope itself stable
as new telemetry domains are added — see "Maintaining compatibility"
below.

`Type` is a free-form `"<domain>.<action>"` string, format-validated
(lowercase, dot-separated) but not restricted to a fixed vocabulary — the
model doesn't know what event kinds exist, since none are implemented
yet. Each subsystem that introduces a domain (Day 04's `process.*`,
Day 05's `network.*`, ...) defines and documents its own values when it
starts emitting them.

## What's deliberately not here yet

Section 6 of the project's engineering standard is explicit: "do not add
every field on Day 1 [/ ahead of the functionality that needs it]." Several
fields listed in that section's long-term event model are intentionally
absent from `Event` today because the subsystem that gives them meaning
hasn't been built:

| Field(s)                              | Deferred to                                    | Why                                                                 |
|----------------------------------------|-------------------------------------------------|----------------------------------------------------------------------|
| `namespace`, `pod` (friendly name), `service` | Day 19 — Kubernetes Support           | Resolvable only via the Kubernetes API (cgroup membership alone reveals a pod's UID, not its name or namespace — see `Container`'s doc comment); Day 19 is where a Kubernetes client is introduced. |

`bytes` (sent/received) was in this table through Day 05; it's now
`Network.BytesSent`/`BytesReceived`, added Day 06 as that day's own new
capability — see `docs/design/socket-data.md`. `container_id` was in
this table through Day 07; it's now `Process.Container.ID` (plus
`PodUID`, the one Kubernetes-adjacent fact resolvable without the API),
added Day 08 — see `docs/design/service-identity.md`.
| `latency`, `status`                    | Day 09 — HTTP Visibility               | Meaningful once there's a protocol (HTTP) they describe.            |
| `trace_id`, `span_id`                  | Day 11 — Distributed Trace Model      | Day 11's stated deliverable; correlating events into traces is a distinct concern from describing one event. |

Each will be added as an additive, non-breaking field (or, for HTTP/DNS
specifics, likely via `Attributes` — see above) when its day arrives.

## Tradeoffs

- **Flat `Process`/`Network` structs over a `oneof`-style typed payload.**
  A more OTel-like design would make `Event` a generic envelope wrapping
  a type-specific payload (`ProcessEvent`, `NetworkEvent`, ...) selected
  by `Type`. That's more precise but adds a layer of indirection with no
  current benefit — today there's exactly one envelope shape and two
  optional sub-structures, not a large type hierarchy. Revisit if the
  number of mutually-exclusive shapes grows large enough that "everything
  is optional on one struct" stops being readable.
- **String `Type` over a Go enum / protobuf enum.** A `enum EventType`
  would give compile-time-checked values, but Pulse doesn't know its
  vocabulary yet — it's defined incrementally, domain by domain, over the
  next several days. A string with a format contract, not a fixed value
  set, matches that reality; an enum can be introduced later if the
  vocabulary stabilizes and cross-language safety matters more than
  extensibility.

## Alternatives considered

- **OpenTelemetry's data model directly** (Span/Metric/Log with a common
  `Resource`/`Attributes` shape) — rejected for now as more machinery
  than today's needs justify; Pulse's `Attributes` bag borrows the same
  idea at much smaller scope. OTLP compatibility is an explicit later
  goal (Day 13), so this may converge rather than diverge over time.
- **Separate Go types per event kind** (`ProcessStartEvent`,
  `TCPConnectEvent`, ...) with no shared envelope — rejected because
  every pipeline stage (queueing, batching, export) would need a type
  switch or a common interface reinventing what a shared envelope already
  gives for free.

## Failure modes

- **Malformed input (bad JSON):** `Unmarshal` returns a wrapped error;
  never a zero-value `Event` presented as success.
- **Well-formed but incomplete input** (valid JSON missing a required
  field): decodes successfully — JSON decoding has no way to know a field
  was "required" — but `Validate()` reports it. Decode and validate are
  intentionally separate steps (matching `internal/config`'s pattern) so
  callers can choose to log/quarantine invalid-but-parseable events
  rather than treat a decode error and a validation error as the same
  failure.
- **Unknown `Type` values:** not a failure at all — the model has no
  fixed vocabulary to violate, only a format to match.

## Performance

Not benchmarked. `Event` is a small, mostly-flat struct; the two pointer
fields and the `Attributes` map are the only heap-allocating parts on an
otherwise stack-friendly shape. No claims are made about allocation or
throughput characteristics until Day 07's pipeline benchmarks give a real
workload to measure against — see the project's "no fake engineering"
principle.

## Security

`pkg/model` holds metadata shape, not policy — it doesn't decide what
gets captured, only how a captured fact is represented. Two things worth
noting for later days that populate it: `Process.Executable`/`Command`
and `Network.*.Address` are metadata (paths, addresses), not payload
contents, consistent with the project's "prefer metadata over sensitive
application contents" principle; and `Attributes` is a free-form string
map, which means it is also the place future capture code could
accidentally put something sensitive (e.g. a raw HTTP header) — subsystems
populating `Attributes` are responsible for only putting metadata there,
and this should be revisited explicitly in `docs/security/threat-model.md`
once a subsystem actually writes to it (Day 09+).

## Limitations

- No fixed event-type vocabulary yet (by design — see above).
- No schema versioning strategy yet. Since every field added so far is
  additive and optional, old and new `Event` values round-trip through
  JSON without conflict; a versioning strategy becomes necessary once a
  field is ever removed or its meaning changes, not before.
- The `.proto` contract is hand-maintained in parallel with the Go
  struct, not generated from a single source of truth — see ADR-006 in
  `docs/design/decisions.md`.
