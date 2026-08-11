//go:build linux

package ebpf

import (
	"errors"
	"fmt"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
)

// Regenerate with `go generate ./...` after changing
// bpf/programs/foundation.c. Requires clang with a BPF target; see
// docs/design/ebpf-foundation.md for the toolchain this depends on.
//
// The "Foundation" identifier below is capitalized deliberately: bpf2go
// exports its generated loader function (LoadFoundationObjects, not
// loadFoundationObjects) whenever the identifier itself looks exported
// (see cmd/bpf2go/gen's templateName.maybeExport). Lowercase the
// identifier and the call below would need to lowercase its first
// letter too.
//go:generate go tool bpf2go -cc clang -no-strip -tags linux Foundation ../../bpf/programs/foundation.c -- -I../../bpf/headers

// Loader owns the lifecycle of Pulse's eBPF foundation program: load,
// attach, receive, detach. Load, Attach, and Read are meant to be called
// in that order, each once. Close is the exception: it is always safe
// to call, including after a failed or partial Load/Attach, so callers
// can unconditionally `defer loader.Close()` right after NewLoader.
//
// A Loader is not safe for concurrent use by multiple goroutines.
type Loader struct {
	objs FoundationObjects
	link link.Link
	rd   *ringbuf.Reader

	loaded   bool
	attached bool
}

// NewLoader returns an unloaded Loader. Load must be called before
// Attach, and Attach before Read.
func NewLoader() *Loader {
	return &Loader{}
}

// Load checks kernel compatibility (see CheckSupport), then loads the
// foundation program and its ring buffer map into the kernel. It does
// not attach the program — no events are observed until Attach is
// called.
func (l *Loader) Load() error {
	if err := CheckSupport(); err != nil {
		return err
	}

	// Permits locking the memory eBPF maps need. A no-op on kernels
	// 5.11+, where locked memory is charged to the memory cgroup
	// instead of RLIMIT_MEMLOCK. Safe to call repeatedly.
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("ebpf: raising memlock limit: %w", err)
	}

	if err := LoadFoundationObjects(&l.objs, nil); err != nil {
		return fmt.Errorf("ebpf: loading foundation program: %w", err)
	}
	l.loaded = true
	return nil
}

// Attach attaches the loaded program to the syscalls:sys_enter_execve
// tracepoint and opens a reader on its ring buffer. Load must have
// succeeded first.
func (l *Loader) Attach() error {
	if !l.loaded {
		return ErrNotLoaded
	}

	lnk, err := link.Tracepoint("syscalls", "sys_enter_execve", l.objs.OnSysEnterExecve, nil)
	if err != nil {
		return fmt.Errorf("ebpf: attaching tracepoint: %w", err)
	}
	l.link = lnk

	rd, err := ringbuf.NewReader(l.objs.Heartbeats)
	if err != nil {
		lnk.Close()
		l.link = nil
		return fmt.Errorf("ebpf: opening ring buffer reader: %w", err)
	}
	l.rd = rd

	l.attached = true
	return nil
}

// Read blocks until the next event arrives, or until Close interrupts
// it (returning an error wrapping ringbuf.ErrClosed). Attach must have
// succeeded first.
func (l *Loader) Read() (HeartbeatEvent, error) {
	if !l.attached {
		return HeartbeatEvent{}, ErrNotLoaded
	}

	record, err := l.rd.Read()
	if err != nil {
		return HeartbeatEvent{}, fmt.Errorf("ebpf: reading ring buffer: %w", err)
	}

	return DecodeHeartbeatEvent(record.RawSample)
}

// Close detaches the program (if attached) and releases every kernel
// resource Load/Attach acquired (if any), collecting every error
// encountered rather than stopping at the first. It is safe to call
// multiple times and safe to call even when Load or Attach never
// succeeded.
func (l *Loader) Close() error {
	var errs []error

	// Close the ring buffer reader first so any goroutine blocked in
	// Read unblocks promptly, before we start tearing down what it
	// reads from.
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
