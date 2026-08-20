package socket_test

import (
	"net/netip"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/socket"
)

// BenchmarkToEvent measures the throughput of the one piece of this
// package's per-event work that's platform-independent and safe to
// measure on any machine: turning an already-decoded CloseEvent into a
// pkg/model.Event. This does not measure kernel-side overhead, ring
// buffer read latency, or decodeRawEvent (unexported, and dominated by
// a handful of fixed-offset slice reads that aren't worth a separate
// benchmark) — see docs/design/socket-data.md's Performance section for
// what this number does and doesn't claim.
//
// Run with: go test ./internal/socket/... -bench=. -benchmem
func BenchmarkToEvent(b *testing.B) {
	ce := socket.CloseEvent{
		Timestamp:     time.Now(),
		PID:           4242,
		SourceAddr:    netip.MustParseAddr("10.0.0.5"),
		DestAddr:      netip.MustParseAddr("93.184.216.34"),
		SourcePort:    51000,
		DestPort:      443,
		BytesSent:     1024,
		BytesReceived: 8192,
		Comm:          "curl",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = socket.ToEvent(ce, "pulse-node-1")
	}
}
