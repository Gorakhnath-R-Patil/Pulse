# Kafka Transport

Package: [`internal/kafka`](../../internal/kafka). Carries
[`pkg/model.Event`](../../pkg/model/event.go) values from `pulse-agent`
(producer) to `pulse-collector` (consumer).

## Problem

Every capability's pipeline through Day 13 does two things with each
event: log it, and (if configured) correlate it into a span and export
that span over OTLP. Nothing yet moves the *raw* event itself off the
agent to `pulse-collector` — the second binary this project has built
since Day 1 has never received anything from the first. This day adds
that transport, using Kafka: the architecture diagram in the root
`README.md` has shown `Pulse Agent → Pulse Collector → Kafka →
Storage/Metrics` since before any of it existed, and this is the first
piece of that chain to become real.

Why Kafka specifically, rather than e.g. agents calling the collector
directly (a gRPC/HTTP push, the way `internal/otlp` calls an OTLP
collector): a direct call couples an agent's throughput to however fast
the collector can currently process one event, host by host. A message
queue decouples the two — an agent keeps producing at its own pace even
if the collector is temporarily slow, restarting, or scaled to fewer
instances than there are agents — which is the specific problem Kafka
exists to solve, and exactly why the architecture diagram already put
it there. This is also "boring, proven technology" in the same sense
Kafka is named explicitly in this project's own design principles.

## Design

```
pulse-agent                                    pulse-collector
────────────                                    ───────────────
5 pipelines ──► LoggingProcessor                  kafka.Consumer.Consume(ctx)
            ──► CorrelatingProcessor (+ OTLP)              │
            ──► kafka.ProducingProcessor                   ▼
                        │                          pipeline.LoggingProcessor
                        ▼                          (only thing done with it today —
                kafka.Producer                      storage is Day 15)
                (kafka-go Writer)
                        │
                        ▼
                   Kafka topic
                        │
                        ▼
              kafka.Consumer (consumer group)
              (kafka-go Reader)
```

**One more pipeline processor, not a parallel path.** Kafka production
is wired the same way Day 13 wired OTLP export into
`internal/correlation`: `internal/agent.Run` constructs one
`*kafka.Producer` (only if `KafkaBrokers` is configured) and appends a
`*kafka.ProducingProcessor` wrapping it to every pipeline's processor
chain, alongside — not instead of — `LoggingProcessor` and
`CorrelatingProcessor`. Every capability produces the same raw
`model.Event` its pipeline already normalized; nothing Kafka-specific
happens per capability.

**A new `extra ...pipeline.EventProcessor` parameter on every
`newXPipeline`, rather than a Kafka-specific one.** Day 13 added
`corrProcessor` as an explicit, named parameter because every pipeline
needs exactly one, always in the same role. Kafka production is
different: optional, and — after this day — the second thing (after
`corrProcessor`) plugged into the same chain from outside a pipeline's
own construction. A variadic `extra` parameter lets `internal/agent.Run`
decide what else belongs in the chain (today: nothing, or one
`*kafka.ProducingProcessor`) without every pipeline constructor needing
its own opinion about Kafka specifically — and it costs existing call
sites nothing, since a variadic parameter with no arguments behaves
exactly as before.

**`pulse-collector` gains its own capability-loading shape.**
`internal/collector.App.Run` now mirrors `internal/agent.App.Run`'s own
best-effort pattern: if `KafkaBrokers` is configured, start a consume
loop in the background; if not, run with nothing to consume from, the
same way an agent runs without a capability it couldn't load. A
consumed event is logged via the *same* `pipeline.LoggingProcessor`
`internal/agent`'s pipelines already use — reusing that logic rather
than re-implementing "how do I turn a `model.Event` into log fields" a
second time.

**Producer and Consumer wrap `kafka-go` directly, no custom
batch/retry layer.** `internal/otlp.BatchExporter` (Day 13) hand-rolls
batching, retry with backoff, and graceful shutdown-drain because
gRPC's `TraceServiceClient` has no batching concept of its own — OTLP
export needed that logic to exist somewhere, and building it once in
`internal/otlp` was the right call. Kafka is different: `kafka-go`'s
`Writer` already batches per-partition and retries transient failures
internally, as a well-tested, widely-used implementation detail of the
library this project chose specifically because it's mature.
Reimplementing that logic a second time in `internal/kafka` on top of
it would be pure duplication — worse, a second, less-tested version of
what the dependency already does correctly — not a meaningful design
choice, so `Producer`/`Consumer` stay thin wrappers: encode/decode via
`pkg/model.Marshal`/`Unmarshal`, delegate everything else to
`kafka-go`.

