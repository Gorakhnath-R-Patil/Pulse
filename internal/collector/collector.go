// Package collector contains the pulse-collector application: startup,
// structured logging, optional internal/kafka consumption run through
// the same internal/pipeline every internal/agent capability uses,
// optional internal/storage (ClickHouse) writing of what's consumed,
// optional internal/metrics exposition, and graceful shutdown on
// context cancellation. See docs/design/kafka-transport.md,
// docs/design/clickhouse-storage.md, and docs/design/metrics.md.
package collector

import (
	"context"
	"log/slog"
	"sync"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/kafka"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/metrics"
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
// Kafka consumption, ClickHouse storage, and the metrics server are
// all best-effort, the same sense internal/agent's capability loading
// is: unconfigured (or, for storage, unreachable at startup), the
// collector simply runs without them rather than requiring any of
// them to start. Storage is nested inside Kafka consumption because it
// has nothing to do without something to consume — there is no other
// event source yet. The metrics server is independent of both: it
// starts (or doesn't) purely based on MetricsAddr, so it can run even
// with nothing yet configured to feed it — an idle collector still
// answers /metrics, just with every counter at zero.
func (a *App) Run(ctx context.Context) error {
	a.logger.Info("pulse-collector starting",
		"version", version.Version,
		"commit", version.Commit,
	)

	var metricsServer *metrics.Server
	var metricsServerDone sync.WaitGroup
	var extra []pipeline.EventProcessor
	if a.cfg.MetricsAddr != "" {
		registry := metrics.New()
		extra = append(extra, &metrics.Processor{Registry: registry})
		metricsServer = metrics.NewServer(a.cfg.MetricsAddr, registry)
		a.logger.Info("metrics server active", "addr", a.cfg.MetricsAddr)
		metricsServerDone.Add(1)
		go func() {
			defer metricsServerDone.Done()
			if err := metricsServer.Run(ctx); err != nil {
				a.logger.Warn("metrics server stopped", "error", err)
			}
		}()
	}

	if len(a.cfg.KafkaBrokers) == 0 {
		<-ctx.Done()
		a.logger.Info("pulse-collector stopping", "reason", ctx.Err())
		metricsServerDone.Wait()
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

	// metricsServer.Run already began its own graceful shutdown the
	// moment ctx was canceled above (it watches the same ctx); wait
	// for that to actually finish before returning.
	metricsServerDone.Wait()

	return nil
}
