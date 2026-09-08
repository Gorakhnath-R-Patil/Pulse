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

// consumeLoop reads events from consumer and logs each one, reusing
// internal/pipeline.LoggingProcessor's own event-to-log-fields logic
// rather than duplicating it — a consumed model.Event is the same
// shape a pipeline processes, just arriving from Kafka instead of a
// local capability. It returns once Consume fails: the expected
// outcome of either ctx being canceled or Close being called during
// shutdown (see Run), logged only if it wasn't.
func (a *App) consumeLoop(ctx context.Context, consumer kafkaConsumer) {
	logProcessor := &pipeline.LoggingProcessor{Logger: a.logger}
	for {
		event, err := consumer.Consume(ctx)
		if err != nil {
			if ctx.Err() == nil {
				a.logger.Warn("kafka consume failed", "error", err)
			}
			return
		}
		_ = logProcessor.Process(ctx, event) // never returns an error
	}
}
