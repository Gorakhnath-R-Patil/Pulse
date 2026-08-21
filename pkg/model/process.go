package model

import "fmt"

// Process identifies the process associated with an observed activity.
// Service identity (namespace, pod name, service name — the parts of
// Day 6's original deferred field group that need the Kubernetes API,
// not just cgroup membership) is not here yet; see Container's doc
// comment and docs/design/service-identity.md.
type Process struct {
	// PID is the process ID as seen by the kernel that observed it.
	PID int32 `json:"pid"`

	// Executable is the resolved path to the running binary, if known
	// (e.g. "/usr/bin/nginx").
	Executable string `json:"executable,omitempty"`

	// Command is the process's reported name (e.g. from /proc/<pid>/comm),
	// which may differ from Executable's basename.
	Command string `json:"command,omitempty"`

	// Container identifies the container this process runs in, if any.
	Container *Container `json:"container,omitempty"`
}

// Validate reports whether p is a well-formed Process reference. A nil
// *Process on an Event means "no process association is known," which is
// valid; Validate is only called on a non-nil Process.
func (p Process) Validate() error {
	if p.PID <= 0 {
		return fmt.Errorf("%w: process.pid must be positive, got %d", ErrInvalidField, p.PID)
	}
	if p.Container != nil {
		if err := p.Container.Validate(); err != nil {
			return err
		}
	}
	return nil
}
