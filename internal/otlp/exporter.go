package otlp

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// Config controls a BatchExporter's connection, batching, and retry
// behavior.
type Config struct {
	// Endpoint is the OTLP/gRPC collector address, e.g. "localhost:4317".
	Endpoint string

	// BatchSize is how many queued spans trigger an immediate flush.
	BatchSize int

	// FlushInterval flushes whatever has queued at least this often,
	// even if BatchSize hasn't been reached, so a quiet period doesn't
	// hold spans indefinitely.
	FlushInterval time.Duration

	// QueueSize bounds how many spans may be waiting for a flush before
	// Enqueue blocks — the same backpressure discipline as
	// internal/pipeline.Config.QueueSize.
	QueueSize int

	// ExportTimeout bounds a single export attempt.
	ExportTimeout time.Duration

	// MaxRetries is how many additional attempts a batch gets, with
	// exponential backoff between them, after its first export attempt
	// fails, before it's dropped.
	MaxRetries int

	// Insecure connects without TLS. Appropriate for a local or
	// sidecar collector; never turn this on for an endpoint reached
	// over an untrusted network.
	Insecure bool
}

// DefaultConfig returns reasonable defaults for everything except
// Endpoint, which has no sensible default — see internal/agent's
// wiring for why an empty Endpoint disables export entirely rather
// than reaching for a made-up default address.
func DefaultConfig(endpoint string) Config {
	return Config{
		Endpoint:      endpoint,
		BatchSize:     512,
		FlushInterval: 5 * time.Second,
		QueueSize:     4096,
		ExportTimeout: 10 * time.Second,
		MaxRetries:    3,
		Insecure:      true,
	}
}

// BatchExporter batches spans and exports them to an OTLP/gRPC
// collector, retrying a failed batch with exponential backoff before
// giving up on it. See docs/design/otlp-export.md.
//
// A BatchExporter is safe for concurrent Enqueue calls (backed by a
// buffered channel); Run must only be called once.
type BatchExporter struct {
	cfg    Config
	client tracepb.TraceServiceClient
	conn   *grpc.ClientConn
	host   string
	logger *slog.Logger

	queue chan model.Span
}

// NewBatchExporter connects to cfg.Endpoint — lazily; grpc.NewClient
// does not block dialing — and returns a BatchExporter ready to have
// Run started. host identifies this agent's machine in exported
// spans' resource attributes.
func NewBatchExporter(cfg Config, host string, logger *slog.Logger) (*BatchExporter, error) {
	var opts []grpc.DialOption
	if cfg.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(cfg.Endpoint, opts...)
	if err != nil {
		return nil, fmt.Errorf("otlp: connecting to %s: %w", cfg.Endpoint, err)
	}

	return &BatchExporter{
		cfg:    cfg,
		client: tracepb.NewTraceServiceClient(conn),
		conn:   conn,
		host:   host,
		logger: logger,
		queue:  make(chan model.Span, cfg.QueueSize),
	}, nil
}

// Enqueue queues span for the next flush, blocking if the queue is
// full — the same backpressure this project's other queues apply,
// rather than silently dropping spans under load.
func (e *BatchExporter) Enqueue(span model.Span) {
	e.queue <- span
}

// Run flushes queued spans — whenever BatchSize is reached, or at
// least every FlushInterval, whichever comes first — until ctx is
// canceled, then drains and flushes whatever was already queued one
// last time (using a fresh, independent timeout, since ctx is already
// done) before returning, so a shutdown doesn't silently drop spans
// Enqueue had already accepted.
func (e *BatchExporter) Run(ctx context.Context) {
	ticker := time.NewTicker(e.cfg.FlushInterval)
	defer ticker.Stop()

	batch := make([]model.Span, 0, e.cfg.BatchSize)

	for {
		select {
		case span := <-e.queue:
			batch = append(batch, span)
			if len(batch) >= e.cfg.BatchSize {
				e.exportWithRetry(ctx, batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				e.exportWithRetry(ctx, batch)
				batch = batch[:0]
			}

		case <-ctx.Done():
		drain:
			for {
				select {
				case span := <-e.queue:
					batch = append(batch, span)
				default:
					break drain
				}
			}
			if len(batch) > 0 {
				finalCtx, cancel := context.WithTimeout(context.Background(), e.cfg.ExportTimeout)
				e.exportWithRetry(finalCtx, batch)
				cancel()
			}
			return
		}
	}
}

// exportWithRetry attempts to export batch, retrying with exponential
// backoff (starting at 100ms, doubling each attempt) up to
// cfg.MaxRetries additional times before giving up and dropping it.
// Export failures never propagate to the caller — telemetry export
// failing is never allowed to be a reason anything upstream of it
// breaks, the same principle internal/agent's capability loading
// already applies.
func (e *BatchExporter) exportWithRetry(ctx context.Context, batch []model.Span) {
	spans := make([]model.Span, len(batch))
	copy(spans, batch)

	var lastErr error
	backoff := 100 * time.Millisecond
	for attempt := 0; attempt <= e.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				e.logger.Warn("otlp export aborted during retry backoff", "spans", len(spans))
				return
			}
			backoff *= 2
		}

		if err := e.export(ctx, spans); err != nil {
			lastErr = err
			continue
		}
		return
	}
	e.logger.Warn("otlp export failed, dropping batch", "spans", len(spans), "error", lastErr)
}

func (e *BatchExporter) export(ctx context.Context, spans []model.Span) error {
	exportCtx, cancel := context.WithTimeout(ctx, e.cfg.ExportTimeout)
	defer cancel()

	req := &tracepb.ExportTraceServiceRequest{
		ResourceSpans: spansToResourceSpans(e.host, spans),
	}
	if _, err := e.client.Export(exportCtx, req); err != nil {
		return fmt.Errorf("otlp: export: %w", err)
	}
	return nil
}

// Close closes the underlying gRPC connection. Call after Run has
// returned.
func (e *BatchExporter) Close() error {
	return e.conn.Close()
}
