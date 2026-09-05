# DNS Telemetry

Package: [`internal/dns`](../../internal/dns). eBPF program:
[`bpf/programs/dns_telemetry.c`](../../bpf/programs/dns_telemetry.c).
Builds on [eBPF Foundation](ebpf-foundation.md) and the tracepoint
preference explained in ADR-008 (`decisions.md`), which this program
also follows.

## Problem

A service that resolves a domain name depends on whatever that name
points to, every bit as much as one that opens a TCP connection to a
literal IP — DNS resolution is often the *first* sign a dependency
exists at all, before any connection to the resolved address is even
attempted. Capturing it, with real latency, is this day's job.

## Design

```
dns_telemetry.c (kernel)                   internal/dns (userspace)
├── sys_enter_sendto (query)   ─┐          ├── decodeRawEvent    — wire bytes → rawEvent (pure, tested)
├── sys_enter/exit_recvfrom    ─┼→ ringbuf → ├── parseMessage      — rawEvent.Data → DNS header + first question (pure, tested)
│   (response; needs a pending  │          ├── queryCorrelator   — matches a response to its query by (pid, txn ID) (pure, tested)
│   -recvfrom map — see below)  │          └── ToEvent()          — DNSEvent → pkg/model.Event (pure, tested)
└── shared ring buffer  ────────┘
```

**Why responses need a correlation map and queries don't.**
`sendto(2)`'s buffer already holds the query at the moment the syscall
is entered — there's something to capture right there. `recvfrom(2)`
is the reverse: its buffer is *empty* at entry and only gets filled in
by the kernel once the call returns, and a `sys_exit_*` tracepoint's own
context carries just a return value, not the original arguments (see
`trace_event_raw_sys_exit` in the program). So the buffer pointer
captured at `sys_enter_recvfrom` is stashed in a small hash map keyed
by the calling thread's `pid_tgid`, and looked back up (and read, now
that it's populated) at `sys_exit_recvfrom`. This is the same shape as
a kprobe/kretprobe correlation map — just built on tracepoints, per
ADR-008.

**Query/response latency, computed for real — and why this can be
done here but not for HTTP (Day 09).** Every DNS message carries a
16-bit transaction ID specifically so a resolver can match a response
to the query it answers, even amid other in-flight queries on the same
socket. `queryCorrelator` uses exactly that: it remembers a query's
timestamp keyed by `(PID, transaction ID)`, and a matching response
looks it up and computes a real latency. HTTP's Day 09 explicitly
*deferred* latency to Day 12 because HTTP/1.1 has no equivalent
protocol-native pairing key — matching a request to its response there
means tracking connection state across multiple, possibly-reordered
writes. DNS's transaction ID makes this a same-day, protocol-specific
mechanism instead of the general cross-event correlation Day 12 is
for — see that day's own scope for where the line actually is.

**No kernel-side port filtering — everything captured is filtered in
userspace instead.** A UDP socket `connect()`ed once to a resolver and
then used with plain `send()`/`recv()` never passes a destination
address to `sendto`/`recvfrom` at all, so filtering on port 53 in the
kernel program would miss exactly that case. Every UDP `sendto`/
`recvfrom` payload is captured and handed to `parseMessage`; whatever
doesn't parse as a plausible DNS message is silently discarded in
`Loader.Read`. See Tradeoffs for the overhead this accepts.

## What's deliberately not here yet

- **Service dependency graph construction.** Same deferral as
  `network-connect.md`: this package produces the events such a graph
  needs (a name, a process, latency), not the graph itself — Day 16.
- **Answer record contents** (resolved IP addresses, CNAME chains).
  Only the question section and the header-level counts/response code
  are parsed; walking the answer section is a bounded, additive
  extension whenever there's a reason to.
- **DNS over TCP.** Large responses can fall back to TCP; this package
  only observes UDP. A real, documented gap, not solved by reusing
  `internal/httpvis`'s `write()` hook, since a TCP DNS message is
  length-prefixed differently than either HTTP text or a raw UDP
  datagram.

## Tradeoffs

- **Every UDP `sendto`/`recvfrom` payload system-wide is captured**,
  not just DNS's. This is a direct consequence of not being able to
  filter reliably on port 53 in-kernel (see Design) — accepted for the
  same reason Day 09 accepted capturing every `write()` call: UDP
  send/receive volume is typically far lower than generic file/socket
  writes, and the alternative (attempting in-kernel content heuristics
  on essentially-random-looking binary header bytes) would be far less
  reliable than HTTP's distinctive text prefixes were.
- **Unanswered queries are silently forgotten**, not reported as
  timeouts. `queryCorrelator` evicts a query after 5 seconds specifically
  to bound its own memory, not to produce a "no response" event —
  reporting that meaningfully would need a background sweep this
  package's synchronous, pull-based `Read()` doesn't have a natural home
  for. A real gap, not a silent one: documented here, not papered over.
- **Only the first question is parsed** (see `parseMessage`'s doc
  comment) — virtually every real-world query has exactly one, and
  DNS's compression scheme means a second question's name could be a
  pointer back into the first, which this package doesn't resolve
  either (see next point).
- **Label compression is not resolved.** A compressed name pointer
  appearing in the *first* question (immediately after the fixed
  12-byte header, with nothing earlier in the message to sensibly point
  back to) is treated as unparseable rather than followed — this
  should essentially never happen for a first question in practice, so
  it costs little, and resolving compression correctly means being able
  to jump backward through the message, real complexity this package's
  scope doesn't need yet.

## Failure modes

Same shape as every other program here: non-Linux platform or
unsupported/under-privileged kernel fails `Load` before touching the
kernel; ring buffer full silently drops the event; `Close` interrupts a
blocked `Read`. Specific to this package: a captured UDP payload that
doesn't parse as DNS (see `parseMessage`'s sentinel errors) is silently
skipped inside `Loader.Read`, exactly like `internal/httpvis`'s
unparseable-line skip — expected given no kernel-side DNS filtering,
not surfaced as an error.

## Performance

`BenchmarkParseMessage` and `BenchmarkToEvent` (`internal/dns`) measure
the two pieces of this package's per-event work that are platform-
independent and worth measuring on any machine — decoding a message's
header and question name, and building the final `pkg/model.Event`.
Run them with `go test ./internal/dns/... -bench=. -benchmem`; this
document doesn't repeat specific numbers here, since they're a
property of whatever machine ran them, not a fixed claim about the
system — see the project's "no fake engineering" principle. Neither
benchmark measures kernel-side overhead or ring buffer read latency;
see Tradeoffs above for the overhead question that actually matters
for this package (capturing every UDP payload system-wide) and why it
hasn't been measured yet.

## Security

Captured fields (a queried domain name, query type, response code,
answer count, latency, a truncated command name) are metadata, not
payload contents beyond the query name itself — which is, definitionally,
what a domain name is: the thing being asked for, not private data
incidentally swept up. No new privilege requirement beyond loading any
eBPF program here (root, or `CAP_BPF`+`CAP_PERFMON`).

## Limitations

- UDP only (no DNS-over-TCP, no DoH/DoT — see What's deliberately not
  here yet).
- Only the first question and header-level response fields are parsed;
  answer records themselves are not.
- Unanswered queries are not reported at all, not even as a timeout.
- A compressed name in the first question is unparseable, not resolved
  (expected to be rare to nonexistent in practice — see Tradeoffs).
