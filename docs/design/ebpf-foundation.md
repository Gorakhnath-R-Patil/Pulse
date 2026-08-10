# eBPF Foundation

Package: [`internal/ebpf`](../../internal/ebpf). eBPF program:
[`bpf/programs/foundation.c`](../../bpf/programs/foundation.c). Vendored
headers: [`bpf/headers`](../../bpf/headers).

## Problem

Every later capture subsystem — process discovery (Day 04), network
connection telemetry (Day 05), socket data (Day 06), HTTP (Day 09), DNS
(Day 10) — needs the same underlying mechanics: compile a C program to
BPF bytecode, load it into the kernel, attach it to a hook point, read
the events it emits through a shared buffer, and tear all of that down
cleanly on shutdown. Building that mechanism once, correctly, with its
own tests, is cheaper than re-deriving it (and re-discovering its
failure modes) five separate times. `internal/ebpf` is that mechanism,
proven against one deliberately trivial program before anything real
depends on it.

## Design

```
Loader (internal/ebpf)
├── Load()   — CheckSupport() → rlimit.RemoveMemlock() → load program+map into kernel
├── Attach() — link.Tracepoint(...) → ringbuf.NewReader(...)
├── Read()   — blocks on the ring buffer reader, decodes one HeartbeatEvent
└── Close()  — reader.Close() → link.Close() → objs.Close(), collecting every error
```

`bpf/programs/foundation.c` attaches to the real
`syscalls:sys_enter_execve` tracepoint — a real, frequently-firing kernel
hook — and submits a real event through a real `BPF_MAP_TYPE_RINGBUF`
map. It deliberately extracts nothing from the tracepoint's actual
arguments: the event is a sequence number plus `bpf_ktime_get_ns()`.
Turning "a process executed" into telemetry is Day 04's job; this
program only proves that a byte written in the kernel arrives intact in
userspace, through the full load/attach/receive/detach cycle.

**Kernel compatibility checking** (`CheckSupport`, `compat_linux.go`)
probes the running kernel directly via `cilium/ebpf`'s `features`
package — attempting the real map/program creation the kernel would
reject if unsupported — rather than parsing `/proc/version` or
`uname()`. A kernel version number is a poor proxy for what a specific
build actually supports: distros backport features, and some configs
disable them outright. `Load` calls `CheckSupport` first and returns
before touching the kernel further if it fails.

**Cross-platform shape.** eBPF is Linux-only, but the rest of this Go
module (and every other binary in this repo) is not. `internal/ebpf`
splits into `_linux.go` files (the real implementation) and `_other.go`
files (`//go:build !linux` stubs with an identical method set that fail
immediately with `ErrUnsupportedPlatform`). This means code elsewhere
that holds a `*ebpf.Loader` needs no build tags of its own — it calls
`Load`/`Attach`/`Read`/`Close` and gets a clear error on non-Linux,
rather than the package failing to compile. `HeartbeatEvent` and its
decoder (`event.go`) have no OS dependency and are the same file on every
platform.

**Ring buffer, not perf buffer.** `BPF_MAP_TYPE_RINGBUF` (Linux 5.8+) is
the modern replacement for the older per-CPU perf event buffer: single
shared buffer, no per-CPU event reordering to do in userspace, simpler
consumer. `CheckSupport` checks for it explicitly rather than falling
back to perf buffers on older kernels — see Limitations.

## What's deliberately not here yet

- **No CO-RE / `vmlinux.h`.** `foundation.c` never reads a kernel struct
  field, so it needs no BTF-based relocation and no generated
  `vmlinux.h`. `bpf/headers/common.h` is a small, hand-vendored stand-in
  covering just the types this program uses. The day a program needs to
  read `task_struct` or similar (likely Day 04), that day introduces
  CO-RE and a real `vmlinux.h`, not this one.
- **No wiring into `pulse-agent`.** `internal/ebpf` is a standalone,
  independently tested package today. Day 04 (process discovery) is the
  day something in `cmd/pulse-agent` actually starts a loader for a real
  purpose and normalizes its output into `pkg/model.Event`.
- **No multi-program / multi-map management.** One program, one map.
  A registry or manager for multiple concurrently-attached programs is
  premature until there's a second program to manage.

## Tradeoffs

- **Pull-based `Read()` over a channel-based event stream.** A
  `chan HeartbeatEvent` API would look more idiomatically "Go," but it
  requires the package to own a background goroutine and decide how to
  surface per-read errors and backpressure on an unbuffered/full
  channel. `ringbuf.Reader.Read()` already blocks and already unblocks
  cleanly on `Close()`; wrapping it 1:1 in `Loader.Read()` inherits that
  behavior for free and leaves goroutine ownership to the caller (a
  future `pulse-agent` integration), which is where it belongs once
  there's a real consumption loop to design around.
