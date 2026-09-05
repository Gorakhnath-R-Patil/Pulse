# HTTP Visibility

Package: [`internal/httpvis`](../../internal/httpvis). eBPF program:
[`bpf/programs/http_visibility.c`](../../bpf/programs/http_visibility.c).
Builds on [eBPF Foundation](ebpf-foundation.md); see ADR-008 in
[decisions.md](decisions.md) for why this program's attach mechanism
deliberately differs from Day 05/06's.

## Problem

Process and connection telemetry (Days 04–06) say *that* a process
talked to something over TCP. Neither says *what it asked for* or
*what it got back* — the method, path, and status that make a
dependency graph (Day 16) mean something more than "A talks to B."

## Design

```
http_visibility.c (kernel)                internal/httpvis (userspace)
└── tracepoint/sys_enter_write            ├── decodeRawEvent  — wire bytes → rawEvent (pure, tested)
    ├── classify_prefix() — kernel-side   ├── parseHTTPLine   — rawEvent.Data → method/path or status (pure, tested)
    │   heuristic; non-HTTP writes        ├── Loader.Read()   — loops past records that don't parse
    │   never even reach the ring buffer  └── ToEvent()       — HTTPEvent → pkg/model.Event (pure, tested)
    └── ringbuf (bounded prefix + size)
```

**One tracepoint, both directions.** `write(2)` is what both a client
sending a request line and a server sending a response status line
ultimately call — hooking `sys_enter_write` once observes both, with
userspace telling them apart by whether the line starts with a known
method or with `"HTTP/"`. See ADR-008 for why this is `sys_enter_write`
specifically, not `tcp_sendmsg`/`vfs_write` via `fentry`.

**In-kernel filtering, not in-kernel parsing.** `classify_prefix` reads
8 bytes and checks them against `GET `, `POST `, `PUT `, and `HTTP/` —
enough to keep non-HTTP writes (the overwhelming majority of all
`write()` calls system-wide) from ever reaching the ring buffer, without
attempting real parsing in a language as constrained as BPF C. The
*actual* line parsing (`parseHTTPLine`) happens in Go, over a bounded
256-byte prefix, and is fully unit-tested against real request/response
line text — something meaningfully harder to get right and test inside
the kernel program itself.

**The raw prefix is never retained.** `Loader.Read` decodes a ring
buffer record, parses it, and returns only the extracted fields
(`Method`, `Path`, `Status`, `Size`) as `HTTPEvent` — the raw
`rawEvent.Data` byte slice goes out of scope the moment `Read` returns.
Nothing in this package logs, stores, or forwards the captured bytes
themselves. See Security below for what this does and doesn't protect
against.

**No socket/connection correlation.** Unlike `internal/network`, an
`HTTPEvent` carries no source/destination address — `sys_enter_write`
hands over a buffer and a length, not a socket. `ToEvent` accordingly
produces an `Event` with `Network == nil`. Correlating an HTTP event
with the TCP connection it rode on is a natural fit for Day 12's trace
correlation, not attempted here.

## Observed / inferred / unavailable

The project's engineering standard calls for being explicit about
which of these three every captured fact is:

| Field                    | Status          | Why                                                                 |
|---------------------------|-----------------|------------------------------------------------------------------------|
| `method`, `path`           | **Observed**    | Parsed directly from a real request line this process actually wrote. |
| `status`                   | **Observed**    | Parsed directly from a real status line this process actually wrote. |
| `http.observed_bytes`      | **Observed**    | The real `count` argument to the `write()` call — but see Limitations for what it isn't. |
| which connection this belongs to | **Unavailable** | Not resolved by this technique at all (see Design above); a `dependency` edge needs Day 12. |
| `latency`                  | **Unavailable** | No request/response correlation is attempted — see Limitations.       |
| anything over HTTPS         | **Unavailable** | TLS-encrypted bytes are exactly that at the `write()` syscall boundary; this technique cannot and does not decrypt anything. |

