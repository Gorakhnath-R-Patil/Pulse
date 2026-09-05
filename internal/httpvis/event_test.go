package httpvis

import (
	"encoding/binary"
	"errors"
	"testing"
)

func buildRawEventBytes(timestampNS uint64, pid, size uint32, comm string, data []byte, padding int) []byte {
	raw := make([]byte, headerSize+len(data)+padding)
	binary.LittleEndian.PutUint64(raw[0:8], timestampNS)
	binary.LittleEndian.PutUint32(raw[8:12], pid)
	binary.LittleEndian.PutUint32(raw[12:16], size)
	binary.LittleEndian.PutUint16(raw[16:18], uint16(len(data)))
	copy(raw[18:34], comm)
	copy(raw[34:], data)
	return raw
}

func TestDecodeRawEvent(t *testing.T) {
	data := []byte("GET /foo HTTP/1.1\r\nHost: example.com\r\n\r\n")
	raw := buildRawEventBytes(0x0102030405060708, 4242, uint32(len(data)), "curl", data, 20 /* trailing unused capacity */)

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
	if got.Size != uint32(len(data)) {
		t.Errorf("Size = %d, want %d", got.Size, len(data))
	}
	if got.Comm != "curl" {
		t.Errorf("Comm = %q, want %q", got.Comm, "curl")
	}
	if string(got.Data) != string(data) {
		t.Errorf("Data = %q, want %q", got.Data, data)
	}
}

func TestDecodeRawEvent_TooShort(t *testing.T) {
	_, err := decodeRawEvent(make([]byte, headerSize-1))
	if !errors.Is(err, ErrShortRead) {
		t.Fatalf("decodeRawEvent() error = %v, want it to wrap ErrShortRead", err)
	}
}

func TestDecodeRawEvent_CapturedLenExceedsAvailable(t *testing.T) {
	raw := make([]byte, headerSize) // claims captured_len bytes follow, but none do
	binary.LittleEndian.PutUint16(raw[16:18], 10)

	_, err := decodeRawEvent(raw)
	if !errors.Is(err, ErrShortRead) {
		t.Fatalf("decodeRawEvent() error = %v, want it to wrap ErrShortRead", err)
	}
}

func TestParseHTTPLine_Request(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		wantMethod string
		wantPath   string
	}{
		{"GET", "GET /foo/bar?x=1 HTTP/1.1\r\nHost: example.com\r\n\r\n", "GET", "/foo/bar?x=1"},
		{"POST", "POST /submit HTTP/1.1\r\nContent-Length: 0\r\n\r\n", "POST", "/submit"},
		{"root path", "GET / HTTP/1.1\r\n\r\n", "GET", "/"},
		{"no trailing CRLF (capture cut off mid-header)", "PUT /x HTTP/1.1\r\nHost: exa", "PUT", "/x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method, path, status, isResponse, ok := parseHTTPLine([]byte(tt.data))
			if !ok {
				t.Fatalf("parseHTTPLine(%q) ok = false, want true", tt.data)
			}
			if isResponse {
				t.Error("isResponse = true, want false for a request line")
			}
			if method != tt.wantMethod {
				t.Errorf("method = %q, want %q", method, tt.wantMethod)
			}
			if path != tt.wantPath {
				t.Errorf("path = %q, want %q", path, tt.wantPath)
			}
			if status != "" {
				t.Errorf("status = %q, want empty for a request line", status)
			}
		})
	}
}

func TestParseHTTPLine_Response(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		wantStatus string
	}{
		{"200", "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\n", "200"},
		{"404", "HTTP/1.1 404 Not Found\r\n\r\n", "404"},
		{"500 no reason phrase", "HTTP/1.1 500\r\n\r\n", "500"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method, path, status, isResponse, ok := parseHTTPLine([]byte(tt.data))
			if !ok {
				t.Fatalf("parseHTTPLine(%q) ok = false, want true", tt.data)
			}
			if !isResponse {
				t.Error("isResponse = false, want true for a status line")
			}
			if status != tt.wantStatus {
				t.Errorf("status = %q, want %q", status, tt.wantStatus)
			}
			if method != "" || path != "" {
				t.Errorf("method/path = %q/%q, want both empty for a response line", method, path)
			}
		})
	}
}

func TestParseHTTPLine_NotParseable(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"empty", ""},
		{"single token", "GET\r\n"},
		{"not http at all", "hello world this is not http\r\n"},
		{"method with no version token", "GET /foo bar\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, _, ok := parseHTTPLine([]byte(tt.data))
			if ok {
				t.Errorf("parseHTTPLine(%q) ok = true, want false", tt.data)
			}
		})
	}
}
