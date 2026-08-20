package socket

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net/netip"
	"time"
)

// CloseEvent is a decoded, wall-clock-timestamped TCP connection close,
// ready to be passed to ToEvent.
type CloseEvent struct {
	Timestamp     time.Time
	PID           uint32
	SourceAddr    netip.Addr
	DestAddr      netip.Addr
	SourcePort    uint16
	DestPort      uint16
	BytesSent     uint64
	BytesReceived uint64
	Comm          string
	// SockError is the socket's pending error at close, if any (a
	// positive errno value, e.g. ECONNRESET), or 0 for none.
	SockError uint32
}

// rawEvent is CloseEvent before its kernel CLOCK_MONOTONIC timestamp has
// been converted to a real time.Time — see internal/network's identical
// rationale in its own rawEvent for why.
type rawEvent struct {
	TimestampNS   uint64
	PID           uint32
	SourceAddr    netip.Addr
	DestAddr      netip.Addr
	SourcePort    uint16
	DestPort      uint16
	BytesSent     uint64
	BytesReceived uint64
	Comm          string
	SockError     uint32
}

// minRawEventSize is the minimum wire size needed to decode a rawEvent:
// 8 (timestamp_ns) + 4 (pid) + 4 (saddr) + 4 (daddr) + 2 (sport) +
// 2 (dport) + 8 (bytes_sent) + 8 (bytes_received) + 16 (comm) +
// 4 (sk_err), matching struct close_event's field layout in
// tcp_close.c. The compiler may pad the C struct's total size up to an
// 8-byte boundary; decodeRawEvent reads fields at their fixed offsets
// and ignores any such trailing padding.
const minRawEventSize = 60

// decodeRawEvent parses the wire bytes bpf/programs/tcp_close.c writes
// to its ring buffer. Byte order follows the same rules as
// internal/network's decodeRawEvent (see its doc comment for the full
// reasoning): little-endian for kernel-native values, raw bytes for
// addresses, host order for the local port, network order for the
// remote port.
func decodeRawEvent(raw []byte) (rawEvent, error) {
	if len(raw) < minRawEventSize {
		return rawEvent{}, fmt.Errorf("%w: got %d bytes, want at least %d", ErrShortRead, len(raw), minRawEventSize)
	}

	return rawEvent{
		TimestampNS:   binary.LittleEndian.Uint64(raw[0:8]),
		PID:           binary.LittleEndian.Uint32(raw[8:12]),
		SourceAddr:    netip.AddrFrom4([4]byte(raw[12:16])),
		DestAddr:      netip.AddrFrom4([4]byte(raw[16:20])),
		SourcePort:    binary.LittleEndian.Uint16(raw[20:22]),
		DestPort:      binary.BigEndian.Uint16(raw[22:24]),
		BytesSent:     binary.LittleEndian.Uint64(raw[24:32]),
		BytesReceived: binary.LittleEndian.Uint64(raw[32:40]),
		Comm:          string(bytes.TrimRight(raw[40:56], "\x00")),
		SockError:     binary.LittleEndian.Uint32(raw[56:60]),
	}, nil
}
