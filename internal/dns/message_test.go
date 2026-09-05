package dns

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

// buildQuery constructs a real, wire-format DNS query message: a
// 12-byte header (QR=0, RD=1, one question, no other records) followed
// by name's labels, qtype, and QCLASS=IN. Used throughout this file (and
// benchmark_test.go) instead of hand-typed byte literals, since this is
// both more readable and, being itself a DNS encoder, doubly-checks the
// wire format parseMessage decodes. Takes no *testing.T: it never fails
// on its own, so there's nothing for it to report.
func buildQuery(id uint16, name string, qtype uint16) []byte {
	var buf bytes.Buffer
	writeUint16(&buf, id)
	buf.Write([]byte{0x01, 0x00}) // flags: QR=0 (query), RD=1
	writeUint16(&buf, 1)          // QDCOUNT
	writeUint16(&buf, 0)          // ANCOUNT
	writeUint16(&buf, 0)          // NSCOUNT
	writeUint16(&buf, 0)          // ARCOUNT
	writeName(&buf, name)
	writeUint16(&buf, qtype)
	writeUint16(&buf, 1) // QCLASS = IN
	return buf.Bytes()
}

// buildResponse constructs a real, wire-format DNS response to a query
// built with the same id/name/qtype, with the given response code and
// answer count.
func buildResponse(id uint16, name string, qtype uint16, rcode uint8, answerCount uint16) []byte {
	var buf bytes.Buffer
	writeUint16(&buf, id)
	buf.Write([]byte{0x81, 0x80 | rcode}) // flags: QR=1, RD=1, RA=1, RCODE=rcode
	writeUint16(&buf, 1)                  // QDCOUNT
	writeUint16(&buf, answerCount)
	writeUint16(&buf, 0) // NSCOUNT
	writeUint16(&buf, 0) // ARCOUNT
	writeName(&buf, name)
	writeUint16(&buf, qtype)
	writeUint16(&buf, 1) // QCLASS = IN
	return buf.Bytes()
}

func writeUint16(buf *bytes.Buffer, v uint16) {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	buf.Write(b[:])
}

func writeName(buf *bytes.Buffer, name string) {
	for _, label := range strings.Split(name, ".") {
		buf.WriteByte(byte(len(label)))
		buf.WriteString(label)
	}
	buf.WriteByte(0)
}

func TestParseMessage_Query(t *testing.T) {
	data := buildQuery(0x1234, "example.com", 1)

	got, err := parseMessage(data)
	if err != nil {
		t.Fatalf("parseMessage() returned error: %v", err)
	}
	if got.id != 0x1234 {
		t.Errorf("id = %#x, want %#x", got.id, 0x1234)
	}
	if got.isResponse {
		t.Error("isResponse = true, want false for a query")
	}
	if got.name != "example.com" {
		t.Errorf("name = %q, want %q", got.name, "example.com")
	}
	if got.qtype != 1 {
		t.Errorf("qtype = %d, want 1 (A)", got.qtype)
	}
}

func TestParseMessage_Response(t *testing.T) {
	data := buildResponse(0x1234, "example.com", 1, 0, 2)

	got, err := parseMessage(data)
	if err != nil {
		t.Fatalf("parseMessage() returned error: %v", err)
	}
	if !got.isResponse {
		t.Error("isResponse = false, want true for a response")
	}
	if got.responseCode != 0 {
		t.Errorf("responseCode = %d, want 0 (NOERROR)", got.responseCode)
	}
	if got.answerCount != 2 {
		t.Errorf("answerCount = %d, want 2", got.answerCount)
	}
}

func TestParseMessage_NXDOMAIN(t *testing.T) {
	data := buildResponse(1, "nonexistent.example", 1, 3, 0)

	got, err := parseMessage(data)
	if err != nil {
		t.Fatalf("parseMessage() returned error: %v", err)
	}
	if got.responseCode != 3 {
		t.Errorf("responseCode = %d, want 3 (NXDOMAIN)", got.responseCode)
	}
	if got.answerCount != 0 {
		t.Errorf("answerCount = %d, want 0", got.answerCount)
	}
}

func TestParseMessage_TooShort(t *testing.T) {
	_, err := parseMessage(make([]byte, headerLen-1))
	if !errors.Is(err, ErrShortMessage) {
		t.Fatalf("parseMessage() error = %v, want it to wrap ErrShortMessage", err)
	}
}

func TestParseMessage_NoQuestion(t *testing.T) {
	header := make([]byte, headerLen) // QDCOUNT (bytes 4:6) left at zero
	_, err := parseMessage(header)
	if !errors.Is(err, ErrNoQuestion) {
		t.Fatalf("parseMessage() error = %v, want it to wrap ErrNoQuestion", err)
	}
}

func TestParseMessage_CompressedNameUnsupported(t *testing.T) {
	data := buildQuery(1, "example.com", 1)
	// Overwrite the name's first label-length byte with a compression
	// pointer marker (top two bits set).
	data[headerLen] = 0xC0

	_, err := parseMessage(data)
	if !errors.Is(err, ErrUnsupportedName) {
		t.Fatalf("parseMessage() error = %v, want it to wrap ErrUnsupportedName", err)
	}
}

func TestParseMessage_TruncatedLabel(t *testing.T) {
	data := buildQuery(1, "example.com", 1)
	truncated := data[:headerLen+3] // claims a 7-byte "example" label but cuts it off after 2 bytes

	_, err := parseMessage(truncated)
	if !errors.Is(err, ErrShortMessage) {
		t.Fatalf("parseMessage() error = %v, want it to wrap ErrShortMessage", err)
	}
}

func TestParseMessage_TruncatedBeforeQType(t *testing.T) {
	data := buildQuery(1, "example.com", 1)
	nameEnd := headerLen + len("example") + 1 + len("com") + 1 + 1 // labels + length bytes + terminator
	truncated := data[:nameEnd+1]                                  // one byte into QTYPE, not enough for it

	_, err := parseMessage(truncated)
	if !errors.Is(err, ErrShortMessage) {
		t.Fatalf("parseMessage() error = %v, want it to wrap ErrShortMessage", err)
	}
}
