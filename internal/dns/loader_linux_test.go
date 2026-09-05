//go:build linux

package dns_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cilium/ebpf/ringbuf"

	pulsedns "github.com/Gorakhnath-R-Patil/Pulse/internal/dns"
)

// requireRoot skips tests that need to actually load a BPF program —
// see internal/ebpf/loader_linux_test.go's identical helper for why.
func requireRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("loading eBPF programs requires root (or equivalent capabilities); run with sudo to exercise this test")
	}
}

// buildDNSQuery/buildDNSResponse build minimal real wire-format DNS
// messages for this file's integration test. message_test.go has
// near-identical helpers, but package dns_test (a separate, black-box
// package) can't see them — small, self-contained duplication here is
// simpler than exporting test-only helpers from the package under test.
func buildDNSQuery(id uint16, name string) []byte {
	var buf bytes.Buffer
	writeUint16(&buf, id)
	buf.Write([]byte{0x01, 0x00}) // QR=0, RD=1
	writeUint16(&buf, 1)          // QDCOUNT
	writeUint16(&buf, 0)
	writeUint16(&buf, 0)
	writeUint16(&buf, 0)
	writeDNSName(&buf, name)
	writeUint16(&buf, 1) // QTYPE = A
	writeUint16(&buf, 1) // QCLASS = IN
	return buf.Bytes()
}

func buildDNSResponse(id uint16, name string) []byte {
	var buf bytes.Buffer
	writeUint16(&buf, id)
	buf.Write([]byte{0x81, 0x80}) // QR=1, RD=1, RA=1, RCODE=0
	writeUint16(&buf, 1)          // QDCOUNT
	writeUint16(&buf, 1)          // ANCOUNT
	writeUint16(&buf, 0)
	writeUint16(&buf, 0)
	writeDNSName(&buf, name)
	writeUint16(&buf, 1) // QTYPE = A
	writeUint16(&buf, 1) // QCLASS = IN
	return buf.Bytes()
}

func writeUint16(buf *bytes.Buffer, v uint16) {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	buf.Write(b[:])
}

func writeDNSName(buf *bytes.Buffer, name string) {
	for _, label := range strings.Split(name, ".") {
		buf.WriteByte(byte(len(label)))
		buf.WriteString(label)
	}
	buf.WriteByte(0)
}

func TestLoader_AttachBeforeLoadFails(t *testing.T) {
	l := pulsedns.NewLoader()
	defer l.Close()

	if err := l.Attach(); !errors.Is(err, pulsedns.ErrNotLoaded) {
		t.Errorf("Attach() before Load(): error = %v, want it to wrap ErrNotLoaded", err)
	}
}

func TestLoader_ReadBeforeAttachFails(t *testing.T) {
	l := pulsedns.NewLoader()
	defer l.Close()

	if _, err := l.Read(); !errors.Is(err, pulsedns.ErrNotLoaded) {
		t.Errorf("Read() before Attach(): error = %v, want it to wrap ErrNotLoaded", err)
	}
}

// TestLoader_ObservesRealQueryAndResponse is the integration test: a
// real UDP "server" goroutine replies to a real query after a
// deliberate delay, and this confirms the loader observes both the
// query and the response, correctly correlated with a latency close to
// that delay — not just that two events showed up.
func TestLoader_ObservesRealQueryAndResponse(t *testing.T) {
	requireRoot(t)

	l := pulsedns.NewLoader()
	defer l.Close()

	if err := l.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if err := l.Attach(); err != nil {
		t.Fatalf("Attach() error: %v", err)
	}

	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("net.ListenUDP() error: %v", err)
	}
	defer server.Close()

	const (
		txID        = 0xBEEF
		name        = "pulse-integration-test.example"
		serverDelay = 20 * time.Millisecond
	)
	query := buildDNSQuery(txID, name)
	response := buildDNSResponse(txID, name)

	go func() {
		buf := make([]byte, 512)
		_, addr, err := server.ReadFromUDP(buf)
		if err != nil {
			return
		}
		time.Sleep(serverDelay)
		_, _ = server.WriteToUDP(response, addr)
	}()

	client, err := net.DialUDP("udp4", nil, server.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatalf("net.DialUDP() error: %v", err)
	}
	defer client.Close()

	if _, err := client.Write(query); err != nil {
		t.Fatalf("client.Write() error: %v", err)
	}

	wantPID := uint32(os.Getpid())

	type result struct {
		response pulsedns.DNSEvent
		err      error
	}
	done := make(chan result, 1)
	go func() {
		haveQuery, haveResponse := false, false
		var response pulsedns.DNSEvent
		for !haveQuery || !haveResponse {
			event, err := l.Read()
			if err != nil {
				done <- result{err: err}
				return
			}
			// Some other process's DNS traffic on this shared runner
			// could be read first; keep going until it's ours.
			if event.PID != wantPID || event.Name != name {
				continue
			}
			if event.IsResponse {
				response = event
				haveResponse = true
			} else {
				haveQuery = true
			}
		}
		done <- result{response: response}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Read() error: %v", r.err)
		}
		if r.response.Latency < serverDelay-5*time.Millisecond {
			t.Errorf("Latency = %v, want at least ~%v (the server's deliberate delay)", r.response.Latency, serverDelay)
		}
		if r.response.ResponseCode != 0 {
			t.Errorf("ResponseCode = %d, want 0 (NOERROR)", r.response.ResponseCode)
		}
		if r.response.AnswerCount != 1 {
			t.Errorf("AnswerCount = %d, want 1", r.response.AnswerCount)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not observe both our query and response within 5s")
	}
}

func TestLoader_CloseInterruptsRead(t *testing.T) {
	requireRoot(t)

	l := pulsedns.NewLoader()
	if err := l.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if err := l.Attach(); err != nil {
		t.Fatalf("Attach() error: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := l.Read()
		done <- err
	}()

	time.Sleep(100 * time.Millisecond)
	if err := l.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, ringbuf.ErrClosed) {
			t.Errorf("Read() error = %v, want it to wrap ringbuf.ErrClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Read() did not return within 2s of Close()")
	}
}
