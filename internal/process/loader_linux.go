//go:build linux

package process

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
// bpf/programs/process.c. Requires clang with a BPF target; see
// docs/development/getting-started.md.
//
// "Process" is capitalized deliberately, exporting the generated loader
// function as LoadProcessObjects — see the equivalent note in
// internal/ebpf/loader_linux.go for why that matters.
//go:generate go tool bpf2go -cc clang -no-strip -tags linux Process ../../bpf/programs/process.c -- -I../../bpf/headers

// Loader owns the lifecycle of Pulse's process discovery program: load,
// attach (to both the exec and exit tracepoints), receive, detach. See
// internal/ebpf's Loader, which this mirrors, for the calling contract:
// Load, Attach, and Read in that order, Close always safe.
//
// A Loader is not safe for concurrent use by multiple goroutines.
type Loader struct {
	objs     ProcessObjects
	execLink link.Link
	exitLink link.Link
	rd       *ringbuf.Reader

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

// Load checks kernel compatibility, then loads the process discovery
// program and its ring buffer map into the kernel. It does not attach
// the program — no events are observed until Attach is called.
func (l *Loader) Load() error {
	if err := ebpf.CheckSupport(); err != nil {
		return err
	}

	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("process: raising memlock limit: %w", err)
	}

	if err := LoadProcessObjects(&l.objs, nil); err != nil {
		return fmt.Errorf("process: loading process program: %w", err)
	}

	// Captured now, at Load time, so it's available before the first
	// event ever arrives.
	l.refMonotonicNS, l.refWallClock = ebpf.MonotonicReference()

	l.loaded = true
	return nil
}

// Attach attaches the loaded program to the sched_process_exec and
// sched_process_exit tracepoints and opens a reader on their shared
// ring buffer. Load must have succeeded first.
func (l *Loader) Attach() error {
	if !l.loaded {
		return ErrNotLoaded
	}

	execLink, err := link.Tracepoint("sched", "sched_process_exec", l.objs.OnProcessExec, nil)
	if err != nil {
		return fmt.Errorf("process: attaching sched_process_exec: %w", err)
	}
	l.execLink = execLink

	exitLink, err := link.Tracepoint("sched", "sched_process_exit", l.objs.OnProcessExit, nil)
	if err != nil {
		execLink.Close()
		l.execLink = nil
		return fmt.Errorf("process: attaching sched_process_exit: %w", err)
	}
	l.exitLink = exitLink

	rd, err := ringbuf.NewReader(l.objs.ProcessEvents)
	if err != nil {
		exitLink.Close()
		execLink.Close()
		l.exitLink = nil
		l.execLink = nil
		return fmt.Errorf("process: opening ring buffer reader: %w", err)
	}
	l.rd = rd

	l.attached = true
	return nil
}

// Read blocks until the next event arrives, or until Close interrupts
// it. Attach must have succeeded first.
func (l *Loader) Read() (ProcessEvent, error) {
	if !l.attached {
		return ProcessEvent{}, ErrNotLoaded
	}

	record, err := l.rd.Read()
	if err != nil {
		return ProcessEvent{}, fmt.Errorf("process: reading ring buffer: %w", err)
	}

	raw, err := decodeRawEvent(record.RawSample)
	if err != nil {
		return ProcessEvent{}, err
	}

	return ProcessEvent{
		Timestamp: l.refWallClock.Add(time.Duration(int64(raw.TimestampNS) - int64(l.refMonotonicNS))),
		PID:       raw.PID,
		PPID:      raw.PPID,
		Comm:      raw.Comm,
		Type:      raw.Type,
	}, nil
}

// Close detaches both tracepoints (if attached) and releases every
// kernel resource Load/Attach acquired (if any), collecting every error
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
	if l.exitLink != nil {
		if err := l.exitLink.Close(); err != nil {
			errs = append(errs, fmt.Errorf("detaching sched_process_exit: %w", err))
		}
		l.exitLink = nil
	}
	if l.execLink != nil {
		if err := l.execLink.Close(); err != nil {
			errs = append(errs, fmt.Errorf("detaching sched_process_exec: %w", err))
		}
		l.execLink = nil
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
