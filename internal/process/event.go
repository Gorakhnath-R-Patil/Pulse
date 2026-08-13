package process

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"
)

// EventType distinguishes the two process lifecycle events
// bpf/programs/process.c reports.
type EventType uint8

const (
	EventStart EventType = 0
	EventExit  EventType = 1
)

func (t EventType) String() string {
	switch t {
	case EventStart:
		return "start"
	case EventExit:
		return "exit"
	default:
		return fmt.Sprintf("unknown(%d)", uint8(t))
	}
}

// ProcessEvent is a decoded, wall-clock-timestamped process lifecycle
// event, ready to be passed to ToEvent.
type ProcessEvent struct {
	Timestamp time.Time
	PID       uint32
	PPID      uint32
	Comm      string
	Type      EventType
}

// rawEvent is ProcessEvent before its kernel CLOCK_MONOTONIC timestamp
// (not wall-clock time) has been converted to a real time.Time — see
// internal/ebpf's MonotonicReference for why that conversion needs a
// reference pair captured at Load time, and so can't happen in the pure
// decode step below.
type rawEvent struct {
	TimestampNS uint64
	PID         uint32
	PPID        uint32
	Comm        string
	Type        EventType
}

// minRawEventSize is the minimum wire size needed to decode a rawEvent:
// 8 (timestamp_ns) + 4 (pid) + 4 (ppid) + 16 (comm) + 1 (event_type),
// matching struct process_event's field layout in process.c. The
// compiler may pad the C struct's total size up to an 8-byte boundary;
// decodeRawEvent reads fields at their fixed offsets and ignores any
// such trailing padding rather than asserting an exact length.
const minRawEventSize = 33

func decodeRawEvent(raw []byte) (rawEvent, error) {
	if len(raw) < minRawEventSize {
		return rawEvent{}, fmt.Errorf("%w: got %d bytes, want at least %d", ErrShortRead, len(raw), minRawEventSize)
	}

	return rawEvent{
		TimestampNS: binary.LittleEndian.Uint64(raw[0:8]),
		PID:         binary.LittleEndian.Uint32(raw[8:12]),
		PPID:        binary.LittleEndian.Uint32(raw[12:16]),
		Comm:        string(bytes.TrimRight(raw[16:32], "\x00")),
		Type:        EventType(raw[32]),
	}, nil
}
