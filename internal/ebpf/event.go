package ebpf

import (
	"encoding/binary"
	"fmt"
)

// HeartbeatEvent is the decoded form of the raw bytes
// bpf/programs/foundation.c writes to its ring buffer. It carries no
// process, network, or other telemetry semantics — see that file's
// header comment for why — only what's needed to prove an event made it
// from the kernel to userspace intact.
type HeartbeatEvent struct {
	// TimestampNS is CLOCK_MONOTONIC nanoseconds at observation time,
	// as returned by the kernel's bpf_ktime_get_ns().
	TimestampNS uint64

	// Sequence is a counter incremented once per event, assigned inside
	// the kernel program. It resets to 0 every time the program is
	// loaded and is not meaningful across restarts.
	Sequence uint64
}

// heartbeatEventSize is the wire size of HeartbeatEvent: two uint64
// fields, matching struct heartbeat_event in foundation.c exactly (both
// fields are 8-byte aligned already, so there's no compiler padding to
// account for).
const heartbeatEventSize = 16

// DecodeHeartbeatEvent parses a HeartbeatEvent from the raw bytes a ring
// buffer read returns (ringbuf.Record.RawSample). The C struct uses the
// target's native byte order; every architecture this project builds
// for (amd64, arm64) is little-endian.
func DecodeHeartbeatEvent(raw []byte) (HeartbeatEvent, error) {
	if len(raw) < heartbeatEventSize {
		return HeartbeatEvent{}, fmt.Errorf("%w: got %d bytes, want at least %d", ErrShortRead, len(raw), heartbeatEventSize)
	}
	return HeartbeatEvent{
		TimestampNS: binary.LittleEndian.Uint64(raw[0:8]),
		Sequence:    binary.LittleEndian.Uint64(raw[8:16]),
	}, nil
}
