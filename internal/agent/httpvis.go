package agent

import (
	"github.com/Gorakhnath-R-Patil/Pulse/internal/correlation"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/httpvis"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/pipeline"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// httpvisLoader is the subset of *httpvis.Loader's method set this
// package needs, letting tests substitute a fake without touching a
// real kernel. Mirrors processLoader in process.go.
type httpvisLoader interface {
	Load() error
	Attach() error
	Read() (httpvis.HTTPEvent, error)
	Close() error
}

// httpvisSource adapts an httpvisLoader to pipeline.EventSource via
// httpvis.ToEvent.
type httpvisSource struct {
	loader   httpvisLoader
	nodeName string
}

func (s httpvisSource) Read() (model.Event, error) {
	event, err := s.loader.Read()
	if err != nil {
		return model.Event{}, err
	}
	return httpvis.ToEvent(event, s.nodeName), nil
}

// newHTTPVisPipeline builds the HTTP visibility pipeline. See
// newProcessPipeline's doc comment for the Load/Attach/Close contract.
func (a *App) newHTTPVisPipeline(loader httpvisLoader, corrProcessor *correlation.CorrelatingProcessor) *pipeline.Pipeline {
	return pipeline.New(
		pipeline.Config{Name: "http visibility", Workers: 2, QueueSize: 256},
		containerEnrichingSource{inner: httpvisSource{loader: loader, nodeName: a.cfg.NodeName}},
		a.logger,
		&pipeline.LoggingProcessor{Logger: a.logger},
		corrProcessor,
	)
}
