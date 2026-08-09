package model_test

import (
	"errors"
	"testing"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

func TestProcess_Validate(t *testing.T) {
	tests := []struct {
		name    string
		p       model.Process
		wantErr bool
	}{
		{"valid", model.Process{PID: 1234, Executable: "/usr/bin/nginx", Command: "nginx"}, false},
		{"minimal valid, pid only", model.Process{PID: 1}, false},
		{"zero pid", model.Process{PID: 0}, true},
		{"negative pid", model.Process{PID: -1}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, model.ErrInvalidField) {
				t.Errorf("Validate() error = %v, want it to wrap ErrInvalidField", err)
			}
		})
	}
}
