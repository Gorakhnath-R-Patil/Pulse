# Process Discovery

Package: [`internal/process`](../../internal/process). eBPF program:
[`bpf/programs/process.c`](../../bpf/programs/process.c). Builds on the
lifecycle proven by [`internal/ebpf`](../../internal/ebpf) — see
[eBPF Foundation](ebpf-foundation.md).

## Problem

The first thing Pulse needs to know about a host, before any network or
protocol telemetry means anything, is *what's running on it*: which
processes exist, what they're called, what spawned them, and when they
start and stop. That's process discovery — PID, parent PID, and a
command name, captured at the two moments they're knowable: when a
process execs a new image, and when it exits.

## Design

```
process.c (kernel)                    internal/process (userspace)
├── sched_process_exec  ─┐            ├── decodeRawEvent   — wire bytes → rawEvent (pure, tested)
├── sched_process_exit  ─┼→ ringbuf → ├── Loader.Read()    — rawEvent → ProcessEvent (wall-clock time)
└── shared task_struct CO-RE          ├── ResolveExecutable — best-effort /proc/<pid>/exe (start events only)
    (real_parent->tgid only)          └── ToEvent()         — ProcessEvent → pkg/model.Event (pure, tested)
```

**What's captured in-kernel vs. resolved in userspace.** PID, PPID, and
a 16-byte command name are captured *in the kernel program*, at the
moment of the event — not looked up afterward. This matters most for
process exit: a task's identity is only guaranteed to exist while the
tracepoint is firing. By the time userspace gets around to processing
the ring buffer record, the process is gone and there's nothing left to
look up. PID's parent (PPID) needs the same treatment for the same
reason, via a minimal CO-RE read of `task_struct.real_parent->tgid` (see
below) — there's no `bpf_get_current_ppid()` helper.

The executable's *full path*, by contrast, is resolved in userspace via
`/proc/<pid>/exe`, and only for `process.start` events. This is
deliberately the one field where "capture now, in-kernel" doesn't apply
the same way: getting a real path in-kernel means walking
`task->mm->exe_file->f_path` via `bpf_d_path()` — several more CO-RE
hops for one field, with no such need for PPID and comm which are one
hop or a plain helper call away. A `process.start` event's subject
process is, by definition, still alive when the tracepoint fires and,
in the ordinary case, still alive by the time userspace catches up
moments later — enough for a best-effort lookup to usually succeed. An
exit event's subject is not, so `ResolveExecutable` is never called for
one. See Limitations for what "usually" leaves out.

**Minimal CO-RE, not `vmlinux.h`.** `process.c` declares only:

```c
struct task_struct {
	int tgid;
	struct task_struct *real_parent;
} __attribute__((preserve_access_index));
```

`preserve_access_index` tells clang to emit CO-RE relocation records for
every field access on this struct, so the actual field *offsets* are
resolved against the running kernel's BTF at load time — the same
mechanism a full `vmlinux.h` would give, just declared for exactly the
two fields this program touches instead of every field of every kernel
struct. This was flagged as a likely need back in
[ebpf-foundation.md](ebpf-foundation.md) and turned out to be exactly
that, at a much smaller scope than a generated `vmlinux.h` would have
required.

**Monotonic-to-wall-clock conversion.** `bpf_ktime_get_ns()` — the same
call `foundation.c` used — returns nanoseconds since boot
(`CLOCK_MONOTONIC`), not since the Unix epoch. `pkg/model.Event.Timestamp`
needs a real `time.Time`. `Loader.Load` captures a reference pair
(`unix.ClockGettime(CLOCK_MONOTONIC)` alongside `time.Now()`) once, at
load time; `Read` converts each event's raw monotonic timestamp to
wall-clock time by adding its delta from that reference onto the
reference's wall-clock side. This is the standard technique for turning
BPF kernel timestamps into real time — see `clock_linux.go`.

**Decode → normalize, kept separate.** `decodeRawEvent` (wire bytes →
`rawEvent`, still holding a raw monotonic timestamp) and `ToEvent`
(`ProcessEvent` → `pkg/model.Event`) are both pure functions with no OS
dependency, fully unit-tested without touching a kernel. Only the
monotonic-to-wall-clock conversion and the `/proc` lookup — the two
genuinely OS-dependent steps — live in Linux-only or filesystem-touching
code.

**Wired into `pulse-agent`, on a best-effort basis.** Unlike Day 03's
foundation program, this one is started from `internal/agent.App.Run`:
on a platform or kernel that doesn't support it, or without sufficient
privilege, `Run` logs why and continues running without process
discovery rather than failing to start — telemetry capture is never
allowed to be a reason `pulse-agent` itself won't run. `App` depends on
an unexported `processLoader` interface (`Load`/`Attach`/`Read`) rather
than `*process.Loader` concretely, so `internal/agent`'s own tests can
substitute a fake and verify the wiring — event logging, and warning
only on an unexpected read failure rather than an expected shutdown-
triggered one — without touching a real kernel.