- **No object pooling / zero-copy decoding.** `DecodeHeartbeatEvent`
  copies two `uint64`s out of `record.RawSample`. This is fine at the
  throughput one execve-triggered tracepoint produces; revisit if a
  future high-frequency hook (e.g. Day 06's socket data) makes allocation
  visible in a profile — not before, per the project's "measure, don't
  guess" performance principle.

## Alternatives considered

- **`perf_event_array` instead of ring buffer** — rejected: ring buffers
  are the better-supported, simpler-to-consume choice on any kernel this
  project targets (5.8+), and using them means one fewer buffer type to
  ever support.
- **cilium/ebpf's pure-Go `asm` package instead of C** — considered as a
  way to avoid needing clang at all. Rejected for anything beyond a
  spike: the project's stated technology principle is C for eBPF
  programs, and every subsystem from Day 04 onward will need real kernel
  struct access that the asm package doesn't make pleasant. Decided in
  conversation with the project owner before implementation — see the
  daily log rather than repeating it here.
- **libbpf-dev system package instead of vendored headers** — rejected:
  pins the build to whatever libbpf version a given distro/CI image
  ships, which drifts. Vendoring the ~4 headers `foundation.c` actually
  needs (see `bpf/headers/README.md`) makes the build reproducible with
  only `clang` as an external requirement.

## Failure modes

- **Non-Linux OS:** every `Loader` method fails immediately with
  `ErrUnsupportedPlatform`. No partial state, nothing to clean up.
- **Linux, incompatible/misconfigured kernel:** `Load` fails with
  `ErrUnsupportedKernel` before loading anything (`CheckSupport` runs
  first). Note `CheckSupport`'s probes themselves require sufficient
  privilege to attempt a map/program creation; on a kernel with
  `unprivileged_bpf_disabled` set, an unprivileged caller sees a failure
  here that reflects "insufficient privilege," not necessarily "kernel
  too old" — the two aren't reliably distinguishable from a failed probe
  alone (see `internal/ebpf/loader_linux_test.go`'s
  `TestCheckSupport_Linux`).
- **Insufficient privilege to load (root/CAP_BPF+CAP_PERFMON missing):**
  `Load` fails with a wrapped error from the kernel (typically `EPERM`),
  not a Pulse-specific sentinel — this is a real operational condition
  Pulse should surface as-is, not mask.
- **`Attach` called before `Load` succeeds, or `Read` before `Attach`:**
  both fail fast with `ErrNotLoaded` without touching the kernel.
- **Ring buffer full:** `foundation.c` drops the event and returns; there
  is no backpressure path from kernel back to the producer. Pulse never
  blocks the traced process on telemetry.
- **Shutdown while `Read` is blocked:** `Close()` closes the ring buffer
  reader first, which unblocks a pending `Read()` with an error wrapping
  `ringbuf.ErrClosed` — verified by
  `TestLoader_CloseInterruptsRead`.
- **Partial `Load`/`Attach` failure:** `Close()` only releases resources
  that were actually acquired (tracked via `loaded`/`attached` flags),
  and collects every cleanup error via `errors.Join` rather than
  stopping at the first — no leaked program/map/link file descriptors.

## Performance

Not benchmarked — there is no realistic workload yet (one tracepoint,
one trivial payload). Real throughput/overhead measurement starts
mattering once a subsystem generating meaningful event volume exists
(Day 05+); see the project's benchmarking principle.

## Security

- The foundation program reads no process arguments, no memory, no
  credentials — only `bpf_ktime_get_ns()` and a kernel-side counter.
  There is nothing sensitive to leak from this specific program.
- Loading it still requires the same kernel-level privilege any eBPF
  program requires (root or `CAP_BPF`+`CAP_PERFMON` depending on
  kernel version) — this package does not lower that bar, and
  `docs/security/threat-model.md` (introduced Day 22) will document
  Pulse's privilege requirements in full once there's more than one
  program's worth of requirements to describe.
- `rlimit.RemoveMemlock()` raises a process-wide resource limit; see its
  doc comment in `loader_linux.go` for why this is safe and effectively
  a no-op on kernels 5.11+.

## Limitations

- Requires Linux 5.8+ (BPF ring buffer support) and either root or
  `CAP_BPF`+`CAP_PERFMON`. Older kernels or unprivileged execution are
  not supported by this package and are reported as such via
  `ErrUnsupportedKernel`, not silently degraded to a different transport
  (e.g. perf buffers) — see Alternatives.
- Requires `clang` (with the BPF target LLVM ships by default) to build
  from source. There is no pre-built fallback; this is a genuine build-
  time dependency for anyone compiling `pulse-agent` from source on
  Linux, documented in `docs/development/getting-started.md`.
- Generated bindings (`internal/ebpf/foundation_bpfel.go` /
  `foundation_bpfeb.go`) are not committed — see ADR-007 in
  `docs/design/decisions.md`. `go generate ./...` must be run (with
  `clang` available) before `go build`/`go test` will succeed for this
  package on Linux.
