# Architecture Decision Log

This log records decisions that materially shape Pulse's architecture —
not every implementation detail. Trivial choices with an obvious default
are not recorded here.

Format:

```
Decision:
Context:
Options:
Choice:
Reason:
Tradeoffs:
```

---

## ADR-001: Go as the sole implementation language

**Decision:** Implement all userspace components (agent, collector, CLI,
future control/processing plane) in Go. eBPF programs are written in C, as
required by the eBPF toolchain itself.

**Context:** A kernel-native observability platform needs a userspace
language with strong concurrency primitives, a mature ecosystem for
networking/gRPC/Kafka, and first-class eBPF tooling support (cilium-ebpf).
The realistic alternatives are Go and Rust.

**Options:**
- **Go** — mature eBPF libraries (cilium/ebpf), simple concurrency model,
  fast compilation, large observability/infra ecosystem (Prometheus,
  Kubernetes, Cilium are all Go).
- **Rust** — stronger compile-time safety guarantees, no GC pauses, but a
  steeper learning curve and a less mature eBPF userspace ecosystem than
  Go's.

**Choice:** Go.

**Reason:** Go is the standard language for cloud-native infrastructure
tooling (Kubernetes, Prometheus, Cilium's userspace components, Docker),
which means the engineering patterns this project studies are directly
applicable. Its lower ceremony keeps a 25-day solo build tractable without
giving up production-grade concurrency support.

**Tradeoffs:** Go's garbage collector introduces latency variance that
Rust would avoid; this is acceptable because the latency-critical path
(eBPF program execution) runs in the kernel, not in Go, and userspace
processing has generous latency budgets relative to kernel-level capture.

---

## ADR-002: Structured logging via the standard library (`log/slog`)

**Decision:** Use `log/slog` from the Go standard library for all
component logging, with no third-party logging dependency.

**Context:** Pulse needs structured, leveled logging across three
binaries today and more components later. Common third-party choices are
`zap` (Uber) and `zerolog`.

**Options:**
- `log/slog` — stdlib since Go 1.21, structured, zero extra dependency,
  handler-based so JSON/text output is pluggable.
- `zap` — faster in allocation-sensitive hot paths, but adds a dependency
  and its own configuration surface.
- `zerolog` — similarly capable, similarly an extra dependency.

**Choice:** `log/slog`.

**Reason:** No component in the current architecture logs at a rate where
`zap`'s allocation advantages matter (that changes for the eBPF ring
buffer read path itself, which won't go through the logger at all — see
`docs/security/threat-model.md`, to be written when eBPF capture lands).
Avoiding a dependency here is a direct application of the "avoid
excessive dependencies" principle, and `slog`'s handler interface leaves
room to swap in a faster backend later without changing call sites if
that ever proves necessary.

**Tradeoffs:** Slightly more allocation per log call than `zap` in
benchmarks; irrelevant at today's log volumes and revisited if
`benchmarks/` ever shows logging as a bottleneck.

---

## ADR-003: Custom minimal YAML config loader, not Viper

**Decision:** Load configuration from an optional YAML file
(`gopkg.in/yaml.v3`) plus a small, explicit set of environment variable
overrides, implemented as ~60 lines of package code rather than via
`spf13/viper`.

**Context:** Viper is the de facto standard for Go CLI configuration
(env vars, flags, multiple file formats, live reload, remote config
stores). Pulse only needs "defaults → optional YAML file → a handful of
env overrides → validate" today.

**Options:**
- **Viper** — batteries-included, but relies on package-level global
  state (`viper.GetString(...)` reads from a shared singleton) and pulls
  in a large dependency tree (afero, several format decoders, remote
  config backends) that Pulse doesn't use.
- **Custom loader** — a `LoadAgentConfig(path string) (AgentConfig,
  error)` function per binary, using `yaml.v3` directly.

**Choice:** Custom loader.

