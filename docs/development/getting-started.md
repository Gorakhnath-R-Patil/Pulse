# Getting Started

This guide gets you from a fresh clone to running binaries.

## Prerequisites

- **Go 1.23+** (developed and tested against 1.26).
- **Git.**
- A POSIX shell (Git Bash, WSL, or Linux/macOS) to use the `Makefile` —
  it uses `$(shell ...)` and `rm -rf`, which native `cmd.exe`/PowerShell
  don't support. If you're on native PowerShell, use the equivalent
  `go build`/`go test` commands shown below instead.
- **`cmd/pulse-agent`, `cmd/pulse-collector`, and `cmd/pulse-cli` build
  and run on any platform Go supports** — none of them require Linux or
  eBPF yet (see `internal/ebpf`'s cross-platform stubs).
- **Linux 5.8+ and `clang`** are required only to build/test
  [`internal/ebpf`](../../internal/ebpf)'s real (non-stub)
  implementation and to regenerate its eBPF bindings — see "eBPF
  development" below. `go build ./...`/`go test ./...` on any other OS
  simply excludes those files via build tags; you don't need Linux to
  work on anything else in this repository.

## Clone and build

```bash
git clone https://github.com/Gorakhnath-R-Patil/Pulse.git
cd Pulse
make build        # builds bin/pulse-agent, bin/pulse-collector, bin/pulse-cli
```

Without `make` (any platform):

```bash
go build -o bin/pulse-agent     ./cmd/pulse-agent
go build -o bin/pulse-collector ./cmd/pulse-collector
go build -o bin/pulse-cli       ./cmd/pulse-cli
```

## Run

```bash
./bin/pulse-agent --config examples/config/agent.example.yaml
./bin/pulse-collector --config examples/config/collector.example.yaml
```

Both currently just start up, log their identity, and block until you
send `SIGINT`/`SIGTERM` (Ctrl+C), then shut down. There is no observation
or telemetry pipeline yet — see the roadmap in the root `README.md`.

```bash
./bin/pulse-cli version
./bin/pulse-cli config validate -target agent -file examples/config/agent.example.yaml
```

## Test

```bash
make test        # go test ./...
make test-race   # with the race detector
make cover       # with a per-package coverage report
```

`-race` requires cgo, which requires a C compiler. On Windows without one
installed (no MinGW/MSVC), run plain `go test ./...` locally — CI runs the
race-enabled variant on Linux, where a C toolchain is always present.

## eBPF development

`internal/ebpf`'s Linux implementation depends on generated bindings for
[`bpf/programs/foundation.c`](../../bpf/programs/foundation.c) that are
**not committed** to the repository (see ADR-007 in
`docs/design/decisions.md`) — you must generate them yourself, on Linux,
with `clang` installed, before `go build`/`go test` will succeed for that
package:

```bash
# Debian/Ubuntu
sudo apt-get install -y clang

go generate ./...
```

This regenerates the bpf2go bindings for `internal/ebpf` (from
`bpf/programs/foundation.c`), `internal/process` (from
`bpf/programs/process.c`), and `internal/network` (from
`bpf/programs/tcp_connect.c`). Re-run it whenever you change a file
under `bpf/programs/` or `bpf/headers/`.

Loading the resulting programs into the kernel (as opposed to just
compiling them) additionally requires root or `CAP_BPF`+`CAP_PERFMON`:

```bash
go test ./internal/ebpf/... ./internal/process/... ./internal/network/...              # unprivileged: everything except the real load/attach/receive/detach cycle
sudo -E env "PATH=$PATH" go test ./internal/ebpf/... ./internal/process/... ./internal/network/... -run TestLoader   # exercises it for real
```

Without root, the privileged tests skip themselves with an explanatory
message rather than failing — see each package's `loader_linux_test.go`.

## Code quality gate

Before committing, run what CI runs:

```bash
make ci          # fmt-check + vet + test-race
```

Or individually: `make fmt` (reformat), `make vet` (static analysis),
`make fmt-check` (verify formatting without modifying files).

## Repository layout

```
cmd/                 Binary entry points — thin: flag parsing, wiring, os.Exit.
internal/agent/      pulse-agent application — starts each capability below, best-effort.
internal/collector/  pulse-collector application (lifecycle skeleton today).
internal/cli/        pulse-cli command tree, unit-testable via Execute().
internal/config/     Configuration structs, YAML loading, env overrides, validation.
internal/logging/    Structured logger construction (log/slog).
internal/version/    Build-time version/commit/date, injected via -ldflags.
internal/ebpf/       eBPF load/attach/receive/detach lifecycle. Linux-only; stubs elsewhere.
internal/process/    Process discovery: kernel capture, decode, normalize to pkg/model.Event.
internal/network/    Network connection telemetry: same shape as internal/process, for TCP connect.
pkg/model/           Canonical telemetry Event and its sub-structures — the shared data contract.
proto/               Wire-format contracts (.proto), checked in ahead of any code generation.
bpf/programs/        Hand-written eBPF C source.
bpf/headers/         Vendored minimal libbpf headers (see bpf/headers/README.md).
examples/config/     Example YAML configs for each binary.
docs/design/         Architecture decisions (decisions.md) and design docs.
docs/development/    This document and related contributor docs.
```

Every directory above exists because something in it is implemented
today. Directories described in the project's long-term target layout
(`pkg/api`, `deployments/`, `helm/`, …) are created only on the day their
content is actually implemented — see `docs/design/decisions.md` and the
project roadmap for why. Note `bpf/generated/` from that target layout
does not exist as a real directory: eBPF codegen output is generated
directly into `internal/ebpf/` and not committed — see ADR-007.

## Coding conventions

See `CONTRIBUTING.md` for the full list. In short: `gofmt` and `go vet`
must be clean, errors are wrapped with `%w` and checked with `errors.Is`
against package-level sentinel errors, and no component reads from
package-level global mutable state — dependencies (config, logger) are
passed explicitly to constructors.
