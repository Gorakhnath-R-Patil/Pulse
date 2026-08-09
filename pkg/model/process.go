package model

import "fmt"

// Process identifies the process associated with an observed activity.
// It is deliberately narrow today: just enough to say "which process."
// Container, pod, namespace, and service identity are a distinct
// enrichment concern with their own lifecycle and data sources, and are
// introduced by the Day 8 service identity model rather than bolted onto
// Process here — see docs/design/event-model.md.
type Process struct {
	// PID is the process ID as seen by the kernel that observed it.
	PID int32 `json:"pid"`

	// Executable is the resolved path to the running binary, if known
	// (e.g. "/usr/bin/nginx").
	Executable string `json:"executable,omitempty"`

	// Command is the process's reported name (e.g. from /proc/<pid>/comm),
	// which may differ from Executable's basename.
	Command string `json:"command,omitempty"`
}

// Validate reports whether p is a well-formed Process reference. A nil
// *Process on an Event means "no process association is known," which is
// valid; Validate is only called on a non-nil Process.
func (p Process) Validate() error {
	if p.PID <= 0 {
		return fmt.Errorf("%w: process.pid must be positive, got %d", ErrInvalidField, p.PID)
	}
	return nil
}
