package pipeline_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/pipeline"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testEvent(n int) model.Event {
	return model.Event{
		ID:        fmt.Sprintf("event-%d", n),
		Type:      "test.event",
		Timestamp: time.Now(),
		Host:      "pulse-node-1",
	}
}

// sliceSource replays a fixed list of events, then returns terminalErr
// forever — standing in for a real Loader that starts failing once its
// caller closes it during shutdown.
type sliceSource struct {
	events      []model.Event
	terminalErr error

	mu   sync.Mutex
	i    int
	done chan struct{} // closed the first time Read returns terminalErr, if non-nil
}

func (s *sliceSource) Read() (model.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.i < len(s.events) {
		e := s.events[s.i]
		s.i++
		return e, nil
	}
	if s.done != nil {
		select {
		case <-s.done:
		default:
			close(s.done)
		}
	}
	return model.Event{}, s.terminalErr
}

// countingProcessor records every event it's given, safe for
// concurrent use by multiple workers.
type countingProcessor struct {
	mu     sync.Mutex
	events []model.Event
}

func (p *countingProcessor) Process(_ context.Context, event model.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
	return nil
}

func (p *countingProcessor) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.events)
}

// erroringProcessor always fails, recording how many times it ran.
type erroringProcessor struct {
	calls atomic.Int64
}

func (p *erroringProcessor) Process(_ context.Context, _ model.Event) error {
	p.calls.Add(1)
	return errors.New("simulated processor failure")
}

// blockingProcessor blocks each Process call until either unblock is
// closed or ctx is done — a well-behaved processor per EventProcessor's
// documented contract.
type blockingProcessor struct {
	unblock chan struct{}
	calls   atomic.Int64
}

func (p *blockingProcessor) Process(ctx context.Context, _ model.Event) error {
	p.calls.Add(1)
	select {
	case <-p.unblock:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestPipeline_ProcessesAllEventsAndReturns(t *testing.T) {
	const n = 20
	events := make([]model.Event, n)
	for i := range events {
		events[i] = testEvent(i)
	}
	src := &sliceSource{events: events, terminalErr: errors.New("source closed")}
	proc := &countingProcessor{}

	p := pipeline.New(pipeline.Config{Name: "test", Workers: 4, QueueSize: 3}, src, discardLogger(), proc)

	done := make(chan struct{})
	go func() {
		p.Run(context.Background())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after the source was exhausted")
	}

	if got := proc.count(); got != n {
		t.Errorf("processed %d events, want all %d", got, n)
	}
}

func TestPipeline_RunsEveryProcessorForEveryEvent(t *testing.T) {
	src := &sliceSource{
		events:      []model.Event{testEvent(1), testEvent(2), testEvent(3)},
		terminalErr: errors.New("source closed"),
	}
	first := &countingProcessor{}
	second := &countingProcessor{}

	p := pipeline.New(pipeline.Config{Name: "test", Workers: 1, QueueSize: 1}, src, discardLogger(), first, second)

	done := make(chan struct{})
	go func() {
		p.Run(context.Background())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return")
	}

	if first.count() != 3 || second.count() != 3 {
		t.Errorf("first=%d second=%d, want both 3", first.count(), second.count())
	}
}

func TestPipeline_ProcessorErrorDoesNotStopPipeline(t *testing.T) {
	src := &sliceSource{
		events:      []model.Event{testEvent(1), testEvent(2), testEvent(3)},
		terminalErr: errors.New("source closed"),
	}
	failing := &erroringProcessor{}
	counting := &countingProcessor{}

	p := pipeline.New(pipeline.Config{Name: "test", Workers: 1, QueueSize: 1}, src, discardLogger(), failing, counting)

	done := make(chan struct{})
	go func() {
		p.Run(context.Background())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return")
	}

	if failing.calls.Load() != 3 {
		t.Errorf("failing processor ran %d times, want 3 (an error must not skip later events)", failing.calls.Load())
	}
	if counting.count() != 3 {
		t.Errorf("counting processor ran %d times, want 3 (a prior processor's error must not skip later processors)", counting.count())
	}
}

func TestPipeline_Backpressure_ReaderBlocksWhenQueueFull(t *testing.T) {
	events := make([]model.Event, 5)
	for i := range events {
		events[i] = testEvent(i)
	}
	src := &sliceSource{events: events, terminalErr: errors.New("unreachable in this test")}
	proc := &blockingProcessor{unblock: make(chan struct{})}

	// QueueSize 1 + 1 worker bounds how far the reader can get ahead of
	// a stalled processor to at most 3 of the 5 available events: one
	// the worker has already dequeued and is (blocked) processing, one
	// sitting in the queue's buffer, and one the reader has read but is
	// itself blocked trying to send — a buffered channel accepting a
	// send doesn't require a receiver to be ready, so which of these
	// three "slots" fills first is a genuine scheduling race, not
	// something this test can pin down further than the bound itself.
	const maxBeforeBlocking = 3
	p := pipeline.New(pipeline.Config{Name: "test", Workers: 1, QueueSize: 1}, src, discardLogger(), proc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx)

	// Wait for the read count to stabilize (no change for a debounce
	// window) rather than for a specific number, since which exact
	// count it stabilizes at (2 or 3) depends on the scheduling race
	// described above.
	readCount := func() int {
		src.mu.Lock()
		defer src.mu.Unlock()
		return src.i
	}
	deadline := time.Now().Add(2 * time.Second)
	stable := 0
	last := -1
	for time.Now().Before(deadline) {
		cur := readCount()
		if cur == last {
			stable++
			if stable >= 20 { // ~20ms of no change
				break
			}
		} else {
			stable = 0
			last = cur
		}
		time.Sleep(time.Millisecond)
	}

	read := readCount()
	if read == 0 {
		t.Fatal("source read 0 events, want at least 1")
	}
	if read > maxBeforeBlocking {
		t.Errorf("source read %d events while the processor was blocked, want at most %d (backpressure should have stopped it)", read, maxBeforeBlocking)
	}

	close(proc.unblock)
	cancel()
}

func TestPipeline_CtxCancelUnblocksBlockedReader(t *testing.T) {
	events := make([]model.Event, 5)
	for i := range events {
		events[i] = testEvent(i)
	}
	src := &sliceSource{events: events, terminalErr: errors.New("unreachable in this test")}
	proc := &blockingProcessor{unblock: make(chan struct{})} // never unblocked

	p := pipeline.New(pipeline.Config{Name: "test", Workers: 1, QueueSize: 1}, src, discardLogger(), proc)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond) // let it fill the queue and block on the next send
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within 2s of ctx cancellation, even though the blocked processor itself respects ctx")
	}
}
