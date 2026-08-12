package process

import (
	"fmt"
	"os"
)

// ResolveExecutable best-effort resolves the path to pid's running
// binary via /proc/<pid>/exe.
//
// It is only reliable when called for a process.start event, observed
// while the process is still alive. By the time a process.exit event is
// processed, the process is already gone — this will almost always fail
// (or, worse, silently resolve to whatever unrelated process has since
// reused that PID), so callers should not call it for exit events, and
// should treat a non-nil error here as "unknown," not as a real failure
// to surface or retry.
func ResolveExecutable(pid uint32) (string, error) {
	path, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return "", fmt.Errorf("process: resolving executable for pid %d: %w", pid, err)
	}
	return path, nil
}
