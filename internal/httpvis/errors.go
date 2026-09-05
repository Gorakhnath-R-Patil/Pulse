// Package httpvis implements Pulse's HTTP visibility: observing
// cleartext HTTP request/response lines via
// bpf/programs/http_visibility.c and normalizing what it reports into
// Pulse's canonical event model (pkg/model).
//
// Like internal/process, internal/network, and internal/socket, this
// package splits into a Linux implementation and cross-platform
// !linux stubs that fail closed with an error wrapping
// ebpf.ErrUnsupportedPlatform.
package httpvis

import "errors"

// ErrShortRead is returned when a ring buffer record is smaller than
// the fixed header decodeRawEvent expects, or claims a captured_len
// longer than the bytes actually available.
var ErrShortRead = errors.New("httpvis: truncated event payload")

// ErrNotLoaded is returned by Loader methods that require Load (and,
// for Read, Attach) to have succeeded first.
var ErrNotLoaded = errors.New("httpvis: program not loaded")
