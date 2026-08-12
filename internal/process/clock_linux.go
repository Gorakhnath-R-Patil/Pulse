//go:build linux

package process

import (
	"time"

	"golang.org/x/sys/unix"
)

// monotonicReference returns the current CLOCK_MONOTONIC time in
// nanoseconds — matching the kernel's bpf_ktime_get_ns(), which
// process.c stamps every event with — paired with the corresponding
// wall-clock time. ProcessEvent.Timestamp values are computed by adding
// a later event's monotonic delta from this reference onto wallClock;
// see loader_linux.go's Read.
//
// bpf_ktime_get_ns() reports time since boot (CLOCK_MONOTONIC), not
// since the Unix epoch, so a raw reading can't be turned into a
// time.Time on its own — this reference pair is what makes that
// conversion possible.
func monotonicReference() (monotonicNS uint64, wallClock time.Time) {
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
