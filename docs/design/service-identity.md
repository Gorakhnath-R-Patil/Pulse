# Service Identity

Package: [`internal/discovery`](../../internal/discovery). Consumed by
[`internal/agent`](../../internal/agent)'s `containerEnrichingSource`
(`enrich.go`), which wraps every capability's `EventSource`.

## Problem

Day 04 answered "which process"; this day answers "which container, and
on Kubernetes, which pod" — the identity layer that lets telemetry from
a bare-metal process and telemetry from a containerized one be told
apart, and that a future service dependency graph (Day 16) will need to
group processes into services at all.

## Design

```
/proc/<pid>/cgroup                 internal/discovery              internal/agent
(already on disk, no runtime  ─→   ResolveContainer(pid)      ─→   containerEnrichingSource
 or Kubernetes API call)           = parseCgroup(data)              wraps every capability's
                                    (pure, tested against              EventSource uniformly
                                     real cgroup path fixtures)
```

**cgroup membership, not the container runtime or Kubernetes API.**
Every process on Linux already belongs to a cgroup, and the path the
container runtime and (on Kubernetes) kubelet assign it already encodes
the container ID and, for Kubernetes, the pod UID — see
`container.go`'s doc comments for the exact Docker/containerd/Kubernetes
path conventions this recognizes, on both the cgroupfs and systemd
cgroup drivers. Reading `/proc/<pid>/cgroup` needs no daemon socket, no
runtime client library, no Kubernetes client, and no extra privilege
beyond what reading `/proc` already requires — the same "no extra moving
parts" bar `internal/process`'s `ResolveExecutable` already set for
`/proc/<pid>/exe`.

**A decorator over `EventSource`, not a `pipeline.EventProcessor`.**
Container identity applies uniformly to every capability's events (they
all carry a `Process`), so it belongs in one shared place, not
duplicated into `process.go`/`network.go`/`socket.go`'s three source
adapters. It can't be a `pipeline.EventProcessor`, though:
`EventProcessor.Process` receives an event by value with no way to hand
an enriched copy to the next processor in the chain — processors are
terminal consumers, not a transform step (see
`docs/design/event-pipeline.md`'s Design section). `containerEnrichingSource`
wraps a `pipeline.EventSource` instead, adding `Process.Container`
after the inner source normalizes but before the pipeline (or any
processor) ever sees the event — the same position `internal/process`'s
own executable-resolution enrichment already occupies in `processSource.Read`,
just factored out so it applies to all three capabilities instead of
being process-discovery-specific.

**`Container`, not `Namespace`/`Pod`/`Service`.** `pkg/model.Container`
holds exactly what cgroup membership can prove: a container runtime ID,
and (on Kubernetes) a pod *UID*. It deliberately does not have
`Namespace`, a pod *name*, or a service name — kubelet's cgroup path
convention embeds the pod's UID, not its human-readable identity, and
turning a UID into a name/namespace requires asking the Kubernetes API,
which is Day 19's job. Adding empty placeholder fields for those now
would be exactly the "TODO-driven incomplete architecture" the project's
engineering standard warns against; `Container` is shaped so Day 19 can
extend it with those fields when it actually populates them, not so it
merely looks complete today.

## What's deliberately not here yet

- **`Namespace`, pod name, `Service`** — see Design, above, and
  `docs/design/event-model.md`'s deferral table.
- **Container runtime metadata beyond an ID** (image name, labels,
  restart count, ...) — would require calling the container runtime's
  API (Docker/CRI socket), a real additional moving part this day's
  "no extra dependencies" design deliberately avoids. Revisit if a
  concrete need for it appears.
- **Caching or invalidation.** `ResolveContainer` re-reads
  `/proc/<pid>/cgroup` on every call; a process's container membership
  doesn't change over its lifetime, so this is simple correctness at
  the cost of a redundant file read per event. Not measured as a
  problem yet — see Performance.

## Tradeoffs

- **Regex-based cgroup path matching over a structured per-controller
  parser.** A 64-hex-character container ID and a `pod<uuid>` substring
  are recognizable anywhere in the file's text, regardless of which
  cgroup v1 controller line or cgroup v2's single unified line they
  appear on — matching across the whole file sidesteps needing to know
  which specific line format a given kernel/driver combination
  produces. The cost is Limitations, below: this recognizes common,
  well-documented conventions, not an exhaustive model of every runtime
  and cgroup driver combination in existence.
- **Silent nil on no match, not an error.** Most processes on most
  hosts are not containerized — `ResolveContainer` returning `nil,
  nil` for a bare host process is the expected common case, not a
  degraded outcome. An error is reserved for genuinely being unable to
  read `/proc/<pid>/cgroup` at all (process gone, `/proc` unavailable).

## Failure modes

- **Non-Linux platform:** `/proc/<pid>/cgroup` doesn't exist;
  `ResolveContainer` returns an error, and `containerEnrichingSource`
  treats that identically to "not containerized" — `Process.Container`
  stays unset, no error propagates to the pipeline.
- **Process already exited by the time enrichment runs:**
  same outcome — `os.ReadFile` fails, treated as "unknown," not
  surfaced as a pipeline read failure. This mirrors
  `internal/process.ResolveExecutable`'s identical race for the same
  reason (see `docs/design/process-discovery.md`'s Failure modes).
- **Cgroup path from a runtime/driver this package doesn't recognize:**
  `parseCgroup` finds no 64-hex ID, returns `nil` — indistinguishable
  from "not containerized" from the caller's side. See Limitations.

## Performance

Not benchmarked. One `/proc/<pid>/cgroup` read and two regex passes per
event is unlikely to be the bottleneck in a pipeline whose per-event
processing is currently dominated by JSON-encoding a log line, but this
is a claim to verify, not assume — see Day 21's performance engineering
pass.

## Security

Cgroup paths and container/pod IDs are infrastructure metadata, not
application data — consistent with "prefer metadata over sensitive
application contents." Reading `/proc/<pid>/cgroup` requires the same
privilege level Pulse already needs to observe `pid` at all (it's
running as root or with `CAP_BPF`/`CAP_PERFMON` already); this adds no
new privilege requirement.

## Limitations

- Recognizes Docker, containerd, and CRI-O's common 64-hex-ID cgroup
  path convention, and Kubernetes' `pod<uuid>` convention on both
  cgroupfs and systemd drivers. Non-standard cgroup layouts, unusual
  custom container runtimes, or future convention changes may not be
  recognized — see Tradeoffs.
- Pod UID only, never a pod name or namespace — see Design.
- No verification that a matched container ID is actually a *running*
  container's ID (vs., in principle, a coincidental 64-hex-character
  substring elsewhere in the cgroup path) — considered acceptably
  unlikely given the path conventions this targets, not formally ruled
  out.
