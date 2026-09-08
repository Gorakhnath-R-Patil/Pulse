// Package collector contains the pulse-collector application: startup,
// structured logging, optional internal/kafka consumption run through
// the same internal/pipeline every internal/agent capability uses,
// optional internal/storage (ClickHouse) writing of what's consumed,
// and graceful shutdown on context cancellation. See
// docs/design/kafka-transport.md and docs/design/clickhouse-storage.md.
package collector

import (
	"context"
	"log/slog"
	"sync"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/kafka"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/pipeline"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/storage"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/version"
)

// App is the pulse-collector application.
type App struct {
	cfg    config.CollectorConfig
	logger *slog.Logger
}

// New constructs an App from its dependencies.
func New(cfg config.CollectorConfig, logger *slog.Logger) *App {
	return &App{cfg: cfg, logger: logger}
}

// Run starts the collector and blocks until ctx is canceled, then shuts
// down cleanly. It returns nil on a normal, context-driven shutdown.
//
// Both Kafka consumption and ClickHouse storage are best-effort, the
// same sense internal/agent's capability loading is: unconfigured (or,
// for storage, unreachable at startup), the collector simply runs
// without them rather than requiring either to start. Storage is
// nested inside Kafka consumption because it has nothing to do without
// something to consume — there is no other event source yet.
func (a *App) Run(ctx context.Context) error {
	a.logger.Info("pulse-collector starting",
		"version", version.Version,
		"commit", version.Commit,
	)

	if len(a.cfg.KafkaBrokers) == 0 {
		<-ctx.Done()
		a.logger.Info("pulse-collector stopping", "reason", ctx.Err())
		return nil
	}

	consumer := kafka.NewConsumer(kafka.ConsumerConfig{
		Brokers: a.cfg.KafkaBrokers,
		Topic:   a.cfg.KafkaTopic,
		GroupID: a.cfg.KafkaGroupID,
	})
	a.logger.Info("kafka consumption active",
		"brokers", a.cfg.KafkaBrokers,
		"topic", a.cfg.KafkaTopic,
		"group_id", a.cfg.KafkaGroupID,
	)

	var writer *storage.BatchWriter
	var writerDone sync.WaitGroup
	var extra []pipeline.EventProcessor
	if len(a.cfg.ClickHouseAddr) > 0 {
		storageCfg := storage.DefaultConfig(a.cfg.ClickHouseAddr)
		storageCfg.Database = a.cfg.ClickHouseDatabase
		storageCfg.Username = a.cfg.ClickHouseUsername
		storageCfg.Password = a.cfg.ClickHousePassword
		storageCfg.Table = a.cfg.ClickHouseTable

		var err error
		writer, err = storage.NewBatchWriter(ctx, storageCfg, a.logger)
		if err != nil {
			a.logger.Warn("clickhouse storage unavailable", "error", err)
		} else {
			extra = append(extra, &storage.Processor{Writer: writer})
			a.logger.Info("clickhouse storage active",
				"addr", a.cfg.ClickHouseAddr,
				"database", a.cfg.ClickHouseDatabase,
				"table", a.cfg.ClickHouseTable,
			)
			writerDone.Add(1)
			go func() {
				defer writerDone.Done()
				writer.Run(ctx)
			}()
		}
	}

	p := a.newKafkaPipeline(consumer, extra...)
	pipelineDone := make(chan struct{})
	go func() {
		defer close(pipelineDone)
		p.Run(ctx)
	}()

	<-ctx.Done()

	a.logger.Info("pulse-collector stopping", "reason", ctx.Err())

	// Close unblocks kafkaSource.Read's blocked Consume call, the same
	// way internal/agent closes each capability's Loader — see
	// pipeline.Pipeline.Run's own shutdown contract.
	if err := consumer.Close(); err != nil {
		a.logger.Warn("kafka consumer close failed", "error", err)
	}
	<-pipelineDone

	// The writer's own Run goroutine already stops (and flushes
	// whatever was queued) on ctx cancellation; wait for it to actually
	// finish before closing its connection — see internal/agent.Run's
	// identical reasoning for its OTLP exporter.
	if writer != nil {
		writerDone.Wait()
		if err := writer.Close(); err != nil {
			a.logger.Warn("clickhouse writer close failed", "error", err)
		}
	}

	return nil
}
