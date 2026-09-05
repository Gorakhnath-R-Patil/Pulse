package model_test

import (
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

var (
	traceIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
	spanIDPattern  = regexp.MustCompile(`^[0-9a-f]{16}$`)
)

func TestNewTraceID_Format(t *testing.T) {
	for i := 0; i < 50; i++ {
		id := model.NewTraceID()
		if !traceIDPattern.MatchString(string(id)) {
			t.Fatalf("NewTraceID() = %q, want 32 lowercase hex characters", id)
		}
	}
}

func TestNewSpanID_Format(t *testing.T) {
	for i := 0; i < 50; i++ {
		id := model.NewSpanID()
		if !spanIDPattern.MatchString(string(id)) {
			t.Fatalf("NewSpanID() = %q, want 16 lowercase hex characters", id)
		}
	}
}

func TestNewTraceID_IsUnique(t *testing.T) {
	const n = 2000
	seen := make(map[model.TraceID]bool, n)
	for i := 0; i < n; i++ {
		id := model.NewTraceID()
		if seen[id] {
			t.Fatalf("NewTraceID() produced a duplicate after %d calls: %q", i, id)
		}
		seen[id] = true
	}
}

func TestNewSpanID_IsUnique(t *testing.T) {
	const n = 2000
	seen := make(map[model.SpanID]bool, n)
	for i := 0; i < n; i++ {
		id := model.NewSpanID()
		if seen[id] {
			t.Fatalf("NewSpanID() produced a duplicate after %d calls: %q", i, id)
		}
		seen[id] = true
	}
}

func validSpan() model.Span {
	return model.Span{
		TraceID:   model.NewTraceID(),
		SpanID:    model.NewSpanID(),
		Name:      "network.connect",
		Service:   "order-service",
		StartTime: time.Now(),
	}
}

func TestSpan_Validate_MinimalValid(t *testing.T) {
	if err := validSpan().Validate(); err != nil {
		t.Fatalf("Validate() on a minimal well-formed span returned error: %v", err)
	}
}

func TestSpan_Validate_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(s *model.Span)
	}{
		{"missing trace_id", func(s *model.Span) { s.TraceID = "" }},
		{"missing span_id", func(s *model.Span) { s.SpanID = "" }},
		{"missing name", func(s *model.Span) { s.Name = "" }},
		{"missing service", func(s *model.Span) { s.Service = "" }},
		{"missing start_time", func(s *model.Span) { s.StartTime = time.Time{} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSpan()
			tt.mutate(&s)
			err := s.Validate()
			if !errors.Is(err, model.ErrMissingField) {
				t.Fatalf("Validate() error = %v, want it to wrap ErrMissingField", err)
			}
		})
	}
}

func TestSpan_Validate_RootSpanHasNoParent(t *testing.T) {
	s := validSpan()
	s.ParentSpanID = ""
	if err := s.Validate(); err != nil {
		t.Errorf("Validate() on a root span (empty ParentSpanID) returned error: %v", err)
	}
}

func TestSpan_Duration_ZeroWhenNotEnded(t *testing.T) {
	s := validSpan()
	if got := s.Duration(); got != 0 {
		t.Errorf("Duration() = %v, want 0 for a span with no EndTime", got)
	}
}

func TestSpan_Duration_Computed(t *testing.T) {
	s := validSpan()
	s.EndTime = s.StartTime.Add(150 * time.Millisecond)
	if got := s.Duration(); got != 150*time.Millisecond {
		t.Errorf("Duration() = %v, want 150ms", got)
	}
}
