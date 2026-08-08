package version_test

import (
	"strings"
	"testing"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/version"
)

func TestString_ContainsBuildIdentity(t *testing.T) {
	got := version.String()

	for _, want := range []string{"pulse", version.Version, version.Commit, version.BuildDate} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, want it to contain %q", got, want)
		}
	}
}

func TestGet_PopulatesRuntimeFields(t *testing.T) {
	info := version.Get()

	if info.GoVersion == "" {
		t.Error("Get().GoVersion is empty, want the Go runtime version")
	}
	if info.OS == "" {
		t.Error("Get().OS is empty, want a GOOS value")
	}
	if info.Arch == "" {
		t.Error("Get().Arch is empty, want a GOARCH value")
	}
}
