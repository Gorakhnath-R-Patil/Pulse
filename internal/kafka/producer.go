// Package kafka carries pkg/model.Event values between pulse-agent and
// pulse-collector over a real Kafka topic — a Producer on the agent
// side, a Consumer on the collector side. See
// docs/design/kafka-transport.md for why Kafka sits here (decoupling
// agent throughput from collector processing speed) and what this
// package deliberately doesn't attempt (exactly-once delivery,
// ordering across partitions, schema evolution beyond
// pkg/model.Marshal's own JSON contract).
package kafka

import (
	"context"
	"fmt"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// ProducerConfig controls a Producer's connection and batching.
type ProducerConfig struct {
	// Brokers is the Kafka cluster's bootstrap addresses, e.g.
	// []string{"localhost:9092"}.
	Brokers []string

	// Topic is the topic events are produced to.
	Topic string
}

// Producer publishes model.Event values to a Kafka topic, JSON-encoded
// via model.Marshal — the same wire format internal/config's
// configuration examples and internal/cli already assume elsewhere in
// this project, so a consumer needs no Kafka-specific schema beyond
// "decode this message's value the same way pkg/model already does."
//
// Producer wraps a *kafkago.Writer directly rather than reimplementing
// internal/otlp.BatchExporter's own hand-rolled batch/retry loop:
// kafka-go's Writer already batches per-partition and retries
// internally (see docs/design/kafka-transport.md's Tradeoffs for why
// building a second version of that logic here would be pure
// duplication, not a meaningful design choice).
type Producer struct {
	writer *kafkago.Writer
}

// NewProducer constructs a Producer. It does not connect eagerly —
// kafka-go dials lazily on the first Produce call — so a misconfigured
// or unreachable broker only surfaces once Produce is actually called,
// matching internal/otlp.NewBatchExporter's own lazy-connect behavior.
func NewProducer(cfg ProducerConfig) *Producer {
	return &Producer{
		writer: &kafkago.Writer{
			Addr:                   kafkago.TCP(cfg.Brokers...),
			Topic:                  cfg.Topic,
			Balancer:               &kafkago.LeastBytes{},
			AllowAutoTopicCreation: true,
		},
	}
}

// Produce encodes event and writes it to the configured topic, blocking
// until kafka-go has queued (and, depending on its internal batching
// state, sent) it. A non-nil error means the event was not delivered —
// callers decide whether that's fatal for them; internal/agent's own
// wiring treats it the same as an internal/otlp export failure: logged,
// never propagated further upstream.
func (p *Producer) Produce(ctx context.Context, event model.Event) error {
	data, err := model.Marshal(event)
	if err != nil {
		return fmt.Errorf("kafka: encode event: %w", err)
	}
	if err := p.writer.WriteMessages(ctx, kafkago.Message{Value: data}); err != nil {
		return fmt.Errorf("kafka: produce: %w", err)
	}
	return nil
}

// Close flushes any buffered messages and closes the underlying
// connection.
func (p *Producer) Close() error {
	return p.writer.Close()
}
