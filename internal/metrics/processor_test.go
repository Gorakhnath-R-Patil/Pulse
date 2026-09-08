package metrics_test

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/metrics"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// histogramSampleCount returns how many observations h has recorded in
// total. testutil.CollectAndCount reports the number of metric
// *series* a collector exposes — for an unlabeled Histogram that's
// always 1, observed or not — so it can't distinguish "never
// observed" from "observed once"; this reads the Histogram's own
// exposed sample count instead, the actual number that matters here.
func histogramSampleCount(t *testing.T, h prometheus.Histogram) uint64 {
	t.Helper()
	var m dto.Metric
	if err := h.Write(&m); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	return m.GetHistogram().GetSampleCount()
}

func TestProcessor_CountsEventsByType(t *testing.T) {
	reg := metrics.New()
	p := &metrics.Processor{Registry: reg}

	if err := p.Process(context.Background(), model.Event{Type: "process.start"}); err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}
	if err := p.Process(context.Background(), model.Event{Type: "process.start"}); err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}

	got := testutil.ToFloat64(reg.EventsTotal.WithLabelValues("process.start"))
	if got != 2 {
		t.Errorf("EventsTotal{type=process.start} = %v, want 2", got)
	}
}

func TestProcessor_CountsProcessEventsByType(t *testing.T) {
	reg := metrics.New()
	p := &metrics.Processor{Registry: reg}

	_ = p.Process(context.Background(), model.Event{Type: "process.start"})
	_ = p.Process(context.Background(), model.Event{Type: "process.exit"})
	_ = p.Process(context.Background(), model.Event{Type: "process.exit"})

	if got := testutil.ToFloat64(reg.ProcessEventsTotal.WithLabelValues("process.start")); got != 1 {
		t.Errorf("ProcessEventsTotal{type=process.start} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(reg.ProcessEventsTotal.WithLabelValues("process.exit")); got != 2 {
		t.Errorf("ProcessEventsTotal{type=process.exit} = %v, want 2", got)
	}
}

func TestProcessor_CountsNetworkConnectBySuccess(t *testing.T) {
	reg := metrics.New()
	p := &metrics.Processor{Registry: reg}

	_ = p.Process(context.Background(), model.Event{Type: "network.connect", Attributes: map[string]string{"tcp.connect_success": "true"}})
	_ = p.Process(context.Background(), model.Event{Type: "network.connect", Attributes: map[string]string{"tcp.connect_success": "false"}})
	_ = p.Process(context.Background(), model.Event{Type: "network.connect", Attributes: map[string]string{"tcp.connect_success": "true"}})

	if got := testutil.ToFloat64(reg.NetworkConnectTotal.WithLabelValues("true")); got != 2 {
		t.Errorf("NetworkConnectTotal{success=true} = %v, want 2", got)
	}
	if got := testutil.ToFloat64(reg.NetworkConnectTotal.WithLabelValues("false")); got != 1 {
		t.Errorf("NetworkConnectTotal{success=false} = %v, want 1", got)
	}
}

func TestProcessor_SumsNetworkBytesFromCloseEvents(t *testing.T) {
	reg := metrics.New()
	p := &metrics.Processor{Registry: reg}

	_ = p.Process(context.Background(), model.Event{Type: "network.close", Network: &model.Network{BytesSent: 100, BytesReceived: 200}})
	_ = p.Process(context.Background(), model.Event{Type: "network.close", Network: &model.Network{BytesSent: 50, BytesReceived: 25}})

	if got := testutil.ToFloat64(reg.NetworkBytesSent); got != 150 {
		t.Errorf("NetworkBytesSent = %v, want 150", got)
	}
	if got := testutil.ToFloat64(reg.NetworkBytesReceived); got != 225 {
		t.Errorf("NetworkBytesReceived = %v, want 225", got)
	}
}

func TestProcessor_NetworkCloseWithNilNetworkDoesNotPanic(t *testing.T) {
	reg := metrics.New()
	p := &metrics.Processor{Registry: reg}

	if err := p.Process(context.Background(), model.Event{Type: "network.close"}); err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}
}

func TestProcessor_CountsHTTPRequestsByMethodAndStatus(t *testing.T) {
	reg := metrics.New()
	p := &metrics.Processor{Registry: reg}

	_ = p.Process(context.Background(), model.Event{Type: "http.response", Attributes: map[string]string{"http.method": "GET", "http.status": "200"}})
	_ = p.Process(context.Background(), model.Event{Type: "http.response", Attributes: map[string]string{"http.method": "GET", "http.status": "200"}})
	_ = p.Process(context.Background(), model.Event{Type: "http.response", Attributes: map[string]string{"http.method": "GET", "http.status": "500"}})

	if got := testutil.ToFloat64(reg.HTTPRequestsTotal.WithLabelValues("GET", "200")); got != 2 {
		t.Errorf("HTTPRequestsTotal{method=GET,status=200} = %v, want 2", got)
	}
	if got := testutil.ToFloat64(reg.HTTPRequestsTotal.WithLabelValues("GET", "500")); got != 1 {
		t.Errorf("HTTPRequestsTotal{method=GET,status=500} = %v, want 1", got)
	}
}

func TestProcessor_CountsDNSQueriesByType(t *testing.T) {
	reg := metrics.New()
	p := &metrics.Processor{Registry: reg}

	_ = p.Process(context.Background(), model.Event{Type: "dns.query", Attributes: map[string]string{"dns.qtype": "1"}})

	if got := testutil.ToFloat64(reg.DNSQueriesTotal.WithLabelValues("1")); got != 1 {
		t.Errorf("DNSQueriesTotal{qtype=1} = %v, want 1", got)
	}
}

func TestProcessor_RecordsDNSResponseCodeAndLatency(t *testing.T) {
	reg := metrics.New()
	p := &metrics.Processor{Registry: reg}

	_ = p.Process(context.Background(), model.Event{
		Type: "dns.response",
		Attributes: map[string]string{
			"dns.response_code": "0",
			"dns.latency_ms":    "12.500",
		},
	})

	if got := testutil.ToFloat64(reg.DNSResponseCodeTotal.WithLabelValues("0")); got != 1 {
		t.Errorf("DNSResponseCodeTotal{code=0} = %v, want 1", got)
	}
	if got := histogramSampleCount(t, reg.DNSQueryLatencyMs); got != 1 {
		t.Errorf("DNSQueryLatencyMs sample count = %d, want 1", got)
	}
}

func TestProcessor_DNSResponseWithoutLatencyDoesNotPanic(t *testing.T) {
	reg := metrics.New()
	p := &metrics.Processor{Registry: reg}

	// A dns.response with no dns.latency_ms attribute is a real case
	// (see internal/dns.ToEvent: only set when Latency > 0) — must be
	// handled, not just tolerated by accident.
	if err := p.Process(context.Background(), model.Event{Type: "dns.response", Attributes: map[string]string{"dns.response_code": "0"}}); err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}
	if got := histogramSampleCount(t, reg.DNSQueryLatencyMs); got != 0 {
		t.Errorf("DNSQueryLatencyMs sample count = %d, want 0 (no observation should have been recorded)", got)
	}
}

func TestProcessor_UnknownEventTypeOnlyCountsTotal(t *testing.T) {
	reg := metrics.New()
	p := &metrics.Processor{Registry: reg}

	if err := p.Process(context.Background(), model.Event{Type: "something.new"}); err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}
	if got := testutil.ToFloat64(reg.EventsTotal.WithLabelValues("something.new")); got != 1 {
		t.Errorf("EventsTotal{type=something.new} = %v, want 1", got)
	}
}
