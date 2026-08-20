# Event Pipeline

Package: [`internal/pipeline`](../../internal/pipeline). Consumed by
[`internal/agent`](../../internal/agent) (`process.go`, `network.go`,
`socket.go`), replacing the hand-rolled read/log loops those three files
each had through Day 06.

## Problem

By Day 06, `internal/agent` had three capabilities (process discovery,
network connect, socket close), each with its own read loop, its own
normalize-and-log logic, and — only for socket data — its own bounded,
drop-on-full channel. That's real, demonstrated duplication, and Day 06's
bounded buffer was explicitly scoped as a stopgap for exactly this day.
This is the day that duplication gets resolved into one shared,
properly tested piece of concurrency machinery, used by all three.

## Design

```
EventSource            bounded queue           Workers × EventProcessor
(one per capability,   (Config.QueueSize,       (Config.Workers goroutines,
 in process.go/        real backpressure:        each running every
 network.go/           blocks, doesn't drop)     processor over each event)
 socket.go)
     │                        │                          │
     └── Pipeline.read() ────►│◄──── Pipeline.work() ────┘
         (1 goroutine)                (N goroutines)
```

**`EventSource`/`EventProcessor` as the only two interfaces.** Section 5
of the project's engineering standard names `EventSource`,
`EventDecoder`, `EventEnricher`, `EventProcessor`, `EventExporter` —
one per pipeline stage — but also warns against interfaces that exist
"only to satisfy 'clean architecture.'" Decoding and normalizing are
already real, tested, working code in `internal/process`,
`internal/network`, and `internal/socket` — turning them into formal
`EventDecoder`/`EventNormalizer` interfaces today would describe
something that already works, not enable something new. `EventSource`
and `EventProcessor` are different: they're the two seams genuinely
shared across three (and growing) capabilities, where a real interface
buys real reuse. `EventEnricher`/`EventExporter` aren't defined yet for
the same reason decoding wasn't turned into an interface — nothing
needs them yet (enrichment starts Day 08, export Day 13).

**Adapters live with the domain, not the pipeline.** `internal/pipeline`
imports nothing from `internal/process`/`network`/`socket` — it doesn't
know they exist. Each of `internal/agent/process.go`,
`network.go`, and `socket.go` defines a small `xSource` type
adapting its own `Loader` + `ToEvent` to `pipeline.EventSource`. This
keeps the pipeline package honestly generic (it's tested entirely
against fakes, see `pipeline_test.go`) and keeps domain-specific logic
where it's already tested, rather than centralizing normalization calls
in a package that has no business knowing what a `ProcessEvent` is.

**Real backpressure, not drop-on-full.** Day 06's bounded channel
dropped an event when full, trading data loss for a bound and a simple
policy. `Pipeline.read` blocks on sending to a full queue instead — see
`pipeline.go`'s doc comment on `Run`. The consequence is that a stalled
consumer stops reads on the underlying `EventSource` (i.e. stops
draining the kernel ring buffer), which is the ring buffer's problem to
absorb via its own fixed capacity and existing drop-on-full policy
(every `bpf/programs/*.c` file already handles this) — backpressure
here means "make the slowness visible upstream," and the kernel program
is where a real, unavoidable event loss decision already lives, not a
second, redundant one in userspace.

**Graceful shutdown depends on well-behaved processors.**
`EventProcessor.Process` takes a `ctx` specifically so a slow or
blocking implementation (a future exporter with a network timeout, say)
can respect cancellation. `Pipeline.Run` waits for in-flight `Process`
calls via a `sync.WaitGroup`; it does not, and structurally cannot,
forcibly abort a processor that ignores `ctx` and never returns — Go
doesn't offer safe goroutine cancellation. `TestPipeline_CtxCancelUnblocksBlockedReader`
verifies the happy path (a processor that does respect `ctx`) shuts down
promptly; there is no test for the unsupported case (one that doesn't),
because there's nothing correct to assert about it.

**`internal/agent`'s shutdown sequence, precisely.** `App.Run` cancels
nothing on its own reader — it can't, since `EventSource.Read` (backed
by a real kernel `Loader.Read`) doesn't take a `ctx` and isn't
interruptible by one. Shutdown instead goes: `ctx` is canceled →
`Run` closes every active capability's `loader`, which unblocks that
loader's in-flight `Read()` with an error (mirroring
`internal/ebpf.Loader.Close`'s contract) → the pipeline's `read` loop
sees the error and closes the queue → workers drain what's left and
return → `Run`'s `sync.WaitGroup` unblocks. Every agent-level pipeline
test (`process_test.go`, `network_test.go`, `socket_test.go`) exercises
this exact sequence via a fake loader whose `Close` unblocks a parked
`Read`, not just `pipeline_test.go`'s more abstract version of it.

