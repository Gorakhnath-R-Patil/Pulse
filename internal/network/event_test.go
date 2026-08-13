package network

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"
)

// buildRawEventBytes lays out bytes exactly as tcp_connect.c's struct
// connect_event does, with the same per-field byte order documented on
// decodeRawEvent: little-endian timestamp/pid/sport, raw address bytes,
// big-endian dport.
func buildRawEventBytes(timestampNS uint64, pid uint32, saddr, daddr [4]byte, sport, dport uint16, comm string, success bool, padding int) []byte {
	raw := make([]byte, minRawEventSize+padding)
	binary.LittleEndian.PutUint64(raw[0:8], timestampNS)
	binary.LittleEndian.PutUint32(raw[8:12], pid)
	copy(raw[12:16], saddr[:])
	copy(raw[16:20], daddr[:])
	binary.LittleEndian.PutUint16(raw[20:22], sport) // skc_num: host byte order
	binary.BigEndian.PutUint16(raw[22:24], dport)    // skc_dport: network byte order
	copy(raw[24:40], comm)
	if success {
		raw[40] = 1
	}
	return raw
}

func TestDecodeRawEvent(t *testing.T) {
	saddr := [4]byte{10, 0, 0, 5}
	daddr := [4]byte{93, 184, 216, 34} // example.com, for a recognizable non-loopback address
	raw := buildRawEventBytes(0x0102030405060708, 4242, saddr, daddr, 51000, 443, "curl", true, 7)

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
	if got.DestAddr != netip.AddrFrom4(daddr) {
		t.Errorf("DestAddr = %v, want %v", got.DestAddr, netip.AddrFrom4(daddr))
	}
	if got.DestAddr.String() != "93.184.216.34" {
		t.Errorf("DestAddr.String() = %q, want %q (sanity-checking octet order, not just round-tripping)", got.DestAddr.String(), "93.184.216.34")
	}
	if got.SourcePort != 51000 {
		t.Errorf("SourcePort = %d, want 51000", got.SourcePort)
	}
	if got.DestPort != 443 {
		t.Errorf("DestPort = %d, want 443", got.DestPort)
	}
	if got.Comm != "curl" {
		t.Errorf("Comm = %q, want %q", got.Comm, "curl")
	}
	if !got.Success {
		t.Error("Success = false, want true")
	}
}

func TestDecodeRawEvent_FailedConnect(t *testing.T) {
	raw := buildRawEventBytes(1, 1, [4]byte{127, 0, 0, 1}, [4]byte{127, 0, 0, 1}, 1234, 9999, "nc", false, 0)

	got, err := decodeRawEvent(raw)
	if err != nil {
		t.Fatalf("decodeRawEvent() returned error: %v", err)
	}
	if got.Success {
		t.Error("Success = true, want false")
	}
}

func TestDecodeRawEvent_TooShort(t *testing.T) {
	_, err := decodeRawEvent(make([]byte, minRawEventSize-1))
	if !errors.Is(err, ErrShortRead) {
		t.Fatalf("decodeRawEvent() error = %v, want it to wrap ErrShortRead", err)
	}
}
