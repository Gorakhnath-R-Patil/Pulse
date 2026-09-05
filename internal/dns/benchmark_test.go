package dns

import (
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// BenchmarkParseMessage measures the one piece of this package's
// per-event work substantial enough to be worth measuring: decoding a
// captured payload's DNS header and walking its question name's
// labels. Unlike the simple fixed-offset reads in decodeRawEvent (not
// separately benchmarked, matching internal/socket's precedent), this
// one loops over label bytes and builds a string, so it's a real
// candidate for a hot-path cost — see docs/design/dns-telemetry.md's
// Performance section for what this number does and doesn't claim.
//
// Run with: go test ./internal/dns/... -bench=. -benchmem
func BenchmarkParseMessage(b *testing.B) {
	data := buildQuery(0x1234, "www.example.com", 1)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := parseMessage(data); err != nil {
			b.Fatalf("parseMessage() returned error: %v", err)
		}
	}
}

// BenchmarkToEvent measures the cost of turning an already-decoded
// DNSEvent into a pkg/model.Event.
func BenchmarkToEvent(b *testing.B) {
	de := DNSEvent{
		Timestamp:    time.Now(),
		PID:          4242,
		Comm:         "systemd-resolve",
		IsResponse:   true,
		Name:         "www.example.com",
		QType:        1,
		ResponseCode: 0,
		AnswerCount:  2,
		Latency:      12 * time.Millisecond,
	}

	b.ReportAllocs()
	b.ResetTimer()
	var event model.Event
	for i := 0; i < b.N; i++ {
		event = ToEvent(de, "pulse-node-1")
	}
	_ = event
}
