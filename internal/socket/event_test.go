package socket

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"
)

// buildRawEventBytes lays out bytes exactly as tcp_close.c's struct
// close_event does, with the same per-field byte order documented on
// decodeRawEvent.
func buildRawEventBytes(timestampNS uint64, pid uint32, saddr, daddr [4]byte, sport, dport uint16, bytesSent, bytesReceived uint64, comm string, sockErr uint32, padding int) []byte {
	raw := make([]byte, minRawEventSize+padding)
	binary.LittleEndian.PutUint64(raw[0:8], timestampNS)
	binary.LittleEndian.PutUint32(raw[8:12], pid)
	copy(raw[12:16], saddr[:])
	copy(raw[16:20], daddr[:])
	binary.LittleEndian.PutUint16(raw[20:22], sport)
	binary.BigEndian.PutUint16(raw[22:24], dport)
	binary.LittleEndian.PutUint64(raw[24:32], bytesSent)
	binary.LittleEndian.PutUint64(raw[32:40], bytesReceived)
	copy(raw[40:56], comm)
	binary.LittleEndian.PutUint32(raw[56:60], sockErr)
	return raw
}

func TestDecodeRawEvent(t *testing.T) {
	saddr := [4]byte{10, 0, 0, 5}
	daddr := [4]byte{93, 184, 216, 34}
	raw := buildRawEventBytes(0x0102030405060708, 4242, saddr, daddr, 51000, 443, 1024, 8192, "curl", 0, 4)

	got, err := decodeRawEvent(raw)
	if err != nil {
		t.Fatalf("decodeRawEvent() returned error: %v", err)
	}

	if got.TimestampNS != 0x0102030405060708 {
		t.Errorf("TimestampNS = %#x, want %#x", got.TimestampNS, uint64(0x0102030405060708))
	}
	if got.PID != 4242 {
		t.Errorf("PID = %d, want 4242", got.PID)
	}
	if got.SourceAddr != netip.AddrFrom4(saddr) {
		t.Errorf("SourceAddr = %v, want %v", got.SourceAddr, netip.AddrFrom4(saddr))
	}
	if got.DestAddr.String() != "93.184.216.34" {
		t.Errorf("DestAddr.String() = %q, want %q", got.DestAddr.String(), "93.184.216.34")
	}
	if got.SourcePort != 51000 {
		t.Errorf("SourcePort = %d, want 51000", got.SourcePort)
	}
	if got.DestPort != 443 {
		t.Errorf("DestPort = %d, want 443", got.DestPort)
	}
	if got.BytesSent != 1024 {
		t.Errorf("BytesSent = %d, want 1024", got.BytesSent)
	}
	if got.BytesReceived != 8192 {
		t.Errorf("BytesReceived = %d, want 8192", got.BytesReceived)
	}
	if got.Comm != "curl" {
		t.Errorf("Comm = %q, want %q", got.Comm, "curl")
	}
	if got.SockError != 0 {
		t.Errorf("SockError = %d, want 0", got.SockError)
	}
}

func TestDecodeRawEvent_WithSockError(t *testing.T) {
	const ECONNRESET = 104
	raw := buildRawEventBytes(1, 1, [4]byte{127, 0, 0, 1}, [4]byte{127, 0, 0, 1}, 1, 2, 0, 0, "nc", ECONNRESET, 0)

	got, err := decodeRawEvent(raw)
	if err != nil {
		t.Fatalf("decodeRawEvent() returned error: %v", err)
	}
	if got.SockError != ECONNRESET {
		t.Errorf("SockError = %d, want %d", got.SockError, ECONNRESET)
	}
}

func TestDecodeRawEvent_TooShort(t *testing.T) {
	_, err := decodeRawEvent(make([]byte, minRawEventSize-1))
	if !errors.Is(err, ErrShortRead) {
		t.Fatalf("decodeRawEvent() error = %v, want it to wrap ErrShortRead", err)
	}
}
