package agent

import (
	"github.com/Gorakhnath-R-Patil/Pulse/internal/correlation"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/dns"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/pipeline"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// dnsLoader is the subset of *dns.Loader's method set this package
// needs, letting tests substitute a fake without touching a real
// kernel. Mirrors processLoader in process.go.
type dnsLoader interface {
	Load() error
	Attach() error
	Read() (dns.DNSEvent, error)
	Close() error
}

// dnsSource adapts a dnsLoader to pipeline.EventSource via dns.ToEvent.
type dnsSource struct {
	loader   dnsLoader
	nodeName string
}

func (s dnsSource) Read() (model.Event, error) {
	event, err := s.loader.Read()
	if err != nil {
		return model.Event{}, err
	}
	return dns.ToEvent(event, s.nodeName), nil
}

// newDNSPipeline builds the DNS telemetry pipeline. See
// newProcessPipeline's doc comment for the Load/Attach/Close contract.
func (a *App) newDNSPipeline(loader dnsLoader, corrProcessor *correlation.CorrelatingProcessor) *pipeline.Pipeline {
	return pipeline.New(
		pipeline.Config{Name: "dns telemetry", Workers: 2, QueueSize: 256},
		containerEnrichingSource{inner: dnsSource{loader: loader, nodeName: a.cfg.NodeName}},
		a.logger,
		&pipeline.LoggingProcessor{Logger: a.logger},
		corrProcessor,
	)
}
