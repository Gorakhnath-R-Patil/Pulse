// Package collector contains the pulse-collector application: startup,
// structured logging, optional internal/kafka consumption, and
// graceful shutdown on context cancellation. Storage (ClickHouse, Day
// 15) doesn't exist yet, so a consumed event today is only logged —
// see docs/design/kafka-transport.md.
package collector

import (
	"context"
	"log/slog"
	"sync"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/kafka"
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
// Kafka consumption is best-effort in the same sense internal/agent's
// capability loading is: if KafkaBrokers isn't configured, the
// collector simply runs without it rather than requiring it to start.
func (a *App) Run(ctx context.Context) error {
	a.logger.Info("pulse-collector starting",
		"version", version.Version,
		"commit", version.Commit,
	)

	var consumer *kafka.Consumer
	var consumeDone sync.WaitGroup
	if len(a.cfg.KafkaBrokers) > 0 {
		consumer = kafka.NewConsumer(kafka.ConsumerConfig{
			Brokers: a.cfg.KafkaBrokers,
			Topic:   a.cfg.KafkaTopic,
			GroupID: a.cfg.KafkaGroupID,
		})
		a.logger.Info("kafka consumption active",
			"brokers", a.cfg.KafkaBrokers,
			"topic", a.cfg.KafkaTopic,
			"group_id", a.cfg.KafkaGroupID,
		)
		consumeDone.Add(1)
		go func() {
			defer consumeDone.Done()
			a.consumeLoop(ctx, consumer)
		}()
	}

	<-ctx.Done()

	a.logger.Info("pulse-collector stopping", "reason", ctx.Err())

	if consumer != nil {
		// Close unblocks a Consume call already in progress, the same
		// way an internal/ebpf Loader's Close unblocks a blocked Read —
		// see consumeLoop and internal/kafka.Consumer.Close.
		if err := consumer.Close(); err != nil {
			a.logger.Warn("kafka consumer close failed", "error", err)
		}
		consumeDone.Wait()
	}

	return nil
}
