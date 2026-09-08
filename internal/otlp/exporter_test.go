package otlp_test

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/otlp"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// fakeTraceServer is a real, in-process OTLP trace collector — enough
// to exercise BatchExporter's gRPC round trip, batching, retry, and
// timeout behavior with no external infrastructure at all.
type fakeTraceServer struct {
	tracepb.UnimplementedTraceServiceServer

	mu          sync.Mutex
	spansPerReq []int

	failFirstN int32
	calls      int32
	delay      time.Duration
}

func (f *fakeTraceServer) Export(ctx context.Context, req *tracepb.ExportTraceServiceRequest) (*tracepb.ExportTraceServiceResponse, error) {
	n := atomic.AddInt32(&f.calls, 1)

	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if n <= f.failFirstN {
		return nil, status.Error(codes.Unavailable, "simulated collector failure")
	}

	count := 0
	for _, rs := range req.ResourceSpans {
		for _, ss := range rs.ScopeSpans {
			count += len(ss.Spans)
		}
	}

	f.mu.Lock()
	f.spansPerReq = append(f.spansPerReq, count)
	f.mu.Unlock()

	return &tracepb.ExportTraceServiceResponse{}, nil
}

func (f *fakeTraceServer) totalSpans() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for _, n := range f.spansPerReq {
		total += n
	}
	return total
}

func (f *fakeTraceServer) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.spansPerReq)
}

// startFakeCollector starts srv as a real gRPC server on a loopback
// port and returns its address. The server (and its listener) stop
// automatically when t's test finishes.
func startFakeCollector(t *testing.T, srv *fakeTraceServer) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error: %v", err)
	}
	s := grpc.NewServer()
	tracepb.RegisterTraceServiceServer(s, srv)
	go func() { _ = s.Serve(ln) }()
	t.Cleanup(s.Stop)
	return ln.Addr().String()
}

func testSpan(name string) model.Span {
	return model.Span{
		TraceID:   model.NewTraceID(),
		SpanID:    model.NewSpanID(),
		Name:      name,
		Service:   "test-service",
		StartTime: time.Now(),
	}
}

func newTestExporter(t *testing.T, cfg otlp.Config) (*otlp.BatchExporter, *bytes.Buffer) {
	t.Helper()
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	exp, err := otlp.NewBatchExporter(cfg, "pulse-node-1", logger)
	if err != nil {
		t.Fatalf("NewBatchExporter() returned error: %v", err)
	}
	t.Cleanup(func() { _ = exp.Close() })
	return exp, &logBuf
}

