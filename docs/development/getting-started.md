# Getting Started

This guide gets you from a fresh clone to running binaries.

## Prerequisites

- **Go 1.23+** (developed and tested against 1.26).
- **Git.**
- A POSIX shell (Git Bash, WSL, or Linux/macOS) to use the `Makefile` —
  it uses `$(shell ...)` and `rm -rf`, which native `cmd.exe`/PowerShell
  don't support. If you're on native PowerShell, use the equivalent
  `go build`/`go test` commands shown below instead.
- **Linux with eBPF support** is not required yet. It becomes required
  starting Day 03, when `pulse-agent` begins loading eBPF programs. Today,
  all three binaries build and run on any platform Go supports.

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
internal/agent/      pulse-agent application (lifecycle skeleton today).
internal/collector/  pulse-collector application (lifecycle skeleton today).
internal/cli/        pulse-cli command tree, unit-testable via Execute().
internal/config/     Configuration structs, YAML loading, env overrides, validation.
internal/logging/    Structured logger construction (log/slog).
internal/version/    Build-time version/commit/date, injected via -ldflags.
examples/config/     Example YAML configs for each binary.
docs/design/         Architecture decisions (decisions.md) and design docs.
docs/development/    This document and related contributor docs.
```

Every directory above exists because something in it is implemented
today. Directories described in the project's long-term target layout
(`bpf/`, `proto/`, `pkg/api`, `deployments/`, `helm/`, …) are created only
on the day their content is actually implemented — see
`docs/design/decisions.md` and the project roadmap for why.

## Coding conventions

See `CONTRIBUTING.md` for the full list. In short: `gofmt` and `go vet`
must be clean, errors are wrapped with `%w` and checked with `errors.Is`
against package-level sentinel errors, and no component reads from
package-level global mutable state — dependencies (config, logger) are
passed explicitly to constructors.
