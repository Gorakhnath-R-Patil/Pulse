// Package network implements Pulse's network connection telemetry:
// observing outbound IPv4 TCP connection attempts via
// bpf/programs/tcp_connect.c and normalizing what it reports into
// Pulse's canonical event model (pkg/model).
//
// Like internal/ebpf and internal/process, this package splits into a
// Linux implementation and cross-platform !linux stubs that fail closed
// with an error wrapping ebpf.ErrUnsupportedPlatform.
package network

import "errors"

var (
	// ErrShortRead is returned when a ring buffer record is smaller
	// than the event type being decoded from it.
	ErrShortRead = errors.New("network: truncated event payload")

	// ErrNotLoaded is returned by Loader methods that require Load (and,
	// for Read, Attach) to have succeeded first.
	ErrNotLoaded = errors.New("network: program not loaded")
)
