# Network Connection Telemetry

Package: [`internal/network`](../../internal/network). eBPF program:
[`bpf/programs/tcp_connect.c`](../../bpf/programs/tcp_connect.c). Builds
on the lifecycle proven by [`internal/ebpf`](../../internal/ebpf) — see
[eBPF Foundation](ebpf-foundation.md) and
[Process Discovery](process-discovery.md), which this package's shape
closely follows.

## Problem

Knowing what's running on a host (Day 04) is half the picture; knowing
what it talks to is the other half. Network connection telemetry answers
"which process connected from where to where, and did it succeed" — the
raw material a service dependency graph (Day 16) is eventually built
from, though building that graph is explicitly not this day's job (see
"What's deliberately not here yet").

## Design

```
tcp_connect.c (kernel)                    internal/network (userspace)
└── fexit/tcp_v4_connect ─→ ringbuf ─→    ├── decodeRawEvent — wire bytes → rawEvent (pure, tested)
    (sock_common CO-RE,                   ├── Loader.Read()  — rawEvent → ConnectEvent (wall-clock time)
     no correlation map needed)           └── ToEvent()      — ConnectEvent → pkg/model.Event (pure, tested)
```

**fexit, not kprobe/kretprobe.** The obvious first design for "capture a
function's arguments *and* its return value" is a kprobe on entry (stash
the arguments) paired with a kretprobe on return (read the return value,
look up the stashed arguments by PID/thread in a correlation map). This
project doesn't do that. `SEC("fexit/tcp_v4_connect")` — a BTF-based
"tracing" program — gets both `tcp_v4_connect`'s original arguments
*and* its return value in one BTF-typed callback
(`BPF_PROG(fn, struct sock *sk, struct sockaddr *uaddr, int addr_len, int ret)`),
with no correlation map, and no kprobe/kretprobe pair to keep in sync.
It also sidesteps a real complication kprobes would have introduced:
reading a kprobe's arguments off `struct pt_regs` requires either
per-architecture register macros (`PT_REGS_PARM1`, needing an
architecture-specific target define this project's default generic
`bpfel`/`bpfeb` bpf2go target doesn't set) or vendoring more of libbpf's
CO-RE read machinery than this program otherwise needs. fexit's `sk`
argument arrives already correctly typed, and — because it's a
BTF-trusted pointer, not one obtained via a helper like
`bpf_get_current_task()` — its fields can be read directly rather than
through `bpf_probe_read_kernel`, the same benefit `tcprtt.c` (the
upstream cilium/ebpf example this design was checked against) gets from
using `fentry` for a similar case.

**Minimal CO-RE for `sock_common`**, same pattern as `process.c`'s
`task_struct`: only the four fields actually read
(`skc_daddr`, `skc_rcv_saddr`, `skc_dport`, `skc_num`), each preserved
via `__attribute__((preserve_access_index))`, no `vmlinux.h`.

**Byte order is genuinely mixed, not a bug.** This is the part most
worth getting right and easiest to get wrong:

| Field                        | Kernel type | Byte order                                   |
|-------------------------------|-------------|-----------------------------------------------|
| `saddr`, `daddr`               | `__be32`    | Network byte order — same left-to-right octet order `netip.AddrFrom4` expects. Read as raw bytes, no conversion. |
| `dport` (`skc_dport`)          | `__be16`    | Network byte order — big-endian. |
| `sport` (`skc_num`)            | `__u16`     | **Host** byte order — the kernel compares the local port directly in hot paths, so it's kept native rather than network-order. |

Getting the last row wrong (treating `skc_num` as big-endian, matching
`skc_dport`) would produce a plausible-looking but wrong port number for
every event — exactly the kind of bug that's easy to ship unnoticed
without a test checking against a real, independently-known value. See
Tests below for how this was actually verified, not just reasoned
through.

**`internal/ebpf.MonotonicReference`, shared with `internal/process`.**
Both packages need the identical `bpf_ktime_get_ns()`-to-wall-clock
conversion for the identical reason (see
[process-discovery.md](process-discovery.md#design)). Day 04 called
this duplication acceptable at two loader implementations; a second,
byte-for-byte identical need for this *specific* piece of logic crossed
the line from "small loader-shape duplication" to "the same non-obvious
correctness logic maintained in two places," so it moved to
`internal/ebpf` as `MonotonicReference`, used by both. The loaders
themselves (`Load`/`Attach`/`Read`/`Close`) remain separate, un-shared
implementations — see process-discovery.md's Tradeoffs for why that
part still isn't generalized.

**`internal/ebpf.HaveTracing`, added alongside this program.**
`tcp_connect.c` needs BPF ring buffers and fentry/fexit ("tracing")
program support — a different, newer (5.5+) kernel requirement than
`CheckSupport`'s ring-buffer-plus-tracepoint check, which
`foundation.c` and `process.c` still use unchanged. Folding tracing
support into `CheckSupport` would have made those two fail on a kernel
new enough for tracepoints but not fentry/fexit — a real regression —
so `internal/ebpf` gained `HaveRingBuffer`, `HaveTracepoints`, and
`HaveTracing` as the specific checks, with `CheckSupport` kept as the
convenience wrapper Day 03/04's callers already use.

## What's deliberately not here yet

- **No service dependency graph or topology aggregation.** That's
  Day 16's stated deliverable. This package's job is producing events
  with everything such a graph would need (source, destination, ports,
  process) — not building the graph itself.
- **No IPv6.** `tcp_v6_connect` is a separate kernel function with a
  different socket address layout; supporting it is an additive follow-
  up, not a redesign, when it's needed.
- **No container/pod/namespace/service identity**, same deferral as
  Day 04, to Day 08.
- **No byte counters or connection duration** — that's socket data
  telemetry, Day 06's stated deliverable, which this program's `sk`
  pointer could feed without needing a new attach point.

## Tradeoffs

- **One connect program, not a family of socket lifecycle programs.**
  `tcp_v4_connect` alone answers "who connected to what" — it says
  nothing about data transferred or how the connection ended. That's
  intentional scoping to this day's stated deliverable, not an
  oversight; Day 06 is where the next piece attaches.
- **Failed connects are reported, not filtered.** `success` reflects
  `tcp_v4_connect`'s return value as-is (`ECONNREFUSED`, `ETIMEDOUT`,
  etc. all just mean `ret != 0`). A caller building topology from this
  data needs to decide what a failed connect means for a graph edge —
  that's downstream interpretation, not this package's job.

## Failure modes

Identical shape to `process-discovery.md`'s Failure modes — non-Linux
platform, unsupported/under-privileged kernel, ring buffer full,
unexpected vs. shutdown-triggered read failure — with `HaveRingBuffer`
and `HaveTracing` (not `CheckSupport`) as the specific checks `Load`
performs first. See that document for the full list; it isn't repeated
here since none of it differs for this package.

## Performance

Not benchmarked. `tcp_v4_connect` fires once per outbound IPv4 TCP
connection attempt system-wide — again, no synthetic benchmark exists
yet that would mean anything; revisit alongside Day 07's pipeline or
Day 21's performance pass.

## Security

Captured fields are connection metadata (addresses, ports, a command
name, success/failure) — never payload contents, consistent with the
project's "prefer metadata over sensitive application contents"
principle. No new privilege requirement beyond what loading any eBPF
program here already needs (root, or `CAP_BPF`+`CAP_PERFMON`) — see
`docs/design/ebpf-foundation.md`'s Security section.

## Limitations

- IPv4 only (see above).
- Outbound connect only — this doesn't observe inbound connections
  accepted via `accept()`/`accept4()`, which is a different kernel path
  entirely and not part of this day's scope.
- `success` reflects `tcp_v4_connect`'s immediate return value. For a
  non-blocking socket, `connect()` legitimately returns `EINPROGRESS`
  (i.e. `success == false`) even when the connection goes on to
  complete asynchronously moments later — this program does not track
  that later completion. Treat `success == false` as "did not complete
  synchronously," not "definitely failed," for non-blocking callers.
