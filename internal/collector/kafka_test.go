package collector

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// syncBuffer is a mutex-protected bytes.Buffer — see
// internal/agent/process_test.go's own copy for why: consumeLoop runs
// in a background goroutine in these tests while the test goroutine
// polls its log output.
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

// fakeConsumer is a kafkaConsumer test double: it never touches a real
// broker, so these tests exercise consumeLoop's own wiring logic
// (does it log what it consumes, does it stop cleanly) without
// needing a live Kafka cluster, unlike internal/kafka's own consumer
// integration tests.
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

func TestConsumeLoop_LogsConsumedEvents(t *testing.T) {
	buf := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(buf, nil))
	app := New(config.DefaultCollectorConfig(), logger)

	fake := &fakeConsumer{
		events: []model.Event{{Type: "process.start", Process: &model.Process{PID: 100, Command: "sh"}}},
		block:  make(chan struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		app.consumeLoop(ctx, fake)
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

	cancel()
	fake.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("consumeLoop did not return after ctx was canceled and the consumer was closed")
	}
}

func TestConsumeLoop_ReturnsWhenConsumeFails(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	app := New(config.DefaultCollectorConfig(), logger)
	fake := &fakeConsumer{terminalErr: errors.New("consume failed")}

	done := make(chan struct{})
	go func() {
		app.consumeLoop(context.Background(), fake)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("consumeLoop did not return after Consume returned a terminal error")
	}
}
