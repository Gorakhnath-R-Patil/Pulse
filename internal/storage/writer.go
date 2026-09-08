package storage

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// Config controls a BatchWriter's connection, batching, and retry
// behavior. Deliberately the same shape as internal/otlp.Config: both
// batch, flush on size-or-interval, retry with backoff, and drain on
// shutdown — see BatchWriter's doc comment for why, unlike
// internal/kafka, this package builds that logic itself rather than
// leaning on a client library that already has it.
type Config struct {
	// Addr is the ClickHouse cluster's native-protocol addresses, e.g.
	// []string{"localhost:9000"}.
	Addr []string

	// Database, Username, and Password authenticate the connection.
	// Username/Password default to ClickHouse's own out-of-the-box
	// defaults ("default" / "") when left empty — appropriate for a
	// local or trusted-network deployment, not one reachable over an
	// untrusted network (see docs/design/clickhouse-storage.md's
	// Security section: there is no TLS configuration surface here).
	Database string
	Username string
	Password string

	// Table is the table events are inserted into. EnsureSchema creates
	// it (see schema.go) if it doesn't already exist.
	Table string

	// BatchSize is how many queued events trigger an immediate flush.
	BatchSize int

	// FlushInterval flushes whatever has queued at least this often,
	// even if BatchSize hasn't been reached.
	FlushInterval time.Duration

	// QueueSize bounds how many events may be waiting for a flush
	// before Enqueue blocks.
	QueueSize int

	// WriteTimeout bounds a single batch insert attempt.
	WriteTimeout time.Duration

	// MaxRetries is how many additional attempts a batch gets, with
	// exponential backoff between them, before it's dropped.
	MaxRetries int
}

// DefaultConfig returns reasonable defaults for everything except Addr,
// which has no sensible default — see internal/collector's wiring for
// why an unconfigured ClickHouseAddr disables storage entirely rather
// than reaching for a made-up default address.
func DefaultConfig(addr []string) Config {
	return Config{
		Addr:          addr,
		Database:      "pulse",
		Table:         "events",
		BatchSize:     1000,
		FlushInterval: 5 * time.Second,
		QueueSize:     8192,
		WriteTimeout:  10 * time.Second,
		MaxRetries:    3,
	}
}

// BatchWriter batches consumed events and inserts them into ClickHouse,
// retrying a failed batch with exponential backoff before giving up on
// it. See docs/design/clickhouse-storage.md.
//
// Unlike internal/kafka, which deliberately does *not* build its own
// batch/retry layer on top of kafka-go (that library already batches
// and retries internally, so a second version of that logic would be
// pure duplication — see docs/design/kafka-transport.md), ClickHouse's
// own Go client does not batch writes for you: driver.Conn.PrepareBatch
// builds exactly the batch you construct, sent exactly when you call
// Send. Batching many rows per insert is also a real, well-documented
// ClickHouse performance requirement (frequent small inserts are its
// canonical anti-pattern), not an optional nicety — so this package
// needs the same shape internal/otlp.BatchExporter already has, for a
// genuinely different reason than Kafka's.
//
// A BatchWriter is safe for concurrent Enqueue calls (backed by a
// buffered channel); Run must only be called once.
type BatchWriter struct {
	cfg    Config
	conn   driver.Conn
	logger *slog.Logger

	queue chan model.Event
}

