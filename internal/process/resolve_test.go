package process_test

import (
	"testing"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/process"
)

func TestResolveExecutable_NoSuchProcess(t *testing.T) {
	// PID 0 is never a real userspace process (it's the kernel
	// scheduler/swapper), and /proc/0 does not exist on Linux — on any
	// other OS, /proc doesn't exist at all. Either way this must fail,
	// not panic or hang, and it must say so via an error rather than
	// silently returning a plausible-looking empty success.
	_, err := process.ResolveExecutable(0)
	if err == nil {
		t.Fatal("ResolveExecutable(0) returned nil error, want an error")
	}
}
