// Package socket implements Pulse's socket data telemetry: observing
// IPv4 TCP connection close via bpf/programs/tcp_close.c (byte counters,
// connection lifecycle end, pending socket errors) and normalizing what
// it reports into Pulse's canonical event model (pkg/model).
//
// Like internal/network, this package splits into a Linux
// implementation and cross-platform !linux stubs that fail closed with
// an error wrapping ebpf.ErrUnsupportedPlatform.
package socket

import "errors"

var (
	// ErrShortRead is returned when a ring buffer record is smaller
	// than the event type being decoded from it.
	ErrShortRead = errors.New("socket: truncated event payload")

	// ErrNotLoaded is returned by Loader methods that require Load (and,
	// for Read, Attach) to have succeeded first.
	ErrNotLoaded = errors.New("socket: program not loaded")
)
