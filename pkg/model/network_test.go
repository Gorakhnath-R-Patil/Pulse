package model_test

import (
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
