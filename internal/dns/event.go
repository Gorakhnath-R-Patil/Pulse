package dns

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"
)

// DNSEvent is a decoded, wall-clock-timestamped, already-parsed DNS
// query or response.
type DNSEvent struct {
	Timestamp  time.Time
	PID        uint32
	Comm       string
	IsResponse bool

	Name  string // the queried domain name
	QType uint16 // the query type (1=A, 28=AAAA, 5=CNAME, ...)

	// ResponseCode and AnswerCount are set for a response, zero for a
	// query.
	ResponseCode uint8
	AnswerCount  uint16

	// Latency is how long after the matching query this response
	// arrived, if one was found — zero (and meaningless; check
	// separately if it matters) when unset. Always zero for a query.
	Latency time.Duration
}

// rawEvent is the decoded wire form of struct dns_event, before its
// data prefix has been parsed as a DNS message (see parseMessage) and
// before its kernel CLOCK_MONOTONIC timestamp has been converted to
// wall-clock time (see internal/ebpf.MonotonicReference).
type rawEvent struct {
	TimestampNS uint64
	PID         uint32
	Comm        string
	Data        []byte // exactly the meaningfully-captured prefix; never longer
}

// headerSize is struct dns_event's fixed header, before the variable-
// meaningful data[] prefix: 8 (timestamp_ns) + 4 (pid) + 2 (size) +
// 2 (captured_len) + 16 (comm).
const headerSize = 32

func decodeRawEvent(raw []byte) (rawEvent, error) {
	if len(raw) < headerSize {
		return rawEvent{}, fmt.Errorf("%w: got %d bytes, want at least %d", ErrShortRead, len(raw), headerSize)
	}

	capturedLen := binary.LittleEndian.Uint16(raw[14:16])
	dataEnd := headerSize + int(capturedLen)
	if dataEnd > len(raw) {
		return rawEvent{}, fmt.Errorf("%w: captured_len %d exceeds the %d bytes available", ErrShortRead, capturedLen, len(raw)-headerSize)
	}

	data := make([]byte, capturedLen)
	copy(data, raw[headerSize:dataEnd])

	return rawEvent{
		TimestampNS: binary.LittleEndian.Uint64(raw[0:8]),
		PID:         binary.LittleEndian.Uint32(raw[8:12]),
		Comm:        string(bytes.TrimRight(raw[16:32], "\x00")),
		Data:        data,
	}, nil
}
