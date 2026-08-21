package agent

import (
	"github.com/Gorakhnath-R-Patil/Pulse/internal/pipeline"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/socket"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// socketLoader is the subset of *socket.Loader's method set this
// package needs, letting tests substitute a fake without touching a
// real kernel. Mirrors processLoader in process.go.
type socketLoader interface {
	Load() error
	Attach() error
	Read() (socket.CloseEvent, error)
	Close() error
}

// socketSource adapts a socketLoader to pipeline.EventSource via
// socket.ToEvent.
type socketSource struct {
	loader   socketLoader
	nodeName string
}

func (s socketSource) Read() (model.Event, error) {
	event, err := s.loader.Read()
	if err != nil {
		return model.Event{}, err
	}
	return socket.ToEvent(event, s.nodeName), nil
}

// newSocketPipeline builds the socket data telemetry pipeline. See
// newProcessPipeline's doc comment for the Load/Attach/Close contract.
//
// Day 06 introduced a hand-rolled bounded, drop-on-full channel here as
// the simplest thing that satisfied that day's "introduce bounded
// buffering" requirement. This pipeline (shared with process.go and
// network.go, both of which had their own equivalent hand-rolled read
// loops) supersedes it: the same bound now comes from
// pipeline.Config.QueueSize, and events aren't dropped under
// backpressure — the reader blocks instead. See
// docs/design/event-pipeline.md.
func (a *App) newSocketPipeline(loader socketLoader) *pipeline.Pipeline {
	return pipeline.New(
		pipeline.Config{Name: "socket data telemetry", Workers: 2, QueueSize: 256},
		containerEnrichingSource{inner: socketSource{loader: loader, nodeName: a.cfg.NodeName}},
		a.logger,
		&pipeline.LoggingProcessor{Logger: a.logger},
	)
}
