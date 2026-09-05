//go:build linux

package httpvis

import (
	"errors"
	"fmt"
	"time"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/ebpf"
)

// Regenerate with `go generate ./...` after changing
// bpf/programs/http_visibility.c. Requires clang with a BPF target; see
// docs/development/getting-started.md.
//
// "HTTPVis" is capitalized deliberately, exporting the generated loader
// function as LoadHTTPVisObjects — see the equivalent note in
// internal/ebpf/loader_linux.go for why that matters.
//go:generate go tool bpf2go -cc clang -no-strip -tags linux HTTPVis ../../bpf/programs/http_visibility.c -- -I../../bpf/headers

// Loader owns the lifecycle of Pulse's HTTP visibility program: load,
// attach, receive, detach. See internal/ebpf's Loader, which this
// mirrors, for the calling contract: Load, Attach, and Read in that
// order, Close always safe.
//
// This program needs only BPF ring buffers and tracepoint support —
// ebpf.CheckSupport, unlike internal/network and internal/socket, which
// need fentry/fexit ("tracing") support instead — see
// bpf/programs/http_visibility.c's header comment for why this
// deliberately didn't follow their approach.
//
// A Loader is not safe for concurrent use by multiple goroutines.
type Loader struct {
	objs HTTPVisObjects
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

// Load checks kernel compatibility, then loads the HTTP visibility
// program and its ring buffer map into the kernel. It does not attach
// the program — no events are observed until Attach is called.
func (l *Loader) Load() error {
	if err := ebpf.CheckSupport(); err != nil {
		return err
	}

	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("httpvis: raising memlock limit: %w", err)
	}

	if err := LoadHTTPVisObjects(&l.objs, nil); err != nil {
		return fmt.Errorf("httpvis: loading http_visibility program: %w", err)
	}

	l.refMonotonicNS, l.refWallClock = ebpf.MonotonicReference()

	l.loaded = true
	return nil
}

// Attach attaches the loaded program to the syscalls:sys_enter_write
// tracepoint and opens a reader on its ring buffer. Load must have
// succeeded first.
func (l *Loader) Attach() error {
	if !l.loaded {
		return ErrNotLoaded
	}

	lnk, err := link.Tracepoint("syscalls", "sys_enter_write", l.objs.OnSysEnterWrite, nil)
	if err != nil {
		return fmt.Errorf("httpvis: attaching tracepoint: %w", err)
	}
	l.link = lnk

	rd, err := ringbuf.NewReader(l.objs.HttpEvents)
	if err != nil {
		lnk.Close()
		l.link = nil
		return fmt.Errorf("httpvis: opening ring buffer reader: %w", err)
	}
	l.rd = rd

	l.attached = true
	return nil
}

// Read blocks until the next event that actually parses as a complete
// HTTP request or status line is available, or until Close interrupts
// it. Ring buffer records that the kernel-side heuristic flagged but
// that don't parse (see parseHTTPLine) are silently skipped — that's
// expected, not an error — so Read may consume more than one record
// per call. Attach must have succeeded first.
func (l *Loader) Read() (HTTPEvent, error) {
	if !l.attached {
		return HTTPEvent{}, ErrNotLoaded
	}

	for {
		record, err := l.rd.Read()
		if err != nil {
			return HTTPEvent{}, fmt.Errorf("httpvis: reading ring buffer: %w", err)
		}

		raw, err := decodeRawEvent(record.RawSample)
		if err != nil {
			return HTTPEvent{}, err
		}

		method, path, status, isResponse, ok := parseHTTPLine(raw.Data)
		if !ok {
			continue
		}

		return HTTPEvent{
			Timestamp:  l.refWallClock.Add(time.Duration(int64(raw.TimestampNS) - int64(l.refMonotonicNS))),
			PID:        raw.PID,
			Comm:       raw.Comm,
			Size:       raw.Size,
			IsResponse: isResponse,
			Method:     method,
			Path:       path,
			Status:     status,
		}, nil
	}
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
			errs = append(errs, fmt.Errorf("detaching tracepoint: %w", err))
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
