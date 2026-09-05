package httpvis_test

import (
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/httpvis"
)

func TestToEvent_Request(t *testing.T) {
	he := httpvis.HTTPEvent{
		Timestamp: time.Now(),
		PID:       100,
		Comm:      "curl",
		Size:      42,
		Method:    "GET",
		Path:      "/foo",
	}

	got := httpvis.ToEvent(he, "pulse-node-1")

	if got.Type != "http.request" {
		t.Errorf("Type = %q, want %q", got.Type, "http.request")
	}
	if got.Process == nil || got.Process.PID != 100 || got.Process.Command != "curl" {
		t.Errorf("Process = %+v, want PID=100 Command=curl", got.Process)
	}
	if got.Network != nil {
		t.Errorf("Network = %+v, want nil (this technique doesn't resolve a socket/connection)", got.Network)
	}
	if got.Attributes["http.method"] != "GET" {
		t.Errorf(`Attributes["http.method"] = %q, want "GET"`, got.Attributes["http.method"])
	}
	if got.Attributes["http.path"] != "/foo" {
		t.Errorf(`Attributes["http.path"] = %q, want "/foo"`, got.Attributes["http.path"])
	}
	if _, present := got.Attributes["http.status"]; present {
		t.Error(`Attributes["http.status"] present on a request event`)
	}
	if got.Attributes["http.observed_bytes"] != "42" {
		t.Errorf(`Attributes["http.observed_bytes"] = %q, want "42"`, got.Attributes["http.observed_bytes"])
	}

	if err := got.Validate(); err != nil {
		t.Errorf("ToEvent() produced an event that failed Validate(): %v", err)
	}
}

func TestToEvent_Response(t *testing.T) {
	he := httpvis.HTTPEvent{
		Timestamp:  time.Now(),
		PID:        200,
		Comm:       "nginx",
		Size:       15,
		IsResponse: true,
		Status:     "404",
	}

	got := httpvis.ToEvent(he, "pulse-node-1")

	if got.Type != "http.response" {
		t.Errorf("Type = %q, want %q", got.Type, "http.response")
	}
	if got.Attributes["http.status"] != "404" {
		t.Errorf(`Attributes["http.status"] = %q, want "404"`, got.Attributes["http.status"])
	}
	if _, present := got.Attributes["http.method"]; present {
		t.Error(`Attributes["http.method"] present on a response event`)
	}

	if err := got.Validate(); err != nil {
		t.Errorf("ToEvent() produced an event that failed Validate(): %v", err)
	}
}