## What's deliberately not here yet

- **No container/pod/namespace/service identity.** That's Day 08's
  stated deliverable, which explicitly designs this identity model; see
  `docs/design/event-model.md`'s deferral table.
- **No correlation between a `process.exit` and its matching
  `process.start`**, and no process tree reconstruction from PPID chains
  beyond reporting the immediate parent. Both are consumers of this raw
  data, not this data itself.
- **No export or persistence.** Events are logged by `pulse-agent`, not
  sent anywhere — Kafka (Day 14) and storage (Day 15) don't exist yet.

## Tradeoffs

- **One shared ring buffer for both event kinds**, discriminated by a
  trailing `event_type` byte, rather than two maps. Simpler lifecycle
  (one reader, one buffer to size) at the cost of every consumer needing
  to branch on the discriminator — an acceptable trade at two event
  kinds; revisit if a third, differently-shaped kind arrives.
- **`internal/process`'s `Loader` duplicates `internal/ebpf`'s `Loader`
  shape** (`Load`/`Attach`/`Read`/`Close`, same cleanup-order-on-Close
  discipline) rather than sharing an abstraction. `internal/ebpf`'s own
  docs called this out as a "premature until there's a second program"
  question — there now is a second program, and the honest answer is
  still that ~100 lines of duplication across two small, independently
  understandable loaders is cheaper than a generic one would be to
  design well. Revisit if a third program makes the pattern's shape more
  obvious.
- **Command name over full executable path, captured in-kernel.**
  `comm` is truncated to 16 bytes by the kernel itself (`TASK_COMM_LEN`)
  and can be changed by the process (e.g. via `prctl(PR_SET_NAME)`) —
  it's an identifying label, not a trustworthy path. Using it in-kernel
  for both event kinds, with the real path resolved separately and only
  where reliable, was judged better than compiling in a `bpf_d_path`
  call whose one extra caller (the exit path) can't use its result
  anyway.

## Failure modes

- **Non-Linux OS, or Linux without ring buffer/tracepoint support:**
  `Loader.Load` fails with `ebpf.CheckSupport`'s error before touching
  the kernel; `internal/agent` logs "process discovery unavailable" and
  keeps running.
- **Insufficient privilege to load:** same path, different underlying
  error (typically `EPERM`), surfaced as-is rather than masked.
- **Ring buffer full:** `process.c` drops the event; no backpressure to
  the traced process, same as `foundation.c`.
- **`ResolveExecutable` after the process has already exited (or been
  reaped and its PID reused):** returns an error (not called at all for
  exit events; possible but rare for a very short-lived start event).
  `internal/agent` treats this as "unknown," not a warning — an empty
  `Executable` field is a normal, expected outcome, not a bug.
- **Unexpected `Read` failure vs. a shutdown-triggered one:** both look
  identical to `Loader.Read` (an error), but `internal/agent`'s watcher
  checks `ctx.Err()` to tell them apart — see `TestWatchProcessEvents_*`
  in `internal/agent/process_test.go` for both paths asserted directly.

## Performance

Not benchmarked. `sched_process_exec`/`sched_process_exit` fire once per
exec/exit system-wide, at a rate driven entirely by host workload, not
by this program — no realistic synthetic benchmark exists yet to run
against. Revisit once Day 07's pipeline gives a real end-to-end
workload, or Day 21's performance engineering pass.

## Security

- Captured fields (PID, PPID, a 16-byte command name, timestamp) are
  process metadata, not memory or credential contents — consistent with
  "prefer metadata over sensitive application contents."
- `/proc/<pid>/exe` is a symlink to the executable path; reading it
  requires no elevated privilege beyond what loading the eBPF program
  itself already required, and reveals nothing `ps`/`/proc` wouldn't to
  any local observer.
- No new privilege requirement beyond `internal/ebpf`'s (root, or
  `CAP_BPF`+`CAP_PERFMON`) — see `docs/design/ebpf-foundation.md`'s
  Security section, which this inherits unchanged.

## Limitations

- PPID is the immediate parent only, read once at event time — a parent
  that itself exits and is reparented (e.g. to `init`) afterward is not
  tracked; this reports parentage as of the exec/exit moment, not a
  live relationship.
- `Executable` is best-effort and frequently empty for very short-lived
  processes or for any exit event — see Failure modes. Don't treat an
  empty value as an error condition upstream.
- No de-duplication or ordering guarantees beyond what a single BPF ring
  buffer already provides (FIFO per-producer, no cross-CPU reordering
  since Linux 5.8's ring buffer replaced the older per-CPU perf buffer
  design) — see `docs/design/ebpf-foundation.md` for why ring buffers
  were chosen over perf buffers in the first place.
