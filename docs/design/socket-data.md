# Socket Data Telemetry

Package: [`internal/socket`](../../internal/socket). eBPF program:
[`bpf/programs/tcp_close.c`](../../bpf/programs/tcp_close.c). Builds on
the same shape as [Network Connection Telemetry](network-connect.md),
which this document assumes as background and doesn't re-explain.

> **Superseded by Day 07:** the hand-rolled bounded, drop-on-full
> channel this document originally described
> (`internal/agent/socket.go`'s `socketEventBufferSize`,
> `readSocketEvents`, `logSocketEvents`) was replaced by the shared
> pipeline in [`internal/pipeline`](event-pipeline.md), which applies
> real backpressure instead of dropping events. The design reasoning
> below (why bytes/errors are captured where they are, why fentry, the
> byte-order handling) is unchanged and still accurate; only the
> "introduce bounded buffering" mechanism moved — see
> [event-pipeline.md](event-pipeline.md) for its replacement.

## Problem

A connect event (Day 05) says a connection started; nothing so far says
what happened over its life. Socket data telemetry closes that loop:
when a TCP connection ends, how many bytes it moved in each direction,
and whether it ended with a pending socket error — the second half of
"who talked to whom, and how much," alongside a place to hang the
"introduce bounded buffering" and "measure event throughput" work this
day also calls for.

## Design

```
tcp_close.c (kernel)                       internal/socket (userspace)          internal/agent
└── fentry/tcp_close ─→ ringbuf ─→         ├── decodeRawEvent (pure, tested) ─→  socketSource ─→ internal/pipeline
    (sock_common + tcp_sock CO-RE,         └── ToEvent (pure, tested,                            (see event-pipeline.md)
     bpf_skc_to_tcp_sock, same                  benchmarked)
     pattern as tcp_connect.c)
```

