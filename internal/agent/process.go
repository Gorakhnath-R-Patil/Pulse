package agent

import (
	"github.com/Gorakhnath-R-Patil/Pulse/internal/correlation"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/pipeline"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/process"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// processLoader is the subset of *process.Loader's method set this
// package needs, letting tests substitute a fake without touching a
// real kernel.
type processLoader interface {
	Load() error
	Attach() error
	Read() (process.ProcessEvent, error)
	Close() error
}

// processSource adapts a processLoader to pipeline.EventSource,
// normalizing each event — including best-effort executable resolution
// for start events, since only they can reliably resolve one (see
// process.ResolveExecutable's doc comment) — via process.ToEvent.
type processSource struct {
	loader   processLoader
	nodeName string
}

func (s processSource) Read() (model.Event, error) {
	event, err := s.loader.Read()
	if err != nil {
		return model.Event{}, err
	}

	executable := ""
	if event.Type == process.EventStart {
		if exe, err := process.ResolveExecutable(event.PID); err == nil {
			executable = exe
		}
	}

	return process.ToEvent(event, s.nodeName, executable), nil
}

// newProcessPipeline builds the process discovery pipeline. The caller
// is responsible for loader.Load()/Attach() before starting it (Run)
// and loader.Close() during shutdown — see capability/Run in agent.go.
// corrProcessor is shared across every capability's pipeline so their
// events correlate into the same traces — see
// docs/design/trace-correlation.md. extra is appended after it — e.g.
// a *kafka.ProducingProcessor when Kafka production is configured, see
// docs/design/kafka-transport.md — so a capability with nothing extra
// configured (the common case in tests) can omit it entirely.
func (a *App) newProcessPipeline(loader processLoader, corrProcessor *correlation.CorrelatingProcessor, extra ...pipeline.EventProcessor) *pipeline.Pipeline {
	processors := append([]pipeline.EventProcessor{
		&pipeline.LoggingProcessor{Logger: a.logger},
		corrProcessor,
	}, extra...)
	return pipeline.New(
		pipeline.Config{Name: "process discovery", Workers: 2, QueueSize: 256},
		containerEnrichingSource{inner: processSource{loader: loader, nodeName: a.cfg.NodeName}},
		a.logger,
		processors...,
	)
}
