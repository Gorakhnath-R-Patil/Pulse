package agent

import (
	"github.com/Gorakhnath-R-Patil/Pulse/internal/network"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/pipeline"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// networkLoader is the subset of *network.Loader's method set this
// package needs, letting tests substitute a fake without touching a
// real kernel. Mirrors processLoader in process.go.
type networkLoader interface {
	Load() error
	Attach() error
	Read() (network.ConnectEvent, error)
	Close() error
}

// networkSource adapts a networkLoader to pipeline.EventSource via
// network.ToEvent.
type networkSource struct {
	loader   networkLoader
	nodeName string
}

func (s networkSource) Read() (model.Event, error) {
	event, err := s.loader.Read()
	if err != nil {
		return model.Event{}, err
	}
	return network.ToEvent(event, s.nodeName), nil
}

// newNetworkPipeline builds the network connection telemetry pipeline.
// See newProcessPipeline's doc comment for the Load/Attach/Close
// contract.
func (a *App) newNetworkPipeline(loader networkLoader) *pipeline.Pipeline {
	return pipeline.New(
		pipeline.Config{Name: "network connection telemetry", Workers: 2, QueueSize: 256},
		containerEnrichingSource{inner: networkSource{loader: loader, nodeName: a.cfg.NodeName}},
		a.logger,
		&pipeline.LoggingProcessor{Logger: a.logger},
	)
}
