package model

import "fmt"

// Endpoint is one side of a network connection.
type Endpoint struct {
	// Address is an IP address. Hostname resolution, if any, is a
	// separate enrichment concern, not part of the raw observation.
	Address string `json:"address"`

	// Port is the transport-layer port. Zero means "not applicable or
	// not observed" (e.g. ICMP has no port).
	Port uint16 `json:"port,omitempty"`
}

// Network describes the network endpoints involved in an observed
// activity. Protocol-specific detail such as HTTP method/status or DNS
// query/response (Day 9/10), and latency (Day 9), are deliberately not
// fields here — see docs/design/event-model.md for where they land
// instead as those days implement them.
type Network struct {
	// Protocol is the transport (or application) protocol name, e.g.
	// "tcp", "udp". Lowercase by convention.
	Protocol string `json:"protocol,omitempty"`

	Source      Endpoint `json:"source"`
	Destination Endpoint `json:"destination"`

	// BytesSent and BytesReceived are cumulative byte counts for the
	// connection at observation time, if known. Zero means "unknown or
	// not applicable," not "confirmed zero bytes transferred" — most
	// events (e.g. a bare connection attempt) simply don't have this
	// information yet.
	BytesSent     uint64 `json:"bytes_sent,omitempty"`
	BytesReceived uint64 `json:"bytes_received,omitempty"`
}

// Validate reports whether n is a well-formed Network reference. A nil
// *Network on an Event means "not network-related," which is valid;
// Validate is only called on a non-nil Network.
func (n Network) Validate() error {
	if n.Source.Address == "" {
		return fmt.Errorf("%w: network.source.address", ErrMissingField)
	}
	if n.Destination.Address == "" {
		return fmt.Errorf("%w: network.destination.address", ErrMissingField)
	}
	return nil
}
