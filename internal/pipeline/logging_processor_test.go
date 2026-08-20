package pipeline_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/pipeline"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

func TestLoggingProcessor_LogsCoreFields(t *testing.T) {
	var buf bytes.Buffer
	p := &pipeline.LoggingProcessor{Logger: slog.New(slog.NewJSONHandler(&buf, nil))}

	event := model.Event{
		ID:        "abc-123",
		Type:      "process.start",
		Timestamp: time.Now(),
		Host:      "pulse-node-1",
	}

	if err := p.Process(context.Background(), event); err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `"type":"process.start"`) {
		t.Errorf("log output missing type: %s", out)
	}
	if !strings.Contains(out, `"id":"abc-123"`) {
		t.Errorf("log output missing id: %s", out)
	}
}

func TestLoggingProcessor_LogsProcessAndNetworkFields(t *testing.T) {
	var buf bytes.Buffer
	p := &pipeline.LoggingProcessor{Logger: slog.New(slog.NewJSONHandler(&buf, nil))}

	event := model.Event{
		ID:        "abc-123",
		Type:      "network.close",
		Timestamp: time.Now(),
		Host:      "pulse-node-1",
		Process:   &model.Process{PID: 4242, Command: "curl", Executable: "/usr/bin/curl"},
		Network: &model.Network{
			Protocol:      "tcp",
			Source:        model.Endpoint{Address: "10.0.0.5", Port: 51000},
			Destination:   model.Endpoint{Address: "93.184.216.34", Port: 443},
			BytesSent:     1024,
			BytesReceived: 8192,
		},
		Attributes: map[string]string{"tcp.sock_error": "0"},
	}

	if err := p.Process(context.Background(), event); err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		`"pid":4242`, `"command":"curl"`, `"executable":"/usr/bin/curl"`,
		`"source":"10.0.0.5"`, `"source_port":51000`,
		`"destination":"93.184.216.34"`, `"destination_port":443`,
		`"bytes_sent":1024`, `"bytes_received":8192`,
		`"tcp.sock_error":"0"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %s: %s", want, out)
		}
	}
}

func TestLoggingProcessor_OmitsUnsetSubStructures(t *testing.T) {
	var buf bytes.Buffer
	p := &pipeline.LoggingProcessor{Logger: slog.New(slog.NewJSONHandler(&buf, nil))}

	event := model.Event{ID: "abc", Type: "process.exit", Timestamp: time.Now(), Host: "n1"}

	if err := p.Process(context.Background(), event); err != nil {
		t.Fatalf("Process() returned error: %v", err)
	}

	out := buf.String()
	for _, unwanted := range []string{`"pid"`, `"source"`, `"bytes_sent"`} {
		if strings.Contains(out, unwanted) {
			t.Errorf("log output contains %s for an event with no Process/Network set: %s", unwanted, out)
		}
	}
}