## What's deliberately not here yet

- **No enrichment, correlation, or export.** `LoggingProcessor` is the
  only `EventProcessor` today. Day 08 (identity enrichment), Day 12
  (correlation), and Day 13 (OTLP export) are each expected to add their
  own `EventProcessor`, attachable to the same pipelines without
  changing `internal/pipeline` itself.
- **No per-capability queue/worker tuning**, configuration, or metrics
  export. `Workers: 2, QueueSize: 256` is currently a literal in each of
  `newProcessPipeline`/`newNetworkPipeline`/`newSocketPipeline` — making
  it configurable is natural future work, not required for the pipeline
  itself to be real and correct today.
- **No fan-in across capabilities.** Each of the three runs its own
  independent `Pipeline` rather than one pipeline reading from all three
  sources. Keeping them separate means one capability's backpressure
  can't stall another's, which seems like the right default; revisit if
  a real need for cross-capability ordering appears.

## Tradeoffs

- **One queue per pipeline, not per worker.** All workers pull from the
  same channel rather than each owning a private queue with events
  sharded across them. This is simpler and gives naturally balanced load
  (a fast worker just pulls more often) at the cost of no per-event
  ordering guarantee across workers — acceptable since nothing today
  depends on processing order.
- **Errors from `EventProcessor.Process` are logged and dropped, not
  retried or surfaced to the caller.** A `LoggingProcessor` failure isn't
  something the current system has anywhere better to route it (no dead-
  letter queue, no retry policy) — see Day 20's failure-handling day for
  where that's expected to be designed properly, once there's an
  exporting processor whose failures are worth retrying.

## Failure modes

- **Source read fails (expected, shutdown):** `read` returns without
  logging (see `internal/agent`'s shutdown sequence above); the queue is
  closed, workers drain and stop.
- **Source read fails (unexpected):** logged once at warn level with the
  error, same outcome otherwise.
- **Processor returns an error:** logged at warn level; the worker moves
  on to the next event immediately (`TestPipeline_ProcessorErrorDoesNotStopPipeline`
  proves this explicitly — a failing first processor doesn't stop a
  second one from running, and doesn't stop later events).
- **Queue full:** the reader blocks (see Design) rather than dropping or
  erroring — there is no failure here, only backpressure.
- **A processor never returns and ignores `ctx`:** `Run` never returns
  either. This is a real limitation of the contract, not a bug — see
  Design's note on graceful shutdown.

## Performance

`BenchmarkPipeline_Throughput` (`internal/pipeline/benchmark_test.go`)
measures the pipeline's own concurrency overhead — read, bounded-queue
send/receive, worker dispatch — using a fast in-memory source and a
no-op processor, isolating it from any real capture/decode/normalize
cost (measured separately per domain, e.g.
`internal/socket`'s `BenchmarkToEvent` in `docs/design/socket-data.md`).
Measured on this project's development machine (Windows, amd64, AMD
Ryzen 5 6600H — not the Linux target this code actually runs on):

```
BenchmarkPipeline_Throughput-12    2287584    817.6 ns/op    524 B/op    0 allocs/op
```

Real output from an actual run (`go test ./internal/pipeline/...
-bench=BenchmarkPipeline_Throughput -benchmem`), not an estimate. It
says nothing about kernel-side or ring-buffer-read overhead, or about
end-to-end throughput with a real `EventSource` — see Day 21's
performance engineering pass for where that number would come from.

## Security

No new surface: `internal/pipeline` moves already-normalized
`pkg/model.Event` values between goroutines in memory. It doesn't touch
the filesystem, network, or any privileged resource itself.

## Limitations

- `Workers`/`QueueSize` below 1 are silently clamped to 1 rather than
  rejected as a configuration error — reasonable for two currently-
  hardcoded literals; worth revisiting if these become user-configurable
  and a typo'd `0` should be loud instead of quietly harmless.
- No visibility into current queue depth or per-pipeline throughput at
  runtime (no metrics export yet — see "What's deliberately not here
  yet").