**Reason:** Section 3 of the project's engineering standard explicitly
calls out "global mutable state" and "excessive dependencies" as things
to avoid. Viper's global singleton is exactly the pattern being avoided
elsewhere in this codebase (every other component takes its dependencies
by constructor argument). A ~60-line loader with strict YAML decoding
(`KnownFields(true)`, so a typo'd field fails loudly) covers 100% of
today's requirements with one dependency instead of a dozen transitive
ones.

**Tradeoffs:** If configuration needs grow substantially (multiple file
formats, live reload, remote config stores), the custom loader will need
deliberate extension rather than getting it for free. That is an
acceptable and reversible tradeoff — this decision should be revisited
if that need materializes, not preemptively solved for now.

---

## ADR-004: Standard library `flag` with manual subcommands, not Cobra

**Decision:** Implement `pulse-cli`'s subcommands (`version`, `config
validate`) using the standard library's `flag` package with a small hand-
written dispatcher (`internal/cli.Execute`), not `spf13/cobra`.

**Context:** Cobra is the standard framework for multi-command Go CLIs
(used by `kubectl`, `docker`, `hugo`) and Pulse will likely grow more
`pulse-cli` subcommands as topology/trace/metrics query commands are
added in later days.

**Options:**
- **Cobra** — nested commands, auto-generated help/completion, flag
  inheritance; the standard choice for CLIs of Pulse's eventual size.
- **stdlib `flag` + manual dispatch** — a `map[string]func(...)`-style
  switch over the first argument, each command owning its own
  `flag.FlagSet`.

**Choice:** stdlib `flag` with manual dispatch, for now.

**Reason:** With exactly two commands today, Cobra's nested-command
machinery and dependency weight buy nothing yet. The manual dispatcher in
`internal/cli/root.go` is under 50 lines and fully unit-testable via
`Execute(args, stdout, stderr) int` without spawning a subprocess.

**Tradeoffs:** This will not scale gracefully past a handful of
commands — nested subcommands, shared persistent flags, and shell
completion would all need to be hand-rolled. Revisit and adopt Cobra if
`pulse-cli`'s command tree grows past roughly 5–6 commands or needs
completion/nesting; the switch is a contained change since all command
logic already lives behind the `Execute` boundary, not in `main()`.

---

## ADR-005: Apache License 2.0

**Decision:** License the project under Apache License 2.0.

**Context:** GitHub's repository creation flow was used to initialize the
remote with this license already selected.

**Options:** MIT (simpler, no explicit patent grant), Apache 2.0 (explicit
patent grant, contributor protections), GPL-family (copyleft — not
appropriate for infrastructure code meant to be embedded/deployed
freely).

**Choice:** Apache License 2.0.

**Reason:** Apache 2.0 is the standard for infrastructure projects Pulse
takes architectural inspiration from (Cilium, Prometheus, Kubernetes,
Envoy, OpenTelemetry all use it), and its explicit patent grant matters
for software with kernel-level system access where patent exposure is a
more realistic concern than for typical application code.

**Tradeoffs:** None material for this project's goals.

---

## ADR-006: Protobuf contract checked in without code generation

**Decision:** Define `proto/pulse/v1/event.proto` as the wire-format
contract for `pkg/model.Event`, but do not wire up `protoc`/`buf` code
generation, a `proto/generated/` output directory, or a Makefile target
for it yet.

**Context:** The project's target layout includes a `proto/` directory,
and section 6 of the engineering standard calls for "protobuf contracts
where appropriate" as part of the event model. Nothing in the codebase
consumes generated protobuf Go bindings yet: JSON (via `encoding/json`)
is what `pkg/model` actually serializes with today, and gRPC/OTLP export
isn't implemented until Day 13, Kafka transport until Day 14.

**Options:**
- Check in the `.proto` file only, as a contract/reference, with no build
  tooling.
- Install `protoc` (or `buf`) now and generate Go bindings immediately,
  even though nothing calls them.

**Choice:** Contract only, no codegen yet.

**Reason:** Generating code with no caller is exactly the "TODO-driven
incomplete architecture" and "fake scalability" the project's standard
warns against — it would add a toolchain dependency (protoc/buf must be
installed to regenerate) and a `generated/` package that `go vet`/tests
would need to account for, with zero present-day payoff. The `.proto`
file itself is real value today: it's the reviewable schema contract that
Day 13/14 will generate against, and it forces `pkg/model`'s shape to
stay expressible in protobuf (no Go-only tricks that wouldn't survive a
wire format) from the start.

**Tradeoffs:** The `.proto` file and `pkg/model`'s Go structs must be
kept in sync by hand until codegen exists; a drift between them wouldn't
be caught by any test today. This is acceptable for two fields' worth of
schema and should be revisited (add a CI check, or just generate code) if
the schema grows before Day 13 arrives.
