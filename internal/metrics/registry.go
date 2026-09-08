// Package metrics turns pkg/model.Event values already flowing through
// pulse-agent's and pulse-collector's pipelines into Prometheus
// metrics, exposed over HTTP for scraping. See
// docs/design/metrics.md for what's measured, what isn't, and why
// this uses the official client library rather than hand-rolling the
// exposition format.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Registry holds every metric Processor updates, backed by its own
// *prometheus.Registry rather than the package-level default one
// (prometheus.DefaultRegisterer) — so a test, or a process running
// both an agent-shaped and collector-shaped Registry side by side,
// never fights over global registration state. Every metric name is
// prefixed "pulse_" per Prometheus's own naming convention for a
// project-specific namespace.
type Registry struct {
	registry *prometheus.Registry

	// EventsTotal counts every event processed, labeled by its Type —
	// the one metric with no capability-specific meaning, useful as a
	// coarse "is this pipeline doing anything at all" signal.
	EventsTotal *prometheus.CounterVec

	// ProcessEventsTotal counts process.start/process.exit events,
	// labeled by which.
	ProcessEventsTotal *prometheus.CounterVec

	// NetworkConnectTotal counts network.connect events, labeled by
	// whether the connect succeeded (the event's own
	// "tcp.connect_success" attribute).
	NetworkConnectTotal *prometheus.CounterVec

	// NetworkBytesSent and NetworkBytesReceived sum the byte counters
	// carried on network.close events (see internal/socket) — the
	// only event type with real byte counts; network.connect never
	// has them (see docs/design/network-connect.md).
	NetworkBytesSent     prometheus.Counter
	NetworkBytesReceived prometheus.Counter

	// HTTPRequestsTotal counts http.request/http.response events,
	// labeled by method and status — status is only ever set on a
	// http.response event (see internal/httpvis), so a http.request
	// row always carries an empty status label.
	HTTPRequestsTotal *prometheus.CounterVec

	// DNSQueriesTotal counts dns.query events, labeled by query type.
	DNSQueriesTotal *prometheus.CounterVec

	// DNSResponseCodeTotal counts dns.response events, labeled by
	// response code.
	DNSResponseCodeTotal *prometheus.CounterVec

	// DNSQueryLatencyMs observes internal/dns's own real, computed
	// query→response latency (see docs/design/dns-telemetry.md) — the
	// one latency this project measures directly, rather than
	// inferring it from separately-observed request/response events
	// the way HTTP would have to.
	DNSQueryLatencyMs prometheus.Histogram
}

// New constructs a Registry with every metric registered against its
// own internal *prometheus.Registry.
func New() *Registry {
	reg := prometheus.NewRegistry()

	r := &Registry{
		registry: reg,
		EventsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pulse_events_total",
			Help: "Total telemetry events processed, by event type.",
		}, []string{"type"}),
		ProcessEventsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pulse_process_events_total",
			Help: "Total process start/exit events, by event type.",
		}, []string{"type"}),
		NetworkConnectTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pulse_network_connect_total",
			Help: "Total outbound TCP connect attempts observed, by whether they succeeded.",
		}, []string{"success"}),
		NetworkBytesSent: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pulse_network_bytes_sent_total",
			Help: "Total bytes sent, summed across observed connection closes.",
		}),
		NetworkBytesReceived: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pulse_network_bytes_received_total",
			Help: "Total bytes received, summed across observed connection closes.",
		}),
		HTTPRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pulse_http_requests_total",
			Help: "Total HTTP request/response lines observed, by method and status.",
		}, []string{"method", "status"}),
		DNSQueriesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pulse_dns_queries_total",
			Help: "Total DNS queries observed, by query type.",
		}, []string{"qtype"}),
		DNSResponseCodeTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pulse_dns_response_code_total",
			Help: "Total DNS responses observed, by response code.",
		}, []string{"code"}),
		DNSQueryLatencyMs: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "pulse_dns_query_latency_ms",
			Help: "DNS query round-trip latency in milliseconds, computed from each response's own transaction ID (see internal/dns).",
			// A DNS lookup is normally single-digit-to-low-tens of
			// milliseconds; this range covers a healthy lookup up
			// through a genuinely slow/retried one, without a made-up
			// long tail beyond what's plausible for DNS specifically.
			Buckets: []float64{0.5, 1, 2, 5, 10, 25, 50, 100, 250, 500, 1000},
		}),
	}

	reg.MustRegister(
		r.EventsTotal,
		r.ProcessEventsTotal,
		r.NetworkConnectTotal,
		r.NetworkBytesSent,
		r.NetworkBytesReceived,
		r.HTTPRequestsTotal,
		r.DNSQueriesTotal,
		r.DNSResponseCodeTotal,
		r.DNSQueryLatencyMs,
	)
	return r
}
