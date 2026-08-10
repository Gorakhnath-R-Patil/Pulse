// Package ebpf is Pulse's minimal eBPF foundation: loading, attaching,
// receiving from, and cleanly detaching a BPF program, plus the kernel
// compatibility checks that guard those operations. It does not yet
// extract or normalize any telemetry — see bpf/programs/foundation.c and
// docs/design/ebpf-foundation.md for the boundary this package stays
// inside of today.
//
// eBPF is a Linux kernel feature. The Linux-specific implementation
// lives in files built only on Linux (loader_linux.go, compat_linux.go);
// on every other platform, this package's exported functions return
// ErrUnsupportedPlatform immediately rather than failing to compile or
// panicking, so the rest of the module keeps building on any OS.
package ebpf

import "errors"

var (
	// ErrUnsupportedPlatform is returned on any non-Linux OS: eBPF has
	// no equivalent there.
	ErrUnsupportedPlatform = errors.New("ebpf: unsupported platform")

	// ErrUnsupportedKernel is returned when running on Linux but the
	// kernel lacks a feature this package's foundation program needs
	// (BPF ring buffers, tracepoint programs).
	ErrUnsupportedKernel = errors.New("ebpf: kernel does not support a required eBPF feature")

	// ErrNotLoaded is returned by Loader methods that require Load to
	// have succeeded first.
	ErrNotLoaded = errors.New("ebpf: program not loaded")

	// ErrShortRead is returned when a ring buffer record is smaller
	// than the event type being decoded from it.
	ErrShortRead = errors.New("ebpf: truncated event payload")
)
