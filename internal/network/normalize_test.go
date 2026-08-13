package network_test

import (
	"net/netip"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/network"
)

func TestToEvent(t *testing.T) {
	ts := time.Date(2026, 8, 13, 9, 30, 0, 0, time.UTC)
	ce := network.ConnectEvent{
		Timestamp:  ts,
		PID:        4242,
		SourceAddr: netip.MustParseAddr("10.0.0.5"),
		DestAddr:   netip.MustParseAddr("93.184.216.34"),
		SourcePort: 51000,
		DestPort:   443,
		Comm:       "curl",
		Success:    true,
	}

	got := network.ToEvent(ce, "pulse-node-1")

	if got.Type != "network.connect" {
		t.Errorf("Type = %q, want %q", got.Type, "network.connect")
	}
	if !got.Timestamp.Equal(ts) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, ts)
	}
	if got.Host != "pulse-node-1" {
		t.Errorf("Host = %q, want %q", got.Host, "pulse-node-1")
	}
	if got.ID == "" {
		t.Error("ID is empty, want a generated identifier")
	}

	if got.Process == nil {
		t.Fatal("Process is nil, want it populated")
	}
	if got.Process.PID != 4242 {
		t.Errorf("Process.PID = %d, want 4242", got.Process.PID)
	}
	if got.Process.Command != "curl" {
		t.Errorf("Process.Command = %q, want %q", got.Process.Command, "curl")
	}

	if got.Network == nil {
		t.Fatal("Network is nil, want it populated")
	}
	if got.Network.Protocol != "tcp" {
		t.Errorf("Network.Protocol = %q, want %q", got.Network.Protocol, "tcp")
	}
	if got.Network.Source.Address != "10.0.0.5" || got.Network.Source.Port != 51000 {
		t.Errorf("Network.Source = %+v, want {10.0.0.5 51000}", got.Network.Source)
	}
	if got.Network.Destination.Address != "93.184.216.34" || got.Network.Destination.Port != 443 {
		t.Errorf("Network.Destination = %+v, want {93.184.216.34 443}", got.Network.Destination)
	}

	if got.Attributes["tcp.connect_success"] != "true" {
		t.Errorf(`Attributes["tcp.connect_success"] = %q, want "true"`, got.Attributes["tcp.connect_success"])
	}

	if err := got.Validate(); err != nil {
		t.Errorf("ToEvent() produced an event that failed Validate(): %v", err)
	}
}

func TestToEvent_FailedConnect(t *testing.T) {
	ce := network.ConnectEvent{
		Timestamp:  time.Now(),
		PID:        1,
		SourceAddr: netip.MustParseAddr("127.0.0.1"),
		DestAddr:   netip.MustParseAddr("127.0.0.1"),
		SourcePort: 1234,
		DestPort:   9999,
		Comm:       "nc",
		Success:    false,
	}

	got := network.ToEvent(ce, "pulse-node-1")

	if got.Attributes["tcp.connect_success"] != "false" {
		t.Errorf(`Attributes["tcp.connect_success"] = %q, want "false"`, got.Attributes["tcp.connect_success"])
	}
}
