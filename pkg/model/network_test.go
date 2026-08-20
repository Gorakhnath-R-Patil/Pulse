package model_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

func TestNetwork_Validate(t *testing.T) {
	tests := []struct {
		name    string
		n       model.Network
		wantErr bool
	}{
		{
			name: "valid",
			n: model.Network{
				Protocol:    "tcp",
				Source:      model.Endpoint{Address: "10.0.0.1", Port: 51234},
				Destination: model.Endpoint{Address: "10.0.0.2", Port: 443},
			},
			wantErr: false,
		},
		{
			name: "missing source address",
			n: model.Network{
				Destination: model.Endpoint{Address: "10.0.0.2", Port: 443},
			},
			wantErr: true,
		},
		{
			name: "missing destination address",
			n: model.Network{
				Source: model.Endpoint{Address: "10.0.0.1", Port: 51234},
			},
			wantErr: true,
		},
		{
			name:    "both addresses missing",
			n:       model.Network{},
			wantErr: true,
		},
		{
			name: "port zero is allowed (not applicable / unknown)",
			n: model.Network{
				Protocol:    "icmp",
				Source:      model.Endpoint{Address: "10.0.0.1"},
				Destination: model.Endpoint{Address: "10.0.0.2"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.n.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, model.ErrMissingField) {
				t.Errorf("Validate() error = %v, want it to wrap ErrMissingField", err)
			}
		})
	}
}

func TestNetwork_ByteCountsValidAndRoundTrip(t *testing.T) {
	n := model.Network{
		Protocol:      "tcp",
		Source:        model.Endpoint{Address: "10.0.0.1", Port: 51234},
		Destination:   model.Endpoint{Address: "10.0.0.2", Port: 443},
		BytesSent:     1024,
		BytesReceived: 8192,
	}

	if err := n.Validate(); err != nil {
		t.Fatalf("Validate() returned error for a network with byte counts set: %v", err)
	}

	data, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("json.Marshal() returned error: %v", err)
	}

	var decoded model.Network
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() returned error: %v", err)
	}
	if decoded.BytesSent != 1024 {
		t.Errorf("BytesSent = %d, want 1024", decoded.BytesSent)
	}
	if decoded.BytesReceived != 8192 {
		t.Errorf("BytesReceived = %d, want 8192", decoded.BytesReceived)
	}
}

func TestNetwork_ZeroByteCountsOmittedFromJSON(t *testing.T) {
	n := model.Network{
		Source:      model.Endpoint{Address: "10.0.0.1"},
		Destination: model.Endpoint{Address: "10.0.0.2"},
	}

	data, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("json.Marshal() returned error: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	for _, field := range []string{"bytes_sent", "bytes_received"} {
		if _, present := raw[field]; present {
			t.Errorf("field %q present for a network with no byte counts set: %s", field, data)
		}
	}
}
