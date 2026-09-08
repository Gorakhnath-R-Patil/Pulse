package kafka

import (
	"context"
	"fmt"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// ConsumerConfig controls a Consumer's connection and consumer-group
// membership.
type ConsumerConfig struct {
	// Brokers is the Kafka cluster's bootstrap addresses, e.g.
	// []string{"localhost:9092"}.
	Brokers []string

	// Topic is the topic events are consumed from — the same topic a
	// Producer is configured with.
	Topic string

	// GroupID is the Kafka consumer group this Consumer joins. Every
	// pulse-collector instance sharing a GroupID divides the topic's
	// partitions between them, rather than each seeing every message —
	// the standard Kafka scale-out pattern. A single collector still
	// needs a GroupID (there is no group-less mode here): it's what
	// lets a restarted collector resume from where it left off instead
	// of replaying the whole topic.
	GroupID string
}

// Consumer reads model.Event values back off a Kafka topic, decoded via
// model.Unmarshal — the inverse of Producer.Produce.
//
// Consumer wraps a *kafkago.Reader configured for consumer-group mode,
// which commits each message's offset automatically as ReadMessage
// returns it (at-least-once delivery: a message is considered
// delivered once handed to the caller, not once the caller finishes
// processing it — see docs/design/kafka-transport.md's Limitations for
// what that means for a crash mid-processing).
type Consumer struct {
	reader *kafkago.Reader
}

// NewConsumer constructs a Consumer. Like NewProducer, it does not
// connect eagerly.
func NewConsumer(cfg ConsumerConfig) *Consumer {
	return &Consumer{
		reader: kafkago.NewReader(kafkago.ReaderConfig{
			Brokers: cfg.Brokers,
			Topic:   cfg.Topic,
			GroupID: cfg.GroupID,
		}),
	}
}

// Consume blocks until the next event is available, ctx is canceled, or
// Close is called (which unblocks a Consume call in progress the same
// way an internal/ebpf Loader's Close unblocks a blocked Read — see
// that package's Loader.Close for the precedent this follows).
func (c *Consumer) Consume(ctx context.Context) (model.Event, error) {
	msg, err := c.reader.ReadMessage(ctx)
	if err != nil {
		return model.Event{}, fmt.Errorf("kafka: consume: %w", err)
	}
	event, err := model.Unmarshal(msg.Value)
	if err != nil {
		return model.Event{}, fmt.Errorf("kafka: decode event: %w", err)
	}
	return event, nil
}

// Close closes the underlying connection, unblocking any in-progress
// Consume call.
func (c *Consumer) Close() error {
	return c.reader.Close()
}