func TestBatchExporter_FlushesOnBatchSize(t *testing.T) {
	srv := &fakeTraceServer{}
	addr := startFakeCollector(t, srv)

	cfg := otlp.DefaultConfig(addr)
	cfg.BatchSize = 3
	cfg.FlushInterval = time.Hour // effectively disabled for this test
	exp, _ := newTestExporter(t, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go exp.Run(ctx)

	for i := 0; i < 3; i++ {
		exp.Enqueue(testSpan("span"))
	}

	deadline := time.After(2 * time.Second)
	for srv.totalSpans() < 3 {
		select {
		case <-deadline:
			t.Fatalf("collector received %d spans after 2s, want 3", srv.totalSpans())
		case <-time.After(time.Millisecond):
		}
	}
	if got := srv.requestCount(); got != 1 {
		t.Errorf("requestCount() = %d, want 1 (all 3 spans in one batch)", got)
	}
}

func TestBatchExporter_FlushesOnInterval(t *testing.T) {
	srv := &fakeTraceServer{}
	addr := startFakeCollector(t, srv)

	cfg := otlp.DefaultConfig(addr)
	cfg.BatchSize = 100 // never reached in this test
	cfg.FlushInterval = 20 * time.Millisecond
	exp, _ := newTestExporter(t, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go exp.Run(ctx)

	exp.Enqueue(testSpan("span"))

	deadline := time.After(2 * time.Second)
	for srv.totalSpans() < 1 {
		select {
		case <-deadline:
			t.Fatal("collector never received the queued span within 2s of the flush interval elapsing")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestBatchExporter_RetriesThenSucceeds(t *testing.T) {
	srv := &fakeTraceServer{failFirstN: 2} // fails twice, succeeds on the 3rd attempt
	addr := startFakeCollector(t, srv)

	cfg := otlp.DefaultConfig(addr)
	cfg.BatchSize = 1
	cfg.FlushInterval = time.Hour
	cfg.MaxRetries = 3
	exp, logBuf := newTestExporter(t, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go exp.Run(ctx)

	exp.Enqueue(testSpan("span"))

	deadline := time.After(3 * time.Second) // 100ms + 200ms backoff, generous margin
	for srv.totalSpans() < 1 {
		select {
		case <-deadline:
			t.Fatalf("collector never received the span despite retries; log: %s", logBuf.String())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestBatchExporter_GivesUpAfterMaxRetries(t *testing.T) {
	srv := &fakeTraceServer{failFirstN: 1000} // always fails
	addr := startFakeCollector(t, srv)

	cfg := otlp.DefaultConfig(addr)
	cfg.BatchSize = 1
	cfg.FlushInterval = time.Hour
	cfg.MaxRetries = 1 // one retry: 100ms backoff, then give up
	exp, logBuf := newTestExporter(t, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go exp.Run(ctx)

	exp.Enqueue(testSpan("span"))

	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(logBuf.String(), "otlp export failed, dropping batch") {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("exporter never logged giving up after exhausting retries; log: %s", logBuf.String())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestBatchExporter_ExportTimeout(t *testing.T) {
	srv := &fakeTraceServer{delay: time.Hour} // never responds in time
	addr := startFakeCollector(t, srv)

	cfg := otlp.DefaultConfig(addr)
	cfg.BatchSize = 1
	cfg.FlushInterval = time.Hour
	cfg.ExportTimeout = 50 * time.Millisecond
	cfg.MaxRetries = 0 // fail fast for this test: only care that the timeout fires at all
	exp, logBuf := newTestExporter(t, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go exp.Run(ctx)

	exp.Enqueue(testSpan("span"))

	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(logBuf.String(), "dropping batch") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("ExportTimeout never caused the export attempt to give up")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestBatchExporter_EnqueueBlocksWhenQueueFull(t *testing.T) {
	srv := &fakeTraceServer{}
	addr := startFakeCollector(t, srv)

	cfg := otlp.DefaultConfig(addr)
	cfg.QueueSize = 1
	exp, _ := newTestExporter(t, cfg)
	// Run is deliberately never started: nothing drains the queue, so
	// a second Enqueue must block once the one slot is full.

	exp.Enqueue(testSpan("first")) // fills the only slot

	blocked := make(chan struct{})
	go func() {
		exp.Enqueue(testSpan("second")) // must block
		close(blocked)
	}()

	select {
	case <-blocked:
		t.Fatal("second Enqueue() returned immediately, want it to block with a full queue")
	case <-time.After(100 * time.Millisecond):
		// Expected: still blocked.
	}
}

func TestBatchExporter_ShutdownFlushesRemaining(t *testing.T) {
	srv := &fakeTraceServer{}
	addr := startFakeCollector(t, srv)

	cfg := otlp.DefaultConfig(addr)
	cfg.BatchSize = 100 // never reached
	cfg.FlushInterval = time.Hour
	exp, _ := newTestExporter(t, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	exp.Enqueue(testSpan("a"))
	exp.Enqueue(testSpan("b"))

	done := make(chan struct{})
	go func() {
		exp.Run(ctx)
		close(done)
	}()

	// Give Run a moment to be blocked in its select before canceling,
	// so both Enqueue calls above are guaranteed to have already been
	// accepted onto the channel.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within 2s of context cancellation")
	}

	if got := srv.totalSpans(); got != 2 {
		t.Errorf("collector received %d spans, want 2 (both flushed during shutdown)", got)
	}
}
