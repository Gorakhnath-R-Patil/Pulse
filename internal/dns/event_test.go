package dns

import (
	"encoding/binary"
	"errors"
	"testing"
)

func buildRawEventBytes(timestampNS uint64, pid uint32, comm string, data []byte, padding int) []byte {
	raw := make([]byte, headerSize+len(data)+padding)
	binary.LittleEndian.PutUint64(raw[0:8], timestampNS)
	binary.LittleEndian.PutUint32(raw[8:12], pid)
	binary.LittleEndian.PutUint16(raw[12:14], uint16(len(data)))
	binary.LittleEndian.PutUint16(raw[14:16], uint16(len(data)))
	copy(raw[16:32], comm)
	copy(raw[32:], data)
	return raw
}

func TestDecodeRawEvent(t *testing.T) {
	data := []byte{0x12, 0x34, 0x01, 0x00, 0x00, 0x01}
	raw := buildRawEventBytes(0x0102030405060708, 4242, "systemd-resolve", data, 12)

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
	if got.Comm != "systemd-resolve" {
		t.Errorf("Comm = %q, want %q", got.Comm, "systemd-resolve")
	}
	if string(got.Data) != string(data) {
		t.Errorf("Data = %v, want %v", got.Data, data)
	}
}

func TestDecodeRawEvent_TooShort(t *testing.T) {
	_, err := decodeRawEvent(make([]byte, headerSize-1))
	if !errors.Is(err, ErrShortRead) {
		t.Fatalf("decodeRawEvent() error = %v, want it to wrap ErrShortRead", err)
	}
}

func TestDecodeRawEvent_CapturedLenExceedsAvailable(t *testing.T) {
	raw := make([]byte, headerSize)
	binary.LittleEndian.PutUint16(raw[14:16], 10)

	_, err := decodeRawEvent(raw)
	if !errors.Is(err, ErrShortRead) {
		t.Fatalf("decodeRawEvent() error = %v, want it to wrap ErrShortRead", err)
	}
}
