package collector

import (
	"context"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/pipeline"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// kafkaConsumer is the subset of *kafka.Consumer's method set this
// package needs, letting tests substitute a fake without touching a
// real Kafka broker. Mirrors internal/agent's per-capability loader
// interfaces (e.g. processLoader).
type kafkaConsumer interface {
	Consume(ctx context.Context) (model.Event, error)
	Close() error
}

// kafkaSource adapts a kafkaConsumer to pipeline.EventSource — the
// same shape internal/agent's own per-capability sources (processSource,
// networkSource, ...) already have. Read blocks until an event is
// available or the underlying consumer is closed (see Run in
// collector.go, which closes it on shutdown the same way
// internal/agent closes each capability's Loader): it deliberately
// takes no context of its own, matching every other EventSource in
// this project.
type kafkaSource struct {
	consumer kafkaConsumer
}

func (s kafkaSource) Read() (model.Event, error) {
	return s.consumer.Consume(context.Background())
}

// newKafkaPipeline builds pulse-collector's Kafka consumption pipeline.
// extra mirrors internal/agent's newXPipeline pattern — e.g. a
// *storage.Processor, appended after the always-present
// LoggingProcessor, when ClickHouse storage is configured (see Run).
func (a *App) newKafkaPipeline(consumer kafkaConsumer, extra ...pipeline.EventProcessor) *pipeline.Pipeline {
	processors := append([]pipeline.EventProcessor{
		&pipeline.LoggingProcessor{Logger: a.logger},
	}, extra...)
	return pipeline.New(
		pipeline.Config{Name: "kafka consumption", Workers: 2, QueueSize: 256},
		kafkaSource{consumer: consumer},
		a.logger,
		processors...,
	)
}
