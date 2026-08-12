package process

import (
	"encoding/binary"
	"errors"
	"testing"
)

func buildRawEventBytes(timestampNS uint64, pid, ppid uint32, comm string, eventType EventType, padding int) []byte {
	raw := make([]byte, minRawEventSize+padding)
	binary.LittleEndian.PutUint64(raw[0:8], timestampNS)
	binary.LittleEndian.PutUint32(raw[8:12], pid)
	binary.LittleEndian.PutUint32(raw[12:16], ppid)
	copy(raw[16:32], comm) // comm shorter than 16 bytes leaves the rest zero, matching the kernel's null-padded comm[]
	raw[32] = byte(eventType)
	return raw
}

func TestDecodeRawEvent(t *testing.T) {
	raw := buildRawEventBytes(0x0102030405060708, 1234, 1, "bash", EventStart, 7) // +7 = compiler's likely 8-byte struct padding

	got, err := decodeRawEvent(raw)
	if err != nil {
		t.Fatalf("decodeRawEvent() returned error: %v", err)
	}

	want := rawEvent{
		TimestampNS: 0x0102030405060708,
		PID:         1234,
		PPID:        1,
		Comm:        "bash",
		Type:        EventStart,
	}
	if got != want {
		t.Errorf("decodeRawEvent() = %+v, want %+v", got, want)
	}
}

func TestDecodeRawEvent_CommFillsAllSixteenBytes(t *testing.T) {
	// A 16-character comm has no trailing NUL for TrimRight to remove;
	// it must still decode intact rather than losing its last byte.
	raw := buildRawEventBytes(1, 1, 0, "0123456789abcdef", EventExit, 0)

	got, err := decodeRawEvent(raw)
	if err != nil {
		t.Fatalf("decodeRawEvent() returned error: %v", err)
	}
	if got.Comm != "0123456789abcdef" {
		t.Errorf("Comm = %q, want %q", got.Comm, "0123456789abcdef")
	}
}

func TestDecodeRawEvent_TooShort(t *testing.T) {
	_, err := decodeRawEvent(make([]byte, minRawEventSize-1))
	if !errors.Is(err, ErrShortRead) {
		t.Fatalf("decodeRawEvent() error = %v, want it to wrap ErrShortRead", err)
	}
}

func TestEventType_String(t *testing.T) {
	tests := []struct {
		typ  EventType
		want string
	}{
		{EventStart, "start"},
		{EventExit, "exit"},
		{EventType(42), "unknown(42)"},
	}
	for _, tt := range tests {
		if got := tt.typ.String(); got != tt.want {
			t.Errorf("EventType(%d).String() = %q, want %q", tt.typ, got, tt.want)
		}
	}
}
