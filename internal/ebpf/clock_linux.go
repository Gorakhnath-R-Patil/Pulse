//go:build linux

package ebpf

import (
	"time"

	"golang.org/x/sys/unix"
)

// MonotonicReference returns the current CLOCK_MONOTONIC time in
// nanoseconds — matching the kernel's bpf_ktime_get_ns(), which every
// program under bpf/programs/ stamps its events with — paired with the
// corresponding wall-clock time.
//
// bpf_ktime_get_ns() reports time since boot (CLOCK_MONOTONIC), not
// since the Unix epoch, so a raw reading can't be turned into a
// time.Time on its own. Callers capture one reference pair (typically
// at Load time) and convert each later event's raw monotonic timestamp
// to wall-clock time by adding its delta from the reference's
// monotonic side onto the reference's wall-clock side — see
// internal/process and internal/network's Loader.Read for the pattern.
func MonotonicReference() (monotonicNS uint64, wallClock time.Time) {
	var ts unix.Timespec
	wallClock = time.Now()
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts); err != nil {
		// clock_gettime with a valid, always-present clock ID does not
		// fail in practice. Falling back to a zero monotonic reference
		// (rather than propagating an error callers would have to
		// handle) means every event's Timestamp would be off by
		// however long this process had been up at Load time — a
		// degraded but harmless result, not a crash.
		return 0, wallClock
	}
	return uint64(ts.Sec)*1e9 + uint64(ts.Nsec), wallClock
}
