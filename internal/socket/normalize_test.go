package socket_test

import (
	"net/netip"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/socket"
)

func TestToEvent(t *testing.T) {
	ts := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	ce := socket.CloseEvent{
		Timestamp:     ts,
		PID:           4242,
		SourceAddr:    netip.MustParseAddr("10.0.0.5"),
		DestAddr:      netip.MustParseAddr("93.184.216.34"),
		SourcePort:    51000,
		DestPort:      443,
		BytesSent:     1024,
		BytesReceived: 8192,
		Comm:          "curl",
		SockError:     0,
	}

	got := socket.ToEvent(ce, "pulse-node-1")

	if got.Type != "network.close" {
		t.Errorf("Type = %q, want %q", got.Type, "network.close")
	}
	if !got.Timestamp.Equal(ts) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, ts)
	}
	if got.Network == nil {
		t.Fatal("Network is nil, want it populated")
	}
	if got.Network.BytesSent != 1024 || got.Network.BytesReceived != 8192 {
		t.Errorf("Network byte counts = %d/%d, want 1024/8192", got.Network.BytesSent, got.Network.BytesReceived)
	}
	if _, present := got.Attributes["tcp.sock_error"]; present {
		t.Errorf("Attributes contains tcp.sock_error for a clean close: %v", got.Attributes)
	}

	if err := got.Validate(); err != nil {
		t.Errorf("ToEvent() produced an event that failed Validate(): %v", err)
	}
}

func TestToEvent_WithSockError(t *testing.T) {
	ce := socket.CloseEvent{
		Timestamp:  time.Now(),
		PID:        1,
		SourceAddr: netip.MustParseAddr("127.0.0.1"),
		DestAddr:   netip.MustParseAddr("127.0.0.1"),
		SourcePort: 1,
		DestPort:   2,
		Comm:       "nc",
		SockError:  104, // ECONNRESET
	}

	got := socket.ToEvent(ce, "pulse-node-1")

	if got.Attributes["tcp.sock_error"] != "104" {
		t.Errorf(`Attributes["tcp.sock_error"] = %q, want "104"`, got.Attributes["tcp.sock_error"])
	}

	if err := got.Validate(); err != nil {
		t.Errorf("ToEvent() produced an event that failed Validate(): %v", err)
	}
}