Nothing here is "inferred" (derived from indirect signals like port
numbers) — every field this package produces was read directly from
data the traced process itself wrote.

## What's deliberately not here yet

- **Request/response latency.** Computing this needs pairing a request
  write with its later response write — on the same connection for
  HTTP/1.1, and not even necessarily the same connection under HTTP/2
  multiplexing. That pairing is what Day 12 (trace correlation) is for.
- **`writev`/`send`/`sendto` observation.** Only plain `write()` is
  hooked — see ADR-008's Tradeoffs.
- **Header fields.** Only the request/status line is parsed; headers
  (which may follow in the same or a later `write()` call) are not
  inspected at all.

## Tradeoffs

- **A single capture-and-discard buffer of 256 bytes**, not a
  reassembly buffer across multiple `write()` calls. A request line
  longer than 256 bytes (an unusually long URL) won't parse; documented
  as a limitation rather than solved with cross-call buffering, which
  would need per-PID state the kernel side doesn't keep today.
- **`classify_prefix`'s method list is short** (`GET`, `POST`, `PUT`,
  plus the `HTTP/` response prefix) rather than exhaustive
  (`DELETE`/`HEAD`/`PATCH`/`OPTIONS`/`CONNECT`). Every check is a fixed
  number of byte comparisons regardless of list length, so this was a
  scope choice, not a performance one — extending it is a small, safe,
  additive change whenever it's actually needed.

## Failure modes

Same shape as every other program here: non-Linux platform or
unsupported/under-privileged kernel fails `Load` before touching the
kernel (`ebpf.CheckSupport`); ring buffer full silently drops the event;
`Close` interrupts a blocked `Read`. One addition specific to this
package: a ring buffer record that the kernel-side heuristic flagged
but that `parseHTTPLine` can't actually parse (truncated by the capture
bound, or a false positive) is silently skipped inside `Loader.Read`,
not surfaced as an error — see its doc comment.

## Performance

Not benchmarked. This program's overhead profile is different from
every other one here: it runs on *every* `write()` syscall system-wide
(not just HTTP-related ones), doing an 8-byte user-memory read and a
handful of comparisons before deciding whether to do anything further.
That per-syscall cost — not the cost of the (rare, filtered) events
that do reach the ring buffer — is the number worth measuring before
this runs anywhere overhead-sensitive; it hasn't been, yet.

## Security

- Extracted fields (method, a truncated command name, a status code,
  an observed byte count) are metadata. The one field worth flagging
  explicitly: **`path` is captured verbatim**, including any query
  string — a URL like `/reset-password?token=abc123` is captured whole,
  token included. This is a real gap relative to "prefer metadata over
  sensitive application contents," not an oversight; redacting query
  strings (or specific known-sensitive parameter names) is exactly the
  kind of hardening `docs/security/threat-model.md` (Day 22) should
  cover once it exists, not something papered over here with a
  half-measure.
- Header contents (cookies, `Authorization`, etc.) are never captured
  at all — only the first line of what a `write()` call sent, which for
  a well-formed request/response is the request/status line, not a
  header.
- No privilege requirement beyond loading any eBPF program here (root,
  or `CAP_BPF`+`CAP_PERFMON`).

## Limitations

- IPv4/IPv6-agnostic in a specific sense: since this observes syscall
  buffers, not socket structures, it has no address-family dependency
  at all — but also no address information whatsoever (see "What's
  deliberately not here yet").
- `http.observed_bytes` is the size of *the one `write()` call
  observed*, not necessarily the complete request or response — a
  chunked or otherwise multi-write body will under-report.
- Only cleartext HTTP is visible. HTTPS (the overwhelming majority of
  real-world web traffic) is entirely invisible to this technique — see
  the observed/inferred/unavailable table above.
- Only `write()` is hooked; `writev`/`send`/`sendto` users go
  unobserved (ADR-008).
