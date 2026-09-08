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
3. **Environment variables** — `PULSE_LOG_LEVEL` and `PULSE_LOG_FORMAT`
   override `logging.level`/`logging.format` for both binaries;
   `PULSE_OTLP_ENDPOINT` (`pulse-agent` only) overrides `otlp_endpoint`;
   `PULSE_KAFKA_BROKERS` (comma-separated) and `PULSE_KAFKA_TOPIC`
   override `kafka_brokers`/`kafka_topic` for both binaries;
   `PULSE_KAFKA_GROUP_ID` (`pulse-collector` only) overrides
   `kafka_group_id`; `PULSE_CLICKHOUSE_ADDR` (comma-separated),
   `PULSE_CLICKHOUSE_DATABASE`, `PULSE_CLICKHOUSE_USERNAME`,
   `PULSE_CLICKHOUSE_PASSWORD`, and `PULSE_CLICKHOUSE_TABLE`
   (`pulse-collector` only) override the corresponding
   `clickhouse_*` fields. This matches the common container pattern of
   overriding a mounted config file at deploy time without editing it.
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
| `otlp_endpoint`   | string | (empty, export disabled) | OTLP/gRPC collector address, e.g. `localhost:4317`. See `docs/design/otlp-export.md`. |
| `kafka_brokers`   | list of strings | (empty, production disabled) | Kafka bootstrap addresses, e.g. `[localhost:9092]`. Must be set together with `kafka_topic`. See `docs/design/kafka-transport.md`. |
| `kafka_topic`     | string | (empty) | Topic events are produced to. Must be set together with `kafka_brokers`. |

## `pulse-collector` fields

| Field             | Type   | Default | Description                          |
|-------------------|--------|---------|---------------------------------------|
| `logging.level`   | string | `info`  | One of `debug`, `info`, `warn`, `error`. |
| `logging.format`  | string | `json`  | One of `json`, `text`.                |
| `kafka_brokers`   | list of strings | (empty, consumption disabled) | Kafka bootstrap addresses. Must be set together with `kafka_topic`. |
| `kafka_topic`     | string | (empty) | Topic events are consumed from — the same topic a producing `pulse-agent` is configured with. |
| `kafka_group_id`  | string | `pulse-collector` (once `kafka_brokers` is set) | Kafka consumer group this collector joins. |
| `clickhouse_addr`     | list of strings | (empty, storage disabled) | ClickHouse native-protocol addresses, e.g. `[localhost:9000]`. See `docs/design/clickhouse-storage.md`. |
| `clickhouse_database` | string | `pulse` (once `clickhouse_addr` is set) | Database events are stored in; created automatically if it doesn't exist. |
| `clickhouse_username` | string | (empty → ClickHouse's own `default`) | |
| `clickhouse_password` | string | (empty) | |
| `clickhouse_table`    | string | `events` (once `clickhouse_addr` is set) | Table events are stored in; created automatically if it doesn't exist. |

The collector's schema grows as ingestion (Kafka, Day 14) and storage
(ClickHouse, Day 15 — both above) are implemented.

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
