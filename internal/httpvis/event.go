package httpvis

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"
)

// HTTPEvent is a decoded, wall-clock-timestamped, already-parsed HTTP
// request or response line. The raw bytes it was parsed from are never
// retained here or anywhere upstream of it — see ToEvent and
// docs/design/http-visibility.md's Security section.
type HTTPEvent struct {
	Timestamp  time.Time
	PID        uint32
	Comm       string
	Size       uint32 // bytes passed to the write() call this was observed on
	IsResponse bool

	// Method and Path are set for a request line, empty for a response.
	Method string
	Path   string

	// Status is set for a response line (e.g. "200"), empty for a
	// request.
	Status string
}

// rawEvent is the decoded wire form of struct http_event, before its
// data prefix has been parsed into a request or status line (see
// parseHTTPLine) and before its kernel CLOCK_MONOTONIC timestamp has
// been converted to wall-clock time (see internal/ebpf.MonotonicReference).
type rawEvent struct {
	TimestampNS uint64
	PID         uint32
	Size        uint32
	Comm        string
	Data        []byte // exactly the meaningfully-captured prefix; never longer
}

// headerSize is struct http_event's fixed header, before the variable-
// meaningful data[] prefix: 8 (timestamp_ns) + 4 (pid) + 4 (size) +
// 2 (captured_len) + 16 (comm).
const headerSize = 34

func decodeRawEvent(raw []byte) (rawEvent, error) {
	if len(raw) < headerSize {
		return rawEvent{}, fmt.Errorf("%w: got %d bytes, want at least %d", ErrShortRead, len(raw), headerSize)
	}

	capturedLen := binary.LittleEndian.Uint16(raw[16:18])
	dataEnd := headerSize + int(capturedLen)
	if dataEnd > len(raw) {
		return rawEvent{}, fmt.Errorf("%w: captured_len %d exceeds the %d bytes available", ErrShortRead, capturedLen, len(raw)-headerSize)
	}

	data := make([]byte, capturedLen)
	copy(data, raw[headerSize:dataEnd])

	return rawEvent{
		TimestampNS: binary.LittleEndian.Uint64(raw[0:8]),
		PID:         binary.LittleEndian.Uint32(raw[8:12]),
		Size:        binary.LittleEndian.Uint32(raw[12:16]),
		Comm:        string(bytes.TrimRight(raw[18:34], "\x00")),
		Data:        data,
	}, nil
}

// parseHTTPLine parses the first line of a captured HTTP request or
// response prefix.
//
// ok is false if data doesn't start with a complete, recognizable HTTP
// request or status line — e.g. the line was longer than
// http_visibility.c's capture bound, or the kernel-side classify_prefix
// check was a false positive on data that only coincidentally started
// with a method name. Callers should treat ok == false as "discard this
// one, not every write() call parses," not as an error.
func parseHTTPLine(data []byte) (method, path, status string, isResponse, ok bool) {
	line := data
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		line = data[:i]
	}
	line = bytes.TrimRight(line, "\r")

	fields := bytes.Fields(line)
	if len(fields) < 2 {
		return "", "", "", false, false
	}

	if bytes.HasPrefix(fields[0], []byte("HTTP/")) {
		// Status line: "HTTP/1.1 200 OK"
		return "", "", string(fields[1]), true, true
	}

	// Request line: "METHOD PATH HTTP/1.1"
	if len(fields) < 3 || !bytes.HasPrefix(fields[2], []byte("HTTP/")) {
		return "", "", "", false, false
	}
	return string(fields[0]), string(fields[1]), "", false, true
}
