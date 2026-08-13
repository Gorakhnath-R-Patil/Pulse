package network

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net/netip"
	"time"
)

// ConnectEvent is a decoded, wall-clock-timestamped TCP connect
// attempt, ready to be passed to ToEvent.
type ConnectEvent struct {
	Timestamp  time.Time
	PID        uint32
	SourceAddr netip.Addr
	DestAddr   netip.Addr
	SourcePort uint16
	DestPort   uint16
	Comm       string
	Success    bool
}

// rawEvent is ConnectEvent before its kernel CLOCK_MONOTONIC timestamp
// has been converted to a real time.Time — see internal/process's
// identical rationale in its own rawEvent/loader_linux.go for why that
// conversion needs a reference pair captured at Load time.
type rawEvent struct {
	TimestampNS uint64
	PID         uint32
	SourceAddr  netip.Addr
	DestAddr    netip.Addr
	SourcePort  uint16
	DestPort    uint16
	Comm        string
	Success     bool
}

// minRawEventSize is the minimum wire size needed to decode a rawEvent:
// 8 (timestamp_ns) + 4 (pid) + 4 (saddr) + 4 (daddr) + 2 (sport) +
// 2 (dport) + 16 (comm) + 1 (success), matching struct connect_event's
// field layout in tcp_connect.c. The compiler may pad the C struct's
// total size up to an 8-byte boundary; decodeRawEvent reads fields at
// their fixed offsets and ignores any such trailing padding.
const minRawEventSize = 41

// decodeRawEvent parses the wire bytes bpf/programs/tcp_connect.c
// writes to its ring buffer.
//
// Byte order here is genuinely mixed, matching real kernel behavior in
// struct sock_common, not a mistake:
//   - timestamp_ns, pid: written by bpf_ktime_get_ns()/
//     bpf_get_current_pid_tgid() in the target's native order
//     (little-endian on every architecture this project builds for).
//   - saddr, daddr: __be32 in the kernel — already network byte order,
//     which is the same left-to-right octet order netip.AddrFrom4
//     expects. No conversion; read as raw bytes.
//   - sport (skc_num): __u16, but the kernel stores the *local* port in
//     host-native order (it's compared directly in hot paths) —
//     little-endian here.
//   - dport (skc_dport): __be16 — network byte order, i.e. big-endian.
func decodeRawEvent(raw []byte) (rawEvent, error) {
	if len(raw) < minRawEventSize {
		return rawEvent{}, fmt.Errorf("%w: got %d bytes, want at least %d", ErrShortRead, len(raw), minRawEventSize)
	}

	return rawEvent{
		TimestampNS: binary.LittleEndian.Uint64(raw[0:8]),
		PID:         binary.LittleEndian.Uint32(raw[8:12]),
		SourceAddr:  netip.AddrFrom4([4]byte(raw[12:16])),
		DestAddr:    netip.AddrFrom4([4]byte(raw[16:20])),
		SourcePort:  binary.LittleEndian.Uint16(raw[20:22]),
		DestPort:    binary.BigEndian.Uint16(raw[22:24]),
		Comm:        string(bytes.TrimRight(raw[24:40], "\x00")),
		Success:     raw[40] != 0,
	}, nil
}
