package model_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

func TestMarshalUnmarshal_RoundTrip(t *testing.T) {
	original := model.Event{
		ID:        model.NewID(),
		Type:      "network.connect",
		Timestamp: time.Now().UTC().Truncate(time.Nanosecond),
		Host:      "pulse-node-1",
		Process: &model.Process{
			PID:        4242,
			Executable: "/usr/bin/curl",
			Command:    "curl",
		},
		Network: &model.Network{
			Protocol:    "tcp",
			Source:      model.Endpoint{Address: "10.0.0.5", Port: 51000},
			Destination: model.Endpoint{Address: "10.0.0.9", Port: 443},
		},
		Attributes: map[string]string{"dns.query": "example.com"},
	}

	data, err := model.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() returned error: %v", err)
	}

	decoded, err := model.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal() returned error: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Type != original.Type {
		t.Errorf("Type = %q, want %q", decoded.Type, original.Type)
	}
	if !decoded.Timestamp.Equal(original.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", decoded.Timestamp, original.Timestamp)
	}
	if decoded.Host != original.Host {
		t.Errorf("Host = %q, want %q", decoded.Host, original.Host)
	}
	if decoded.Process == nil || *decoded.Process != *original.Process {
		t.Errorf("Process = %+v, want %+v", decoded.Process, original.Process)
	}
	if decoded.Network == nil || *decoded.Network != *original.Network {
		t.Errorf("Network = %+v, want %+v", decoded.Network, original.Network)
	}
	if decoded.Attributes["dns.query"] != "example.com" {
		t.Errorf("Attributes[\"dns.query\"] = %q, want %q", decoded.Attributes["dns.query"], "example.com")
	}

	if err := decoded.Validate(); err != nil {
		t.Errorf("round-tripped event failed Validate(): %v", err)
	}
}

func TestMarshal_MinimalEventOmitsUnsetFields(t *testing.T) {
	e := validEvent()

	data, err := model.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal() returned error: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	for _, field := range []string{"process", "network", "attributes"} {
		if _, present := raw[field]; present {
			t.Errorf("field %q present in output for an event that didn't set it: %s", field, data)
		}
	}
	for _, field := range []string{"id", "type", "timestamp", "host"} {
		if _, present := raw[field]; !present {
			t.Errorf("required field %q missing from output: %s", field, data)
		}
	}
}

func TestUnmarshal_InvalidJSON(t *testing.T) {
	_, err := model.Unmarshal([]byte("{not valid json"))
	if err == nil {
		t.Fatal("Unmarshal() with malformed JSON returned nil error")
	}
}

func TestUnmarshal_ValidJSONMissingRequiredFieldFailsValidateNotUnmarshal(t *testing.T) {
	// Unmarshal only decodes; it's Validate's job to catch missing
	// required fields, so parseable-but-incomplete JSON should decode
	// successfully and then fail validation.
	e, err := model.Unmarshal([]byte(`{"type":"process.start"}`))
	if err != nil {
		t.Fatalf("Unmarshal() returned error for parseable JSON: %v", err)
	}
	if err := e.Validate(); err == nil {
		t.Fatal("Validate() on a decoded event missing id/timestamp/host returned nil error")
	}
}
