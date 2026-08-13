# Pulse

**Pulse is a kernel-native distributed observability platform.** It uses
eBPF on Linux to observe microservices and infrastructure — network
connections, HTTP/DNS traffic, process activity — with little to no
application instrumentation, and turns that into distributed traces,
service dependency graphs, and metrics.

> **Status: early-stage, active development (Day 1 of a 25-day build).**
> This is not yet functional software — see [What works today](#what-works-today).
> Nothing here should be treated as production-ready until the roadmap
> below says so explicitly.

## Why

Most distributed tracing requires instrumenting every service — SDKs,
middleware, code changes — before you get a single trace. That's a real
cost, and it means unowned or legacy services stay invisible. eBPF lets a
Linux host observe TCP connections, HTTP request/response boundaries, and
DNS queries directly from the kernel, without touching application code.
Pulse's goal is to turn that kernel-level visibility into the same
artifacts an instrumented system would produce — traces, topology,
latency/error metrics — for services that were never instrumented at all.

Pulse is a systems project first: it exists to be a technically credible,
inspectable implementation of that idea, built the way an infrastructure
team would actually build it — measured, tested, documented, and honest
about what it can't yet do.

## How it fits together

```
Applications
     │
     ▼
Linux Kernel
     │
     ▼
   eBPF
     │
     ▼
Pulse Agent  ──────▶  Pulse Collector  ──────▶  Kafka ──────▶  Storage / Metrics
(per host)            (aggregation)                                  │
                                                                      ▼
                                                                  Grafana
```

`pulse-agent` runs on each Linux host and will observe kernel-level
activity via eBPF. `pulse-collector` receives and processes what agents
report. `pulse-cli` is the operator-facing tool for inspecting and
validating the system. See
[docs/design/decisions.md](docs/design/decisions.md) for the reasoning
behind specific technical choices as they're made, day by day.

## What works today

- Three binaries (`pulse-agent`, `pulse-collector`, `pulse-cli`) that
  build, accept configuration, log their startup/shutdown lifecycle, and
  exit cleanly on `SIGINT`/`SIGTERM`.
- A configuration model (`internal/config`): YAML file + environment
  variable overrides + validation, with defaults sane enough to run with
  zero configuration.
- Structured logging (`internal/logging`, built on `log/slog`) shared by
  every component.
- `pulse-cli version` and `pulse-cli config validate` for inspecting a
  build and checking a config file without starting anything.
- Pulse's canonical telemetry record, [`pkg/model.Event`](pkg/model) —
  identifiers, timestamps, process/network metadata, validation, and JSON
  serialization — plus its wire-format contract,
  [`proto/pulse/v1/event.proto`](proto/pulse/v1/event.proto). See
  [docs/design/event-model.md](docs/design/event-model.md).
- A CI foundation that runs formatting, `go vet`, and the test suite on
  every push.
- The eBPF foundation ([`internal/ebpf`](internal/ebpf)): load a real BPF
  program, attach it to a real kernel tracepoint, receive real events
  through a BPF ring buffer, and detach cleanly — with kernel
  compatibility checks and graceful failure on unsupported platforms.
  The program itself ([`bpf/programs/foundation.c`](bpf/programs/foundation.c))
  extracts no telemetry — see
  [docs/design/ebpf-foundation.md](docs/design/ebpf-foundation.md) for
  what it proves and what it deliberately doesn't do yet.
- Process discovery ([`internal/process`](internal/process)): real
  process start/exit events, captured in-kernel via
  [`bpf/programs/process.c`](bpf/programs/process.c) (PID, parent PID,
  command name, minimal CO-RE — no `vmlinux.h` needed), normalized into
  `pkg/model.Event`, and logged by `pulse-agent` on any Linux host it
  runs on with sufficient privilege — gracefully unavailable, not fatal,
  everywhere else. See
  [docs/design/process-discovery.md](docs/design/process-discovery.md).
- Network connection telemetry ([`internal/network`](internal/network)):
  real outbound IPv4 TCP connect attempts, captured in-kernel via
  [`bpf/programs/tcp_connect.c`](bpf/programs/tcp_connect.c) — source and
  destination address/port, the connecting process, and whether the
  connect succeeded — normalized into `pkg/model.Event` and logged by
  `pulse-agent` alongside process events. See
  [docs/design/network-connect.md](docs/design/network-connect.md).

This is real, kernel-observed telemetry, not a placeholder — run
`pulse-agent` on Linux and it logs every process that starts or exits,
and every outbound TCP connection attempt, on the host. What's not here
yet: byte-level socket data, container/service identity, distributed
tracing, export, and storage — all later
in the roadmap.

## Getting started

```bash
git clone https://github.com/Gorakhnath-R-Patil/Pulse.git
cd Pulse
make build
./bin/pulse-cli version
./bin/pulse-agent --config examples/config/agent.example.yaml
```

Full instructions: [docs/development/getting-started.md](docs/development/getting-started.md).
Configuration reference: [docs/development/configuration.md](docs/development/configuration.md).

## Roadmap

Pulse is being built over 25 scoped development stages, each landing as
one reviewable commit. Rough phases, not a fixed schedule:

1. **Foundation** — project structure, config, logging, CLI *(Day 1 — here)*
2. **Telemetry model** — the internal event schema everything else builds on
3. **eBPF foundation & kernel telemetry** — process, network connection, socket-level capture
4. **Userspace pipeline** — decode → normalize → enrich → correlate → aggregate → export
5. **Protocol visibility** — HTTP and DNS telemetry from captured traffic
6. **Distributed tracing** — trace/span model, correlation across services
7. **Transport & storage at scale** — OTLP export, Kafka, ClickHouse
8. **Topology & metrics** — service dependency graphs, latency/error aggregation, Grafana dashboards
9. **Production hardening** — Kubernetes/Helm deployment, failure handling, performance engineering, security review
10. **Demonstration & release** — chaos scenarios, optional local AI incident analysis, `v0.1.0`

Each stage's actual scope is defined and recorded as it's implemented —
this list will stay a summary, not a promise of exact dates or features.

## Design principles

- **Correctness and honesty over feature count.** If something is
  experimental, unsupported, or unmeasured, the docs say so — no fabricated
  benchmarks, no claimed compatibility that wasn't tested.
- **Boring, proven technology.** Go, the standard library where it
  suffices, well-established infrastructure (Kafka, ClickHouse, Prometheus,
  Grafana, Kubernetes) over novel or fashionable alternatives.
- **No global mutable state.** Every component receives its dependencies
  explicitly and can be tested without touching real environment/filesystem
  state.
- **Metadata over payloads.** Pulse observes *that* traffic happened and its
  shape (addresses, ports, timing, status) — not application payload
  contents — unless a specific, documented feature says otherwise.

## License

[Apache License 2.0](LICENSE).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).
