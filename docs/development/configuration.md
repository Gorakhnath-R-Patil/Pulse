# Configuration

Both `pulse-agent` and `pulse-collector` load configuration the same way,
implemented in `internal/config`.

## Precedence

Configuration is resolved in this order, each step able to override the
previous:

1. **Built-in defaults** (`config.DefaultAgentConfig()` /
   `config.DefaultCollectorConfig()`) — safe enough to run with no
   configuration at all.
2. **YAML file**, if `--config <path>` is given. If the flag is given but
   the file doesn't exist, this is a startup error (`ErrNotFound`) rather
   than a silent fallback to defaults — an operator who names a file
   expects it to be read.
3. **Environment variables** — currently `PULSE_LOG_LEVEL` and
   `PULSE_LOG_FORMAT`, which override the `logging.level` /
   `logging.format` fields regardless of source. This matches the common
   container pattern of overriding a mounted config file at deploy time
   without editing it.
4. **Validation** — the fully-resolved config is validated before the
   loader returns it. Any failure here is also a startup error
   (`ErrInvalidValue`), never a silent correction.

YAML decoding is strict (`KnownFields(true)`): an unrecognized field
(e.g. a typo) is a startup error (`ErrInvalidSyntax`), not a silently
ignored key.

## `pulse-agent` fields

| Field             | Type   | Default            | Description                                                        |
|-------------------|--------|---------------------|----------------------------------------------------------------------|
| `node_name`       | string | OS hostname          | Identifies this host in emitted telemetry (used starting Day 04+). |
| `logging.level`   | string | `info`               | One of `debug`, `info`, `warn`, `error`.                            |
| `logging.format`  | string | `json`               | One of `json`, `text`.                                              |

## `pulse-collector` fields

| Field             | Type   | Default | Description                          |
|-------------------|--------|---------|---------------------------------------|
| `logging.level`   | string | `info`  | One of `debug`, `info`, `warn`, `error`. |
| `logging.format`  | string | `json`  | One of `json`, `text`.                |

The collector's schema is intentionally minimal today — it grows as
ingestion (Kafka, Day 14) and storage (ClickHouse, Day 15) are
implemented, not before.

## Validating a config file without starting a binary

```bash
pulse-cli config validate -target agent     -file agent.yaml
pulse-cli config validate -target collector -file collector.yaml
```

Exits `0` and prints confirmation if valid; exits `1` and prints the
specific validation error otherwise.

## Example files

See `examples/config/agent.example.yaml` and
`examples/config/collector.example.yaml`.
