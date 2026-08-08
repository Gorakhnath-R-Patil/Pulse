package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/cli"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/version"
)

func TestExecute_NoArgsPrintsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli.Execute(nil, &stdout, &stderr)

	if code != cli.ExitUsage {
		t.Errorf("code = %d, want %d", code, cli.ExitUsage)
	}
	if !strings.Contains(stderr.String(), "pulse-cli") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}

func TestExecute_UnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli.Execute([]string{"bogus"}, &stdout, &stderr)

	if code != cli.ExitUsage {
		t.Errorf("code = %d, want %d", code, cli.ExitUsage)
	}
	if !strings.Contains(stderr.String(), "bogus") {
		t.Errorf("stderr = %q, want it to name the unknown command", stderr.String())
	}
}

func TestExecute_Version(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli.Execute([]string{"version"}, &stdout, &stderr)

	if code != cli.ExitSuccess {
		t.Fatalf("code = %d, want %d; stderr = %q", code, cli.ExitSuccess, stderr.String())
	}
	if !strings.Contains(stdout.String(), version.Version) {
		t.Errorf("stdout = %q, want it to contain %q", stdout.String(), version.Version)
	}
}

func TestExecute_ConfigValidate_ValidAgentConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte("node_name: n1\nlogging:\n  level: info\n  format: json\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := cli.Execute([]string{"config", "validate", "-target", "agent", "-file", path}, &stdout, &stderr)

	if code != cli.ExitSuccess {
		t.Fatalf("code = %d, want %d; stderr = %q", code, cli.ExitSuccess, stderr.String())
	}
	if !strings.Contains(stdout.String(), "valid") {
		t.Errorf("stdout = %q, want confirmation the config is valid", stdout.String())
	}
}

func TestExecute_ConfigValidate_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(path, []byte("node_name: n1\nlogging:\n  level: loud\n  format: json\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := cli.Execute([]string{"config", "validate", "-target", "agent", "-file", path}, &stdout, &stderr)

	if code != cli.ExitFailure {
		t.Fatalf("code = %d, want %d", code, cli.ExitFailure)
	}
	if !strings.Contains(stderr.String(), "invalid") {
		t.Errorf("stderr = %q, want it to explain the config is invalid", stderr.String())
	}
}

func TestExecute_ConfigValidate_MissingTarget(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli.Execute([]string{"config", "validate", "-file", "irrelevant.yaml"}, &stdout, &stderr)

	if code != cli.ExitUsage {
		t.Fatalf("code = %d, want %d", code, cli.ExitUsage)
	}
}

func TestExecute_ConfigValidate_MissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli.Execute([]string{"config", "validate", "-target", "agent"}, &stdout, &stderr)

	if code != cli.ExitUsage {
		t.Fatalf("code = %d, want %d", code, cli.ExitUsage)
	}
}
