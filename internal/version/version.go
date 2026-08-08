// Package version carries build-time identity for Pulse binaries.
//
// The variables below are intended to be overridden at build time via
// -ldflags, e.g.:
//
//	go build -ldflags "-X github.com/Gorakhnath-R-Patil/Pulse/internal/version.Version=v0.1.0" ./...
//
// See the Makefile for the canonical build invocation. When unset, they
// fall back to development defaults so `go run` and `go test` keep working
// without any special flags.
package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the Pulse release version (e.g. "v0.1.0"). "dev" indicates
	// a build that was not produced through the release process.
	Version = "dev"

	// Commit is the Git commit SHA the binary was built from.
	Commit = "none"

	// BuildDate is the UTC build timestamp in RFC3339 format.
	BuildDate = "unknown"
)

// Info is a structured snapshot of build identity, suitable for embedding in
// structured log lines or CLI output.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// Get returns the current build's Info.
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

// String renders Info as a single human-readable line, used by `<binary>
// --version` output across all Pulse commands.
func (i Info) String() string {
	return fmt.Sprintf("pulse %s (commit %s, built %s, %s/%s, %s)",
		i.Version, i.Commit, i.BuildDate, i.OS, i.Arch, i.GoVersion)
}

// String returns the current build's identity as a single human-readable
// line. It is a convenience wrapper around Get().String().
func String() string {
	return Get().String()
}
