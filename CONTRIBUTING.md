# Contributing to Pulse

Pulse is an early-stage, actively-developed open-source project. This
document covers the practical mechanics of contributing; see
`docs/design/decisions.md` for the reasoning behind existing architectural
choices before proposing to change them.

## Prerequisites

See `docs/development/getting-started.md` for toolchain requirements and
build/test instructions.

## Development workflow

1. Fork/branch, make your change.
2. Add or update tests — see "Testing" below for expectations.
3. Run the local quality gate before opening a PR:
   ```bash
   make ci   # gofmt check + go vet + go test -race
   ```
4. Update relevant documentation (`docs/`, package doc comments,
   `README.md`) in the same change as the code it describes.
5. Open a PR with a description of *why*, not just *what* — link to any
   relevant issue or design discussion.

## Coding conventions

- **Format with `gofmt`.** CI enforces this; `make fmt` applies it.
- **`go vet` must be clean.**
- **Errors are wrapped, not swallowed.** Use `fmt.Errorf("...: %w", err)`
  to add context while preserving the underlying error for
  `errors.Is`/`errors.As`. Packages that need callers to distinguish
  error conditions define package-level sentinel errors (see
  `internal/config/errors.go` for the pattern) rather than relying on
  string matching.
- **No package-level mutable state.** Components take their dependencies
  (config, logger, clients) as constructor arguments, not from globals or
  `init()`-populated singletons. This is why `internal/logging.New`
  takes an `io.Writer` and returns a `*slog.Logger` instead of the
  package exposing a shared default logger.
- **Structured logging only**, via `log/slog`. Log with key-value pairs
  (`logger.Info("event", "key", value)`), not formatted strings, so
  output stays machine-parseable in JSON mode.
- **Keep functions small and single-purpose.** If a function is doing
  "and" — parsing and validating and logging — consider splitting it.
- **Justify new dependencies.** Before adding one, check whether the
  standard library already covers the need (see ADR-002 and ADR-003 in
  `docs/design/decisions.md` for the reasoning already applied to logging
  and configuration). If a dependency is genuinely warranted, say why in
  the PR description.
- **Justify new directories/packages.** Every package should exist
  because it holds real, implemented functionality — not because the
  target architecture in the root README eventually needs one. See
  `docs/development/getting-started.md`'s repository layout section.

## Testing

- New behavior needs a test. Table-driven tests are preferred for
  functions with several input/output cases (see
  `internal/config/logging_test.go` for the pattern).
- Tests that exercise CLI/binary argument handling should go through the
  package's testable entry point (`internal/cli.Execute`, or a `run()`
  function in the binary's own `main` package) rather than shelling out to
  a compiled binary — see `cmd/pulse-agent/main_test.go`.
- Tests must not hang. Anything waiting on a channel, context, or signal
  should have a `time.After`/`context.WithTimeout` bound (see
  `internal/agent/agent_test.go`).
- Don't fake benchmark numbers or claim performance/compatibility
  characteristics that haven't actually been measured — see "No Fake
  Engineering" in the project's engineering standard.

## Commit messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(day-01): initialize pulse architecture
fix(day-05): correct TCP connect event byte order
docs(day-11): document trace model design
```

Describe the actual change. Avoid non-descriptive messages like `update`
or `fixes`.

## Security

Pulse operates at kernel level once eBPF capture lands (Day 03+). If you
find a security issue, please do not open a public issue — see
`docs/security/threat-model.md` (added when eBPF capture is introduced)
for the disclosure process once it exists; until then, contact the
maintainer directly via the contact information on their GitHub profile.

## Code of conduct

Be respectful, be constructive, assume good faith. Technical disagreements
should be resolved by evidence (benchmarks, tests, prior art) rather than
assertion.