**JSON over the wire, via the same `Marshal`/`Unmarshal` every other
part of this project already uses.** `pkg/model.Event` already has a
JSON contract (`pkg/model/serialize.go`); a Kafka message's value is
just that same encoding. No new schema, no Kafka-specific framing.

## Tradeoffs

- **At-least-once delivery, not exactly-once.** `kafka-go`'s consumer-
  group `Reader.ReadMessage` commits an offset as soon as a message is
  *handed to the caller* — not once the caller finishes acting on it.
  If `pulse-collector` crashes between receiving a message and finishing
  whatever it eventually does with it (today: logging, which is
  effectively instantaneous and low-risk to lose; Day 15's storage
  write is a real future risk), that message can be lost on restart
  rather than redelivered. The alternative (commit only after
  processing succeeds) trades this for the opposite risk — redelivering
  and double-processing a message after a crash — which is Kafka's
  standard at-least-once/at-most-once tradeoff, not something specific
  to this project. Revisit once Day 15's storage step actually has
  something to lose.
- **No ordering guarantee across partitions.** `Producer` uses
  `kafka-go`'s `LeastBytes` balancer and no explicit per-message key, so
  events from the same process can land on different partitions and be
  consumed out of order relative to each other. `internal/correlation`'s
  own trace/span ordering already doesn't depend on Kafka at all (it
  runs upstream, in the agent, before events ever reach a Kafka topic),
  so this doesn't affect trace correctness today — but a future
  consumer that cares about strict per-process ordering would need to
  key messages by PID (or another process-identifying field) to pin
  them to one partition, which nothing here does yet.
- **A single hardcoded consumer group default (`"pulse-collector"`).**
  Multiple `pulse-collector` instances sharing this default all join
  the same group, dividing the topic's partitions between them — the
  intended scale-out behavior. A deployment that wants isolated,
  independent consumers (e.g. two collectors each seeing every message)
  needs to set distinct `kafka_group_id` values explicitly; nothing
  validates that they're distinct across a fleet.

## Failure modes

- **Broker unreachable when `pulse-agent` starts:** `kafka.NewProducer`
  does not connect eagerly (matching `otlp.NewBatchExporter`'s and
  `grpc.NewClient`'s own lazy-connect behavior), so this only surfaces
  once the first `Produce` call actually tries to write — logged as a
  processor error by the pipeline that called it (see
  `pipeline.Pipeline.work`), never fatal to the pipeline itself.
- **Broker unreachable when `pulse-collector` starts:** the same —
  `kafka.NewConsumer` doesn't connect eagerly either; `Consume` blocks
  (or errors, depending on `kafka-go`'s own retry behavior) until a
  broker becomes reachable.
- **Producer misconfigured (empty topic, unresolvable broker
  address):** caught by `AgentConfig.Validate` before `pulse-agent`
  even starts, for the specific case of `kafka_topic` and
  `kafka_brokers` disagreeing about whether Kafka is enabled at all —
  see `internal/config`.

## Performance

Not benchmarked, for the same reason as `internal/otlp` and
`internal/correlation`: no representative production load exists yet.
`kafka-go`'s own internal batching is what determines actual throughput
characteristics here, not any code this package adds on top of it.

## Security

No new sensitive data leaves the process here that wasn't already
computed elsewhere — a produced Kafka message carries exactly the same
`model.Event` a pipeline already logs and (if OTLP export is
configured) correlates into a span. As with `internal/otlp`'s
`Insecure: true` default, this package uses `kafka-go`'s defaults,
which means **no TLS or SASL authentication to the Kafka cluster** —
appropriate only for a broker on infrastructure this deployment already
trusts, not one reachable over an untrusted network. There is no
configuration surface for either yet; a real gap for any deployment
that needs one.

## Limitations

- At-least-once delivery only — see Tradeoffs.
- No message ordering guarantee across partitions — see Tradeoffs.
- No TLS or SASL authentication configuration surface.
- `pulse-collector` only logs what it consumes; there is no storage
  (ClickHouse, Day 15) yet for a consumed event to end up in.
- Integration tests (`internal/kafka/integration_test.go`) need a real
  broker and skip themselves everywhere one isn't configured via
  `PULSE_TEST_KAFKA_BROKERS` — including on this project's own Windows
  dev machine, which has neither a Kafka broker nor Docker to run one.
  CI's dedicated `kafka-integration` job (see
  `.github/workflows/ci.yml`) is what actually exercises them, against
  a real single-node broker running as a GitHub Actions service
  container.