**`fentry/tcp_close` + `bpf_skc_to_tcp_sock`**, not a new attach
strategy. This follows `tcp_connect.c`'s reasoning exactly (see
[network-connect.md](network-connect.md#design)) with one addition:
`tcp_close`'s `sk` argument is a generic `struct sock *`, but the byte
counters this day needs (`bytes_sent`, `bytes_received`) live on the
wider `struct tcp_sock` it's embedded in. `bpf_skc_to_tcp_sock(sk)` — a
real BPF helper, not a raw pointer cast — safely reinterprets it,
returning `NULL` if `sk` isn't actually backing a TCP socket. This is
the same technique `cilium/ebpf`'s own `tcprtt.c` example uses for the
identical purpose (reading `tcp_sock.srtt_us`); `tcp_close.c` follows it
closely rather than inventing a new way to get at the same struct.

**Endpoints captured again, not looked up.** `tcp_close.c` captures its
own source/destination/ports rather than trying to correlate with the
`tcp_connect.c` event that opened the same connection. Correlating two
independently-observed kernel events into one logical "connection"
record is downstream work — this package's job is making sure each
event is self-describing enough for that correlation to be possible
later, not performing it now.

**Bounded buffering, originally a stopgap, now the real pipeline.** At
the time this program was written, `internal/agent` had a single
bounded channel between draining the ring buffer and logging, dropping
(and counting) an event when a 256-capacity buffer was full — the
simplest thing that satisfied this day's "introduce bounded buffering"
requirement. Day 07 replaced it with [`internal/pipeline`](event-pipeline.md),
shared across all three telemetry capabilities, applying real
backpressure (blocking, not dropping) instead. See that document for
the current mechanism; this section is kept for the historical
reasoning behind introducing bounded buffering here in the first place.

**`Network.BytesSent`/`BytesReceived`, added to `pkg/model` today.**
Day 02's event model deliberately left these out, noting they'd arrive
"as part of [Day 06's] own new capability" — see
[event-model.md](event-model.md#whats-deliberately-not-here-yet). This
is that day.

## What's deliberately not here yet

- **No connect/close correlation or per-connection state.** See Design,
  above.
- **No Day 07 pipeline** (worker pools, multi-stage bounded queues,
  graceful shutdown coordination across stages) — today's bounded
  channel is real, working backpressure, just at the smallest scope that
  counts as "introduced."
- **No Prometheus/metrics export.** "Measure event throughput" is
  satisfied by a Go benchmark (see Performance) — real, reproducible,
  runnable by anyone who clones the repo — rather than a metrics
  pipeline that doesn't exist until Day 17.

## Tradeoffs

- **Drop-and-count over blocking or growing the buffer (as originally
  built; see the superseded note at the top of this document).** A slow
  log sink should never make the kernel ring buffer back up (which risks
  the kernel-side drops `tcp_close.c` already has no way to avoid once
  *its* buffer fills) or make the read goroutine's memory usage
  unbounded. A fixed-capacity channel with a counted drop was the
  simplest policy that kept both bounded at the time; Day 07's pipeline
  replaced it with blocking backpressure instead — see
  [event-pipeline.md](event-pipeline.md#design) for why blocking turned
  out to be the better default once there was a shared abstraction
  across all three capabilities to put it in.
- **`sk_err` reported as a raw errno-shaped integer, not decoded.**
  Translating it to a human string (`"ECONNRESET"`) is a small,
  legitimate userspace enrichment step, deferred rather than done here
  because nothing yet consumes `network.close` events in a way that
  would benefit from it — `Attributes["tcp.sock_error"]` carries the raw
  value so no information is lost while that's true.

## Failure modes

Same shape as [network-connect.md](network-connect.md#failure-modes) —
non-Linux platform, unsupported/under-privileged kernel (`HaveRingBuffer`
and `HaveTracing`, not `CheckSupport`), ring buffer full, unexpected vs.
shutdown-triggered read failure. Two additions specific to this
package:

- **`bpf_skc_to_tcp_sock` returns `NULL`:** the program returns early
  without emitting an event — `tcp_close` fires for non-TCP socket types
  too (its tracepoint isn't TCP-specific the way `tcp_v4_connect` is),
  and this is the expected, silent way of ignoring those.
- **Pipeline queue full:** as of Day 07, the reader blocks (real
  backpressure) rather than dropping — see
  [event-pipeline.md](event-pipeline.md#failure-modes). Originally (see
  Tradeoffs) this was a distinct, counted userspace drop point; that
  policy no longer applies.

## Performance

`BenchmarkToEvent` (`internal/socket/benchmark_test.go`) measures the
one piece of this package's per-event work that's platform-independent
and meaningful to benchmark on any machine: converting an
already-decoded `CloseEvent` into a `pkg/model.Event`. Measured on this
project's development machine (Windows, amd64, AMD Ryzen 5 6600H — not
the Linux target this code actually runs on, and not representative of
kernel-side or ring-buffer-read overhead):

```
BenchmarkToEvent-12    2303014    514.7 ns/op    336 B/op    11 allocs/op
```

That's real output from an actual run, not an estimate — reproduce it
with `go test ./internal/socket/... -bench=BenchmarkToEvent -benchmem`.
It says nothing about `decodeRawEvent`, ring buffer read latency, or
kernel-side overhead, and shouldn't be read as a throughput claim for
the system as a whole — see Day 21's performance engineering pass for
where an end-to-end number would come from, once there's a real
workload and a Linux benchmarking environment to run it in.

## Security

Same as [network-connect.md](network-connect.md#security): connection
metadata and byte *counts*, never payload contents. `sk_err` is a status
code, not application data.

## Limitations

- IPv4 only, outbound-initiated connections only — same scope as
  `tcp_connect.c`.
- Byte counters are the connection's cumulative totals *at close time*,
  not a stream of incremental deltas — there's no visibility into a
  still-open, long-lived connection's byte counts from this program
  alone.
- The pipeline's queue capacity and worker count are hardcoded literals,
  not yet configurable — see [event-pipeline.md](event-pipeline.md#whats-deliberately-not-here-yet).
