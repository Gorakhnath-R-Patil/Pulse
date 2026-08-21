package model

import "fmt"

// Container identifies the container — and, on Kubernetes, the pod — a
// process is running in. It's resolved from cgroup membership alone
// (see internal/discovery), which needs no container runtime or
// Kubernetes API access, so it's available on any Linux host today.
//
// Namespace, the pod's friendly name, and a service name are
// deliberately not fields here yet: cgroup membership on a Kubernetes
// host reveals the pod's UID, not its human-readable name or namespace
// — getting those requires the Kubernetes API, which is Day 19's job.
// See docs/design/service-identity.md.
type Container struct {
	// ID is the container runtime's ID for this container (Docker,
	// containerd, CRI-O, ...).
	ID string `json:"id"`

	// PodUID is the Kubernetes pod UID this container belongs to, if
	// the cgroup path indicates a Kubernetes-managed cgroup. Empty on
	// non-Kubernetes hosts.
	PodUID string `json:"pod_uid,omitempty"`
}

// Validate reports whether c is a well-formed Container reference. A
// nil *Container on a Process means "not running in a recognized
// container," which is valid; Validate is only called on a non-nil
// Container.
func (c Container) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("%w: container.id", ErrMissingField)
	}
	return nil
}
