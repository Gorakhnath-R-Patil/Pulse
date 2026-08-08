package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_VersionFlagExitsZeroWithoutStarting(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--version"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run() = %d, want 0; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "pulse") {
		t.Errorf("stdout = %q, want version output", stdout.String())
	}
}

func TestRun_InvalidConfigPathExitsNonZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--config", filepath.Join(t.TempDir(), "missing.yaml")}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("run() = %d, want 1; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "configuration error") {
		t.Errorf("stderr = %q, want it to mention a configuration error", stderr.String())
	}
}

func TestRun_UnknownFlagExitsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--not-a-real-flag"}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("run() = %d, want 2 (usage error)", code)
	}
}
