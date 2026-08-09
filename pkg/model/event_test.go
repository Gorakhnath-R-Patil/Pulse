package model_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// validEvent returns a minimal Event that passes Validate(), for tests
// to copy and mutate one field at a time.
func validEvent() model.Event {
	return model.Event{
		ID:        model.NewID(),
		Type:      "process.start",
		Timestamp: time.Now(),
		Host:      "pulse-node-1",
	}
}

func TestEvent_Validate_MinimalValid(t *testing.T) {
	if err := validEvent().Validate(); err != nil {
		t.Fatalf("Validate() on a minimal well-formed event returned error: %v", err)
	}
}

func TestEvent_Validate_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(e *model.Event)
		wantErr error
	}{
		{"missing id", func(e *model.Event) { e.ID = "" }, model.ErrMissingField},
		{"missing type", func(e *model.Event) { e.Type = "" }, model.ErrMissingField},
		{"missing timestamp", func(e *model.Event) { e.Timestamp = time.Time{} }, model.ErrMissingField},
		{"missing host", func(e *model.Event) { e.Host = "" }, model.ErrMissingField},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := validEvent()
			tt.mutate(&e)
			err := e.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() error = %v, want it to wrap %v", err, tt.wantErr)
			}
		})
	}
}

func TestEvent_Validate_TypeFormat(t *testing.T) {
	tests := []struct {
		typ     string
		wantErr bool
	}{
		{"process.start", false},
		{"network.connect", false},
		{"dns.query_sent", false},
		{"noDot", true},
		{"Process.Start", true},
		{"process..start", true},
		{".start", true},
		{"process.", true},
		{"process start", true},
	}

	for _, tt := range tests {
		t.Run(tt.typ, func(t *testing.T) {
			e := validEvent()
			e.Type = tt.typ
			err := e.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() with Type=%q error = %v, wantErr %v", tt.typ, err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, model.ErrInvalidField) {
				t.Errorf("Validate() error = %v, want it to wrap ErrInvalidField", err)
			}
		})
	}
}

func TestEvent_Validate_PropagatesProcessError(t *testing.T) {
	e := validEvent()
	e.Process = &model.Process{PID: -1}

	err := e.Validate()
	if !errors.Is(err, model.ErrInvalidField) {
		t.Fatalf("Validate() error = %v, want the nested Process validation error to propagate", err)
	}
}

func TestEvent_Validate_PropagatesNetworkError(t *testing.T) {
	e := validEvent()
	e.Network = &model.Network{} // missing both addresses

	err := e.Validate()
	if !errors.Is(err, model.ErrMissingField) {
		t.Fatalf("Validate() error = %v, want the nested Network validation error to propagate", err)
	}
}

func TestEvent_Validate_WithProcessAndNetworkPopulated(t *testing.T) {
	e := validEvent()
	e.Type = "network.connect"
	e.Process = &model.Process{PID: 4242, Executable: "/usr/bin/curl"}
	e.Network = &model.Network{
		Protocol:    "tcp",
		Source:      model.Endpoint{Address: "10.0.0.5", Port: 51000},
		Destination: model.Endpoint{Address: "10.0.0.9", Port: 443},
	}
	e.Attributes = map[string]string{"dns.query": "example.com"}

	if err := e.Validate(); err != nil {
		t.Fatalf("Validate() on a fully-populated valid event returned error: %v", err)
	}
}
