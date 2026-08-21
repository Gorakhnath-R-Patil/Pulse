package model_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

func TestContainer_Validate(t *testing.T) {
	tests := []struct {
		name    string
		c       model.Container
		wantErr bool
	}{
		{"valid with pod uid", model.Container{ID: "abc123", PodUID: "12345678-90ab-cdef-1234-567890abcdef"}, false},
		{"valid without pod uid (non-Kubernetes)", model.Container{ID: "abc123"}, false},
		{"missing id", model.Container{PodUID: "12345678-90ab-cdef-1234-567890abcdef"}, true},
		{"empty", model.Container{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.c.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, model.ErrMissingField) {
				t.Errorf("Validate() error = %v, want it to wrap ErrMissingField", err)
			}
		})
	}
}

func TestContainer_JSONRoundTrip(t *testing.T) {
	c := model.Container{ID: "abc123", PodUID: "12345678-90ab-cdef-1234-567890abcdef"}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("json.Marshal() returned error: %v", err)
	}

	var decoded model.Container
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() returned error: %v", err)
	}
	if decoded != c {
		t.Errorf("decoded = %+v, want %+v", decoded, c)
	}
}

func TestContainer_PodUIDOmittedWhenEmpty(t *testing.T) {
	data, err := json.Marshal(model.Container{ID: "abc123"})
	if err != nil {
		t.Fatalf("json.Marshal() returned error: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if _, present := raw["pod_uid"]; present {
		t.Errorf("pod_uid present for a non-Kubernetes container: %s", data)
	}
}
