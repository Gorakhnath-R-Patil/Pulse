// Package dns implements Pulse's DNS telemetry: observing DNS queries
// and responses via bpf/programs/dns_telemetry.c, correlating a
// response with its query to compute latency, and normalizing both
// into Pulse's canonical event model (pkg/model).
//
// Like internal/process, internal/network, internal/socket, and
// internal/httpvis, this package splits into a Linux implementation
// and cross-platform !linux stubs that fail closed with an error
// wrapping ebpf.ErrUnsupportedPlatform.
package dns

import "errors"

var (
	// ErrShortMessage is returned when a captured payload is too short
	// to contain a DNS header, or a name/label runs past the end of
	// what was captured.
	ErrShortMessage = errors.New("dns: message too short to parse")

	// ErrNoQuestion is returned when a message's question count is
	// zero — valid DNS, but nothing this package can extract a domain
	// name from.
	ErrNoQuestion = errors.New("dns: message has no question section")

	// ErrUnsupportedName is returned when the first question's name
	// uses label compression — see parseName's doc comment for why
	// this package doesn't resolve it.
	ErrUnsupportedName = errors.New("dns: name uses unsupported compression")

	// ErrShortRead is returned when a ring buffer record is smaller
	// than the fixed header decodeRawEvent expects, or claims a
	// captured_len longer than the bytes actually available.
	ErrShortRead = errors.New("dns: truncated event payload")

	// ErrNotLoaded is returned by Loader methods that require Load
	// (and, for Read, Attach) to have succeeded first.
	ErrNotLoaded = errors.New("dns: program not loaded")
)
