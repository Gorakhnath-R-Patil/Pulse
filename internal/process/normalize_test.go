package process_test

import (
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/process"
)

func TestToEvent_Start(t *testing.T) {
	ts := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	pe := process.ProcessEvent{
		Timestamp: ts,
		PID:       4242,
		PPID:      1,
		Comm:      "curl",
		Type:      process.EventStart,
	}

	got := process.ToEvent(pe, "pulse-node-1", "/usr/bin/curl")

	if got.Type != "process.start" {
		t.Errorf("Type = %q, want %q", got.Type, "process.start")
	}
	if !got.Timestamp.Equal(ts) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, ts)
	}
	if got.Host != "pulse-node-1" {
		t.Errorf("Host = %q, want %q", got.Host, "pulse-node-1")
	}
	if got.ID == "" {
		t.Error("ID is empty, want a generated identifier")
	}
	if got.Process == nil {
		t.Fatal("Process is nil, want it populated")
	}
	if got.Process.PID != 4242 {
		t.Errorf("Process.PID = %d, want 4242", got.Process.PID)
	}
	if got.Process.Command != "curl" {
		t.Errorf("Process.Command = %q, want %q", got.Process.Command, "curl")
	}
	if got.Process.Executable != "/usr/bin/curl" {
		t.Errorf("Process.Executable = %q, want %q", got.Process.Executable, "/usr/bin/curl")
	}

	if err := got.Validate(); err != nil {
		t.Errorf("ToEvent() produced an event that failed Validate(): %v", err)
	}
}

func TestToEvent_Exit(t *testing.T) {
	pe := process.ProcessEvent{
		Timestamp: time.Now(),
		PID:       4242,
		Comm:      "curl",
		Type:      process.EventExit,
	}

	got := process.ToEvent(pe, "pulse-node-1", "")

	if got.Type != "process.exit" {
		t.Errorf("Type = %q, want %q", got.Type, "process.exit")
	}
	if got.Process.Executable != "" {
		t.Errorf("Process.Executable = %q, want empty (unresolved for exit events)", got.Process.Executable)
	}

	if err := got.Validate(); err != nil {
		t.Errorf("ToEvent() produced an event that failed Validate(): %v", err)
	}
}
