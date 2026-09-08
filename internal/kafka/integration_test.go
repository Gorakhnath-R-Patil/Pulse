package kafka_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/kafka"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// envTestBrokers names the Kafka cluster these integration tests
// produce to and consume from. Unset (the default everywhere except
// CI's dedicated Kafka job — see .github/workflows/ci.yml), they skip
// rather than fail: there is no Kafka broker on this project's Windows
// dev machine, and no Docker to run one locally either — see
// docs/design/kafka-transport.md. Mirrors internal/ebpf's requireRoot
// skip-with-explanation convention for the same underlying reason
// (infrastructure this test genuinely needs isn't available
// everywhere it runs), just gated on an env var instead of privilege.
const envTestBrokers = "PULSE_TEST_KAFKA_BROKERS"

func requireKafka(t *testing.T) []string {
	t.Helper()
	v := os.Getenv(envTestBrokers)
	if v == "" {
		t.Skipf("%s not set: these tests need a real Kafka broker; see docs/design/kafka-transport.md", envTestBrokers)
	}
	return strings.Split(v, ",")
}

// uniqueTopic returns a topic name unlikely to collide with another
// test run or a previous run against the same broker — AllowAutoTopicCreation
// creates it on first produce, and nothing here ever deletes it
// afterward (Kafka topic deletion isn't this package's concern).
func uniqueTopic(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("pulse-test-%s-%d", t.Name(), time.Now().UnixNano())
}

func TestProducerConsumer_RoundTrip(t *testing.T) {
	brokers := requireKafka(t)
	topic := uniqueTopic(t)

	producer := kafka.NewProducer(kafka.ProducerConfig{Brokers: brokers, Topic: topic})
	t.Cleanup(func() { _ = producer.Close() })

	consumer := kafka.NewConsumer(kafka.ConsumerConfig{Brokers: brokers, Topic: topic, GroupID: "pulse-test-" + t.Name()})
	t.Cleanup(func() { _ = consumer.Close() })

	want := model.Event{
		Type: "process.start",
		Host: "pulse-node-1",
		Process: &model.Process{
			PID:     100,
			Command: "sh",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := producer.Produce(ctx, want); err != nil {
		t.Fatalf("Produce() returned error: %v", err)
	}

	got, err := consumer.Consume(ctx)
	if err != nil {
		t.Fatalf("Consume() returned error: %v", err)
	}

	if got.Type != want.Type || got.Host != want.Host {
		t.Errorf("Consume() = %+v, want Type=%q Host=%q", got, want.Type, want.Host)
	}
	if got.Process == nil || got.Process.PID != want.Process.PID || got.Process.Command != want.Process.Command {
		t.Errorf("Consume().Process = %+v, want %+v", got.Process, want.Process)
	}
}

func TestProducerConsumer_MultipleEventsPreserveContent(t *testing.T) {
	brokers := requireKafka(t)
	topic := uniqueTopic(t)

	producer := kafka.NewProducer(kafka.ProducerConfig{Brokers: brokers, Topic: topic})
	t.Cleanup(func() { _ = producer.Close() })

	consumer := kafka.NewConsumer(kafka.ConsumerConfig{Brokers: brokers, Topic: topic, GroupID: "pulse-test-" + t.Name()})
	t.Cleanup(func() { _ = consumer.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const n = 5
	sent := make([]model.Event, n)
	for i := range n {
		sent[i] = model.Event{Type: "network.connect", Host: fmt.Sprintf("node-%d", i)}
		if err := producer.Produce(ctx, sent[i]); err != nil {
			t.Fatalf("Produce() event %d returned error: %v", i, err)
		}
	}

	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		got, err := consumer.Consume(ctx)
		if err != nil {
			t.Fatalf("Consume() event %d returned error: %v", i, err)
		}
		seen[got.Host] = true
	}
	for i := range n {
		host := fmt.Sprintf("node-%d", i)
		if !seen[host] {
			t.Errorf("never consumed an event with host %q", host)
		}
	}
}

func TestConsumer_CloseUnblocksConsume(t *testing.T) {
	brokers := requireKafka(t)
	topic := uniqueTopic(t)

	consumer := kafka.NewConsumer(kafka.ConsumerConfig{Brokers: brokers, Topic: topic, GroupID: "pulse-test-" + t.Name()})

	done := make(chan error, 1)
	go func() {
		_, err := consumer.Consume(context.Background())
		done <- err
	}()

	// Give Consume a moment to actually start waiting on the (empty)
	// topic before closing, so this exercises Close unblocking an
	// in-progress call rather than racing its own start.
	time.Sleep(200 * time.Millisecond)
	if err := consumer.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	select {
	case err := <-done:
		if err == nil {
			t.Error("Consume() returned nil error after Close(), want a non-nil error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Consume() did not return within 5s of Close() being called")
	}
}
