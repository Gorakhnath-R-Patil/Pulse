package ebpf_test

import (
	"errors"
	"testing"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/ebpf"
)

func TestDecodeHeartbeatEvent(t *testing.T) {
	// Bytes as they'd arrive from the kernel: little-endian uint64
	// timestamp_ns (0x0102030405060708), then little-endian uint64
	// sequence (42).
	raw := []byte{
		0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01,
		0x2a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}

	event, err := ebpf.DecodeHeartbeatEvent(raw)
	if err != nil {
		t.Fatalf("DecodeHeartbeatEvent() returned error: %v", err)
	}
	if event.TimestampNS != 0x0102030405060708 {
		t.Errorf("TimestampNS = %#x, want %#x", event.TimestampNS, uint64(0x0102030405060708))
	}
	if event.Sequence != 42 {
		t.Errorf("Sequence = %d, want 42", event.Sequence)
	}
}

func TestDecodeHeartbeatEvent_ExtraTrailingBytesIgnored(t *testing.T) {
	raw := make([]byte, 32) // ring buffer records can be padded
	raw[8] = 7              // sequence = 7

	event, err := ebpf.DecodeHeartbeatEvent(raw)
	if err != nil {
		t.Fatalf("DecodeHeartbeatEvent() returned error: %v", err)
	}
	if event.Sequence != 7 {
		t.Errorf("Sequence = %d, want 7", event.Sequence)
	}
}

func TestDecodeHeartbeatEvent_TooShort(t *testing.T) {
	_, err := ebpf.DecodeHeartbeatEvent(make([]byte, 15))
	if !errors.Is(err, ebpf.ErrShortRead) {
		t.Fatalf("DecodeHeartbeatEvent() error = %v, want it to wrap ErrShortRead", err)
	}
}

func TestDecodeHeartbeatEvent_Empty(t *testing.T) {
	_, err := ebpf.DecodeHeartbeatEvent(nil)
	if !errors.Is(err, ebpf.ErrShortRead) {
		t.Fatalf("DecodeHeartbeatEvent(nil) error = %v, want it to wrap ErrShortRead", err)
	}
}
