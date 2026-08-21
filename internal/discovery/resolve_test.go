package discovery_test

import (
	"testing"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/discovery"
)

func TestResolveContainer_NoSuchProcess(t *testing.T) {
	// PID 0 is never a real userspace process (it's the kernel
	// scheduler/swapper on Linux); /proc/0 doesn't exist there, and
	// /proc doesn't exist at all on non-Linux platforms. Either way
	// this must fail with an error, not panic or return a fabricated
	// result.
	_, err := discovery.ResolveContainer(0)
	if err == nil {
		t.Fatal("ResolveContainer(0) returned nil error, want an error")
	}
}
