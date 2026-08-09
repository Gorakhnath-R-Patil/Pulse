// Package model defines Pulse's canonical internal telemetry record,
// Event, and the sub-structures it is composed of. Every event any Pulse
// component produces — regardless of what kernel activity it originated
// from — is represented as one Event, so that decoding, enrichment,
// correlation, and export (the pipeline stages introduced from Day 07
// onward) can all operate on a single stable shape.
//
// This is a schema package: it defines structure, identifiers,
// validation, and serialization. It intentionally contains no capture,
// decoding, or enrichment logic — that arrives with the specific
// subsystems that produce or consume events (eBPF capture from Day 03,
// service identity from Day 08, distributed tracing from Day 11, and so
// on). See docs/design/event-model.md for the full reasoning behind what
// is, and is not, part of the model yet.
package model

import (
	"fmt"
	"regexp"
	"time"
)

// typePattern constrains Event.Type to a "<domain>.<action>" convention,
// e.g. "process.start", "network.connect". The model does not know or
// enforce which specific values exist — that vocabulary is defined by
// whichever subsystem introduces a given domain — only that a value is
// present and consistently shaped.
var typePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// Event is Pulse's canonical telemetry record.
type Event struct {
	// ID uniquely identifies this event instance. Assigned by whichever
	// component first constructs the event, typically via NewID(). It is
	// independent of any distributed trace the event may later be
	// correlated into (trace/span identifiers are introduced in Day 11).
	ID string `json:"id"`

	// Type names the kind of observation this event represents, using
	// the "<domain>.<action>" convention described on typePattern.
	Type string `json:"type"`

	// Timestamp is when the observed activity occurred, not when the
	// event was constructed or received downstream.
	Timestamp time.Time `json:"timestamp"`

	// Host identifies the machine the activity was observed on.
	Host string `json:"host"`

	// Process identifies the process associated with the observed
	// activity, if known. Nil means no association is known.
	Process *Process `json:"process,omitempty"`

	// Network describes the network endpoints involved, if the observed
	// activity was network activity. Nil means not applicable.
	Network *Network `json:"network,omitempty"`

	// Attributes carries protocol- or domain-specific detail that
	// doesn't warrant its own top-level field yet (or ever) — e.g. an
	// HTTP method or a DNS query name once those subsystems exist. This
	// keeps the core shape above stable as new domains are added; see
	// ADR-006 in docs/design/decisions.md.
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Validate reports whether e is well-formed enough to enter the
// processing pipeline. It checks structure and format, not domain-level
// semantics (e.g. it confirms Type looks like "domain.action", not that
// "domain" is a domain Pulse recognizes).
func (e Event) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("%w: id", ErrMissingField)
	}
	if e.Type == "" {
		return fmt.Errorf("%w: type", ErrMissingField)
	}
	if !typePattern.MatchString(e.Type) {
		return fmt.Errorf("%w: type %q must match <domain>.<action> in lowercase (e.g. \"process.start\")", ErrInvalidField, e.Type)
	}
	if e.Timestamp.IsZero() {
		return fmt.Errorf("%w: timestamp", ErrMissingField)
	}
	if e.Host == "" {
		return fmt.Errorf("%w: host", ErrMissingField)
	}
	if e.Process != nil {
		if err := e.Process.Validate(); err != nil {
			return err
		}
	}
	if e.Network != nil {
		if err := e.Network.Validate(); err != nil {
			return err
		}
	}
	return nil
}
