// Package process implements Pulse's process discovery: observing
// process start and exit via bpf/programs/process.c and normalizing
// what it reports into Pulse's canonical event model
// (github.com/Gorakhnath-R-Patil/Pulse/pkg/model).
//
// Like internal/ebpf, this package splits into a Linux implementation
// and cross-platform !linux stubs that fail closed with an error
// wrapping ebpf.ErrUnsupportedPlatform, so callers (internal/agent) can
// hold a *Loader on any OS without their own build tags.
package process

import "errors"

var (
	// ErrShortRead is returned when a ring buffer record is smaller
	// than the event type being decoded from it.
	ErrShortRead = errors.New("process: truncated event payload")

	// ErrNotLoaded is returned by Loader methods that require Load (and,
	// for Read, Attach) to have succeeded first.
	ErrNotLoaded = errors.New("process: program not loaded")
)
