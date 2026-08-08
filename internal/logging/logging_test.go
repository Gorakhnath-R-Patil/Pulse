package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/config"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/logging"
)

func TestNew_RejectsInvalidConfig(t *testing.T) {
	_, err := logging.New(config.LoggingConfig{Level: "loud", Format: "json"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("New() with an invalid config returned nil error, want an error")
	}
}

func TestNew_JSONFormatProducesParseableLines(t *testing.T) {
	var buf bytes.Buffer
	logger, err := logging.New(config.LoggingConfig{Level: "info", Format: "json"}, &buf)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	logger.Info("agent starting", "node_name", "n1")

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if decoded["msg"] != "agent starting" {
		t.Errorf("msg = %v, want %q", decoded["msg"], "agent starting")
	}
	if decoded["node_name"] != "n1" {
		t.Errorf("node_name = %v, want %q", decoded["node_name"], "n1")
	}
}

func TestNew_TextFormatIsHumanReadable(t *testing.T) {
	var buf bytes.Buffer
	logger, err := logging.New(config.LoggingConfig{Level: "info", Format: "text"}, &buf)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	logger.Info("agent starting")

	if !strings.Contains(buf.String(), "agent starting") {
		t.Errorf("output = %q, want it to contain the log message", buf.String())
	}
	if strings.HasPrefix(strings.TrimSpace(buf.String()), "{") {
		t.Errorf("output = %q, looks like JSON but format was text", buf.String())
	}
}

func TestNew_LevelFiltersBelowThreshold(t *testing.T) {
	var buf bytes.Buffer
	logger, err := logging.New(config.LoggingConfig{Level: "warn", Format: "text"}, &buf)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	logger.Debug("should be filtered out")
	logger.Info("should also be filtered out")
	logger.Warn("should appear")

	out := buf.String()
	if strings.Contains(out, "filtered out") {
		t.Errorf("output = %q, want debug/info suppressed at warn level", out)
	}
	if !strings.Contains(out, "should appear") {
		t.Errorf("output = %q, want the warn-level message present", out)
	}
}

func TestNew_ReturnsUsableLogger(t *testing.T) {
	var buf bytes.Buffer
	logger, err := logging.New(config.DefaultLoggingConfig(), &buf)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	if logger == nil {
		t.Fatal("New() returned a nil logger with a nil error")
	}
	var _ *slog.Logger = logger
}