// NewBatchWriter connects to cfg.Addr and ensures cfg.Database and its
// target table exist (see EnsureSchema) before returning. Unlike
// otlp.NewBatchExporter and kafka.NewProducer, this connects and
// verifies schema eagerly rather than lazily: there is no equivalent
// to gRPC/kafka-go's lazy dial here, and failing fast here — during
// startup, alongside every other capability's Load/Attach — is more
// useful than failing on the first Enqueue-triggered flush, arbitrarily
// later.
//
// Connecting is two-step: first to ClickHouse's own always-present
// "default" database, to create cfg.Database if it doesn't exist yet
// (a session can't set a nonexistent database as its own default —
// clickhouse-go would fail to connect at all otherwise); then a second,
// real connection with cfg.Database as the default, which every
// subsequent query (EnsureSchema's CREATE TABLE, and every batch
// insert in write) relies on to name cfg.Table unqualified.
func NewBatchWriter(ctx context.Context, cfg Config, logger *slog.Logger) (*BatchWriter, error) {
	bootstrap, err := clickhouse.Open(&clickhouse.Options{
		Addr: cfg.Addr,
		Auth: clickhouse.Auth{
			Database: "default",
			Username: cfg.Username,
			Password: cfg.Password,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("storage: connecting to %v: %w", cfg.Addr, err)
	}
	if err := bootstrap.Exec(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", cfg.Database)); err != nil {
		bootstrap.Close()
		return nil, fmt.Errorf("storage: create database %s: %w", cfg.Database, err)
	}
	bootstrap.Close()

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: cfg.Addr,
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("storage: connecting to %v (database %s): %w", cfg.Addr, cfg.Database, err)
	}
	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("storage: ping %v: %w", cfg.Addr, err)
	}
	if err := EnsureSchema(ctx, conn, cfg.Table); err != nil {
		return nil, err
	}

	return &BatchWriter{
		cfg:    cfg,
		conn:   conn,
		logger: logger,
		queue:  make(chan model.Event, cfg.QueueSize),
	}, nil
}

// Enqueue queues event for the next flush, blocking if the queue is
// full — the same backpressure internal/otlp.BatchExporter.Enqueue and
// internal/pipeline apply, rather than silently dropping events under
// load.
func (w *BatchWriter) Enqueue(event model.Event) {
	w.queue <- event
}

// Run flushes queued events — whenever BatchSize is reached, or at
// least every FlushInterval, whichever comes first — until ctx is
// canceled, then drains and flushes whatever was already queued one
// last time before returning. See otlp.BatchExporter.Run, which this
// mirrors exactly.
func (w *BatchWriter) Run(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.FlushInterval)
	defer ticker.Stop()

	batch := make([]model.Event, 0, w.cfg.BatchSize)

	for {
		select {
		case event := <-w.queue:
			batch = append(batch, event)
			if len(batch) >= w.cfg.BatchSize {
				w.writeWithRetry(ctx, batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				w.writeWithRetry(ctx, batch)
				batch = batch[:0]
			}

		case <-ctx.Done():
		drain:
			for {
				select {
				case event := <-w.queue:
					batch = append(batch, event)
				default:
					break drain
				}
			}
			if len(batch) > 0 {
				finalCtx, cancel := context.WithTimeout(context.Background(), w.cfg.WriteTimeout)
				w.writeWithRetry(finalCtx, batch)
				cancel()
			}
			return
		}
	}
}

// writeWithRetry attempts to insert batch, retrying with exponential
// backoff (starting at 100ms, doubling each attempt) up to
// cfg.MaxRetries additional times before giving up and dropping it.
// Write failures never propagate to the caller — storage failing is
// never allowed to be a reason event consumption itself breaks, the
// same principle internal/otlp.BatchExporter.exportWithRetry already
// establishes.
func (w *BatchWriter) writeWithRetry(ctx context.Context, batch []model.Event) {
	events := make([]model.Event, len(batch))
	copy(events, batch)

	var lastErr error
	backoff := 100 * time.Millisecond
	for attempt := 0; attempt <= w.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				w.logger.Warn("clickhouse write aborted during retry backoff", "events", len(events))
				return
			}
			backoff *= 2
		}

		if err := w.write(ctx, events); err != nil {
			lastErr = err
			continue
		}
		return
	}
	w.logger.Warn("clickhouse write failed, dropping batch", "events", len(events), "error", lastErr)
}

func (w *BatchWriter) write(ctx context.Context, events []model.Event) error {
	writeCtx, cancel := context.WithTimeout(ctx, w.cfg.WriteTimeout)
	defer cancel()

	batch, err := w.conn.PrepareBatch(writeCtx, fmt.Sprintf("INSERT INTO %s", w.cfg.Table))
	if err != nil {
		return fmt.Errorf("storage: prepare batch: %w", err)
	}
	for _, e := range events {
		if err := appendEvent(batch, e); err != nil {
			return fmt.Errorf("storage: append event %s: %w", e.ID, err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("storage: send batch: %w", err)
	}
	return nil
}

// Close closes the underlying ClickHouse connection. Call after Run has
// returned.
func (w *BatchWriter) Close() error {
	return w.conn.Close()
}
