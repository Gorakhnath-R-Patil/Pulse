package collector

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// syncBuffer is a mutex-protected bytes.Buffer — see
// internal/agent/process_test.go's own copy for why: the pipeline
// built by newKafkaPipeline runs in a background goroutine in these
// tests while the test goroutine polls its log output.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// errFakeConsumerClosed is what Consume returns once block has been
// closed (via Close) and no test-specific terminalErr was set — a real
// Consumer's ReadMessage always returns a non-nil error once its
// underlying Reader is closed, and Consume must be faithful to that:
// returning (zero Event, nil) instead would look like a real, empty
// event to a caller, which pipeline.Pipeline.read would then queue and
// loop on indefinitely rather than treating as the end of the stream.
var errFakeConsumerClosed = errors.New("fakeConsumer: closed")

// fakeConsumer is a kafkaConsumer test double: it never touches a real
// broker, so these tests exercise this package's own wiring logic (does
// newKafkaPipeline's kafkaSource adapt correctly, does the resulting
// pipeline log what it consumes) without needing a live Kafka cluster,
// unlike internal/kafka's own integration tests.
//
// block, if non-nil, makes Consume block forever once events is
// exhausted instead of returning terminalErr — mirrors
// fakeProcessLoader in internal/agent/process_test.go.
type fakeConsumer struct {
	events      []model.Event
	terminalErr error
	block       chan struct{}
	closeErr    error

	i int
}

func (f *fakeConsumer) Consume(ctx context.Context) (model.Event, error) {
	if f.i < len(f.events) {
		e := f.events[f.i]
		f.i++
		return e, nil
	}
	if f.block != nil {
		select {
		case <-f.block:
			if f.terminalErr != nil {
				return model.Event{}, f.terminalErr
			}
			return model.Event{}, errFakeConsumerClosed
		case <-ctx.Done():
			return model.Event{}, ctx.Err()
		}
	}
	return model.Event{}, f.terminalErr
}

func (f *fakeConsumer) Close() error {
	if f.block != nil {
		select {
		case <-f.block:
		default:
			close(f.block)
		}
	}
	return f.closeErr
}

func testApp() (*App, *syncBuffer) {
	buf := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(buf, nil))
	return New(config.DefaultCollectorConfig(), logger), buf
}

func TestKafkaPipeline_LogsConsumedEventsEndToEnd(t *testing.T) {
	app, buf := testApp()
	fake := &fakeConsumer{
		events: []model.Event{{Type: "process.start", Process: &model.Process{PID: 100, Command: "sh"}}},
		block:  make(chan struct{}), // keep the pipeline alive without racing buf after the one event
	}

	p := app.newKafkaPipeline(fake)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	deadline := time.After(2 * time.Second)
	for !strings.Contains(buf.String(), `"pid":100`) {
		select {
		case <-deadline:
			t.Fatalf("log output never contained the consumed event's pid: %s", buf.String())
		case <-time.After(time.Millisecond):
		}
	}
	if !strings.Contains(buf.String(), "process.start") {
		t.Errorf("log output missing the event type: %s", buf.String())
	}

	// Mirrors App.Run's real shutdown sequence: cancel ctx, then close
	// the consumer so its blocked Consume unblocks — see
	// internal/agent's own tests for the identical reasoning.
	cancel()
	fake.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pipeline did not shut down after the consumer was closed")
	}
}

func TestKafkaPipeline_ForwardsToExtraProcessors(t *testing.T) {
	app, _ := testApp()
	fake := &fakeConsumer{
		events: []model.Event{{Type: "network.connect"}},
		block:  make(chan struct{}),
	}

	received := make(chan model.Event, 1)
	extra := processorFunc(func(_ context.Context, event model.Event) error {
		received <- event
		return nil
	})

	p := app.newKafkaPipeline(fake, extra)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	select {
	case event := <-received:
		if event.Type != "network.connect" {
			t.Errorf("event.Type = %q, want %q", event.Type, "network.connect")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("extra processor never received the consumed event")
	}

	cancel()
	fake.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pipeline did not shut down after the consumer was closed")
	}
}

// processorFunc adapts a function to pipeline.EventProcessor, letting
// TestKafkaPipeline_ForwardsToExtraProcessors assert on what extra
// actually receives without needing a real storage.Processor (which
// would need a real ClickHouse connection).
type processorFunc func(ctx context.Context, event model.Event) error

func (f processorFunc) Process(ctx context.Context, event model.Event) error { return f(ctx, event) }

// Run's behavior with no Kafka configured at all — the default,
// zero-value config — is already covered by
// TestApp_Run_ReturnsWhenContextCanceled in collector_test.go; nothing
// here duplicates it.

func TestKafkaSource_PropagatesConsumerError(t *testing.T) {
	wantErr := errors.New("consume failed")
	src := kafkaSource{consumer: &fakeConsumer{terminalErr: wantErr}}

	_, err := src.Read()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Read() error = %v, want %v", err, wantErr)
	}
}
