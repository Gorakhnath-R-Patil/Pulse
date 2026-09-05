package dns_test

import (
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/dns"
)

func TestToEvent_Query(t *testing.T) {
	de := dns.DNSEvent{
		Timestamp: time.Now(),
		PID:       100,
		Comm:      "systemd-resolve",
		Name:      "example.com",
		QType:     1,
	}

	got := dns.ToEvent(de, "pulse-node-1")

	if got.Type != "dns.query" {
		t.Errorf("Type = %q, want %q", got.Type, "dns.query")
	}
	if got.Attributes["dns.name"] != "example.com" {
		t.Errorf(`Attributes["dns.name"] = %q, want "example.com"`, got.Attributes["dns.name"])
	}
	if got.Attributes["dns.qtype"] != "1" {
		t.Errorf(`Attributes["dns.qtype"] = %q, want "1"`, got.Attributes["dns.qtype"])
	}
	if _, present := got.Attributes["dns.response_code"]; present {
		t.Error(`Attributes["dns.response_code"] present on a query event`)
	}
	if got.Network != nil {
		t.Errorf("Network = %+v, want nil", got.Network)
	}

	if err := got.Validate(); err != nil {
		t.Errorf("ToEvent() produced an event that failed Validate(): %v", err)
	}
}

func TestToEvent_ResponseWithLatency(t *testing.T) {
	de := dns.DNSEvent{
		Timestamp:    time.Now(),
		PID:          100,
		Comm:         "systemd-resolve",
		IsResponse:   true,
		Name:         "example.com",
		QType:        1,
		ResponseCode: 0,
		AnswerCount:  2,
		Latency:      42500 * time.Microsecond,
	}

	got := dns.ToEvent(de, "pulse-node-1")

	if got.Type != "dns.response" {
		t.Errorf("Type = %q, want %q", got.Type, "dns.response")
	}
	if got.Attributes["dns.response_code"] != "0" {
		t.Errorf(`Attributes["dns.response_code"] = %q, want "0"`, got.Attributes["dns.response_code"])
	}
	if got.Attributes["dns.answer_count"] != "2" {
		t.Errorf(`Attributes["dns.answer_count"] = %q, want "2"`, got.Attributes["dns.answer_count"])
	}
	if got.Attributes["dns.latency_ms"] != "42.500" {
		t.Errorf(`Attributes["dns.latency_ms"] = %q, want "42.500"`, got.Attributes["dns.latency_ms"])
	}

	if err := got.Validate(); err != nil {
		t.Errorf("ToEvent() produced an event that failed Validate(): %v", err)
	}
}

func TestToEvent_ResponseWithoutLatency(t *testing.T) {
	de := dns.DNSEvent{
		Timestamp:  time.Now(),
		PID:        100,
		IsResponse: true,
		Name:       "example.com",
		QType:      1,
	}

	got := dns.ToEvent(de, "pulse-node-1")

	if _, present := got.Attributes["dns.latency_ms"]; present {
		t.Error(`Attributes["dns.latency_ms"] present when no query was correlated`)
	}
}
