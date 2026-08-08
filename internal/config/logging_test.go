package config_test

import (
	"errors"
	"testing"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
)

func TestLoggingConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.LoggingConfig
		wantErr bool
	}{
		{"defaults are valid", config.DefaultLoggingConfig(), false},
		{"debug+text valid", config.LoggingConfig{Level: "debug", Format: "text"}, false},
		{"unknown level", config.LoggingConfig{Level: "verbose", Format: "json"}, true},
		{"unknown format", config.LoggingConfig{Level: "info", Format: "xml"}, true},
		{"empty config", config.LoggingConfig{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, config.ErrInvalidValue) {
				t.Errorf("Validate() error = %v, want it to wrap ErrInvalidValue", err)
			}
		})
	}
}
