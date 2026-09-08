package metrics

import (
	"context"
	"strconv"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// Processor adapts a Registry to internal/pipeline's EventProcessor
// interface, so recording metrics is one more step in a pipeline's
// processor chain alongside pipeline.LoggingProcessor and whichever
// others are configured — see internal/agent's and
// internal/collector's wiring.
type Processor struct {
	Registry *Registry
}

// Process updates Registry's metrics from event and always returns
// nil: a metrics update can't meaningfully fail in a way worth
// reporting back to the pipeline, the same rationale
// pipeline.LoggingProcessor's own doc comment already gives.
func (p *Processor) Process(_ context.Context, event model.Event) error {
	p.Registry.EventsTotal.WithLabelValues(event.Type).Inc()

	switch event.Type {
	case "process.start", "process.exit":
		p.Registry.ProcessEventsTotal.WithLabelValues(event.Type).Inc()

	case "network.connect":
		p.Registry.NetworkConnectTotal.WithLabelValues(event.Attributes["tcp.connect_success"]).Inc()

	case "network.close":
		if event.Network != nil {
			p.Registry.NetworkBytesSent.Add(float64(event.Network.BytesSent))
			p.Registry.NetworkBytesReceived.Add(float64(event.Network.BytesReceived))
		}

	case "http.request", "http.response":
		p.Registry.HTTPRequestsTotal.WithLabelValues(event.Attributes["http.method"], event.Attributes["http.status"]).Inc()

	case "dns.query":
		p.Registry.DNSQueriesTotal.WithLabelValues(event.Attributes["dns.qtype"]).Inc()

	case "dns.response":
		p.Registry.DNSResponseCodeTotal.WithLabelValues(event.Attributes["dns.response_code"]).Inc()
		if latStr, ok := event.Attributes["dns.latency_ms"]; ok {
			if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
				p.Registry.DNSQueryLatencyMs.Observe(lat)
			}
		}
	}

	return nil
}
