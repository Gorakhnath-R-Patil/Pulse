//go:build linux

package network

import (
	"errors"
	"fmt"
	"time"

	cebpf "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/ebpf"
)

// Regenerate with `go generate ./...` after changing
// bpf/programs/tcp_connect.c. Requires clang with a BPF target; see
// docs/development/getting-started.md.
//
// "TCPConnect" is capitalized deliberately, exporting the generated
// loader function as LoadTCPConnectObjects — see the equivalent note in
// internal/ebpf/loader_linux.go for why that matters.
//go:generate go tool bpf2go -cc clang -no-strip -tags linux TCPConnect ../../bpf/programs/tcp_connect.c -- -I../../bpf/headers

// Loader owns the lifecycle of Pulse's TCP connect telemetry program:
// load, attach, receive, detach. See internal/ebpf's Loader, which this
// mirrors, for the calling contract: Load, Attach, and Read in that
// order, Close always safe.
//
// A Loader is not safe for concurrent use by multiple goroutines.
type Loader struct {
	objs TCPConnectObjects
	link link.Link
	rd   *ringbuf.Reader

	loaded   bool
	attached bool

	refMonotonicNS uint64
	refWallClock   time.Time
}

// NewLoader returns an unloaded Loader. Load must be called before
// Attach, and Attach before Read.
func NewLoader() *Loader {
	return &Loader{}
}

// Load checks kernel compatibility, then loads the TCP connect program
// and its ring buffer map into the kernel. It does not attach the
// program — no events are observed until Attach is called.
//
// This program needs BPF ring buffers and fentry/fexit ("tracing")
// support, not tracepoint support — see internal/ebpf.HaveTracing's doc
// comment for why that's checked separately from ebpf.CheckSupport.
func (l *Loader) Load() error {
	if err := ebpf.HaveRingBuffer(); err != nil {
		return err
	}
	if err := ebpf.HaveTracing(); err != nil {
		return err
	}

	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("network: raising memlock limit: %w", err)
	}

	if err := LoadTCPConnectObjects(&l.objs, nil); err != nil {
		return fmt.Errorf("network: loading tcp_connect program: %w", err)
	}

	l.refMonotonicNS, l.refWallClock = ebpf.MonotonicReference()

	l.loaded = true
	return nil
}

// Attach attaches the loaded program to tcp_v4_connect and opens a
// reader on its ring buffer. Load must have succeeded first.
func (l *Loader) Attach() error {
	if !l.loaded {
		return ErrNotLoaded
	}

	lnk, err := link.AttachTracing(link.TracingOptions{
		Program:    l.objs.OnTcpV4Connect,
		AttachType: cebpf.AttachTraceFExit,
	})
	if err != nil {
		return fmt.Errorf("network: attaching fexit/tcp_v4_connect: %w", err)
	}
	l.link = lnk

	rd, err := ringbuf.NewReader(l.objs.ConnectEvents)
	if err != nil {
		lnk.Close()
		l.link = nil
		return fmt.Errorf("network: opening ring buffer reader: %w", err)
	}
	l.rd = rd

	l.attached = true
	return nil
}

// Read blocks until the next event arrives, or until Close interrupts
// it. Attach must have succeeded first.
func (l *Loader) Read() (ConnectEvent, error) {
	if !l.attached {
		return ConnectEvent{}, ErrNotLoaded
	}

	record, err := l.rd.Read()
	if err != nil {
		return ConnectEvent{}, fmt.Errorf("network: reading ring buffer: %w", err)
	}

	raw, err := decodeRawEvent(record.RawSample)
	if err != nil {
		return ConnectEvent{}, err
	}

	return ConnectEvent{
		Timestamp:  l.refWallClock.Add(time.Duration(int64(raw.TimestampNS) - int64(l.refMonotonicNS))),
		PID:        raw.PID,
		SourceAddr: raw.SourceAddr,
		DestAddr:   raw.DestAddr,
		SourcePort: raw.SourcePort,
		DestPort:   raw.DestPort,
		Comm:       raw.Comm,
		Success:    raw.Success,
	}, nil
}

// Close detaches the program (if attached) and releases every kernel
// resource Load/Attach acquired (if any), collecting every error
// encountered rather than stopping at the first. Safe to call multiple
// times and even when Load or Attach never succeeded.
func (l *Loader) Close() error {
	var errs []error

	if l.rd != nil {
		if err := l.rd.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing ring buffer reader: %w", err))
		}
		l.rd = nil
	}
	if l.link != nil {
		if err := l.link.Close(); err != nil {
			errs = append(errs, fmt.Errorf("detaching fexit/tcp_v4_connect: %w", err))
		}
		l.link = nil
	}
	if l.loaded {
		if err := l.objs.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing program/map handles: %w", err))
		}
		l.loaded = false
	}
	l.attached = false

	return errors.Join(errs...)
}
