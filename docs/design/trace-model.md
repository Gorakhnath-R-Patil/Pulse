# Distributed Trace Model

Data model: [`pkg/model.Span`](../../pkg/model/trace.go). Assembly
logic: [`internal/tracing`](../../internal/tracing).

## Problem

A single `pkg/model.Event` — a process starting, a connection opening,
an HTTP line, a DNS lookup — describes one fact at one instant. A
distributed trace answers a different question: *which of these facts,
across however many services, were part of one causally-related chain
of operations, and how did they nest?* That needs its own data model
(identity, parent linkage, duration) before it needs anything else —
which raw events actually belong to which trace is a separate,
harder problem, deliberately left to Day 12.

## Design

```
pkg/model (data)                          internal/tracing (behavior)
├── TraceID, SpanID — random 128/64-bit    └── AssembleTrace(spans) → Trace
│   values, hex-encoded (W3C Trace              ├── groups by ParentSpanID
│   Context / OTLP format)                     ├── Roots: no parent at all
└── Span{TraceID, SpanID, ParentSpanID,          ├── Orphans: parent specified,
        Name, Service, StartTime, EndTime,           but not among the given spans
        Attributes}                          └── Children of each node, and
                                                  Roots/Orphans, sorted by StartTime
```

**Data lives in `pkg/model`, assembly logic lives in
`internal/tracing`** — the same split Day 02 established between
`pkg/model.Event` (data) and `internal/process`/`internal/network`/etc.
(the logic that produces it). `Span` is data anyone can construct;
`AssembleTrace` is the one piece of real logic this day introduces.

**Not OpenTelemetry's data model.** `docs/design/event-model.md`'s
Alternatives section already declined to adopt OTel's Span/Resource
model wholesale, as more machinery than was needed at the time — that
reasoning still holds today. `Span` here has exactly what this day's
scope calls for (identity, parent linkage, timing, service identity,
attributes) and nothing OTel has that Pulse doesn't yet need (span
kind, span status, span links, resource attributes as a separate
concept from span attributes). OTLP compatibility remains a later,
explicit goal (Day 13) this may converge toward, not a constraint
today. ID *format* is the one place this deliberately anticipates that
future: 128-bit trace IDs and 64-bit span IDs, hex-encoded, are exactly
the W3C Trace Context / OTLP wire format, even though nothing parses
or emits that wire format yet — free interoperability later, for the
cost of picking sizes now rather than arbitrarily.

**`TraceID`/`SpanID` are not new fields on `Event`.** The natural first
instinct is to add `TraceID string` and `SpanID string` to
`pkg/model.Event` itself. This was deliberately not done: an `Event` is
raw, kernel-observed fact — a process started, a socket connected — and
has no inherent trace membership at capture time; Day 12 is what
decides whether and how a group of events becomes a span. Putting
`TraceID` on `Event` now would either sit unset on every event this
project has produced through Day 10, or force Day 11 to also solve Day
12's problem to populate it. Keeping trace identity on the new `Span`
type instead means today's change is purely additive — nothing about
`Event` or any existing package changes at all (see Tradeoffs).

**Incomplete traces are the norm, not an edge case.** `AssembleTrace`
tolerates multiple root-like spans and orphaned spans without erroring
— it organizes whatever it's given, rather than demanding a complete
tree. This isn't a "handle this later" placeholder for Day 12's own
"handle incomplete traces gracefully" requirement — it's a genuine
design choice for *this* day's function, since even a fully-specified
set of spans (all sharing real trace/parent IDs) can be assembled from
a partial view (a span whose parent was never observed, was dropped,
or arrives out of order) well before Day 12's harder problem (deciding
which raw events belong together at all) enters the picture.

## What's deliberately not here yet

- **Correlating raw telemetry events into spans.** `AssembleTrace`
  takes `[]model.Span` — it assumes spans already exist. Turning a
  `process.start` event and a `network.connect` event into two spans in
  the same trace is Day 12's stated deliverable.
- **Trace storage or querying.** Explicitly out of scope per this day's
  own instructions ("do not implement the entire storage backend yet").
  `Trace` is an in-memory structure `AssembleTrace` returns; nothing
  persists it.
- **Cycle detection.** A malformed input where two spans each name the
  other as parent isn't specifically detected — those nodes simply end
  up reachable from neither `Roots` nor any real root's `Children`,
  effectively (silently) lost from a root-down traversal, though still
  present in `Trace.Spans`. Not implemented because nothing today
  constructs spans by hand in a way that could produce this — revisit
  once Day 12 builds spans from real, potentially messy event data.

## Tradeoffs

- **`Span` duplicates `Name`/`Service`/timing rather than embedding
  `Event`.** A span could instead hold `[]model.Event` (its evidence)
  plus derive name/timing from them. `Span` doesn't do this: it's
  meant to be constructible directly (as every test in this package
  does), independent of whatever events justified it, keeping
  `internal/tracing` free of any dependency on how Day 12 chooses to
  derive a span from events. The tradeoff is that nothing here
  currently traces a span back to the raw events it came from — that
  linkage, if wanted, is Day 12's to add.
- **Multiple roots and orphans are tolerated, not errors** (see
  Design). This makes `AssembleTrace` more permissive than "assemble a
  complete tree or fail" would be — the right tradeoff for a function
  whose main consumer (Day 12) explicitly needs to handle incomplete
  data, but it does mean a genuinely malformed trace (as opposed to
  merely incomplete) produces a Trace value rather than an error;
  callers that care about the difference need to inspect `Orphans`
  themselves.

## Failure modes

- **No spans given:** `AssembleTrace` returns `ErrEmptyTrace`.
- **Spans from more than one trace:** `AssembleTrace` returns
  `ErrMixedTraceIDs` rather than silently assembling a nonsensical
  mixed tree.
- **A span whose parent isn't present:** not a failure — it's recorded
  in `Orphans`, and the trace assembles anyway.
- **More than one span with no parent:** not a failure — both appear
  in `Roots`.

## Performance

Not benchmarked. `AssembleTrace` runs once over its input (build a map,
one pass to link parents, a handful of `sort.Slice` calls each bounded
by that node's own child count) — no measurement exists yet because no
realistic trace size exists yet to measure against; revisit once Day
12 produces real spans to assemble at real volume.

## Security

`Span`/`Trace` hold identity and timing metadata, no payload contents —
consistent with the project's "prefer metadata over sensitive
application contents" principle. `Attributes` carries the same
free-form-string caveat already noted for `Event.Attributes` in
`docs/design/event-model.md`'s Security section: whoever populates a
span's `Attributes` (Day 12, eventually) is responsible for only
putting metadata there.

## Limitations

- No cycle detection (see What's deliberately not here yet).
- No linkage from a `Span` back to the raw `Event`s it might have been
  built from — a `Span` is authoritative on its own once constructed.
- `AssembleTrace` organizes one trace at a time; grouping a larger set
  of spans by `TraceID` first is the caller's job.
