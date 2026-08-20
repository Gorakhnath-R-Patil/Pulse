package socket

import (
	"strconv"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// ToEvent converts a CloseEvent into Pulse's canonical event model.
// SockError doesn't warrant its own top-level Event field — see
// pkg/model.Event.Attributes's doc comment — so a nonzero value is
// carried there instead, under the "tcp.sock_error" key.
func ToEvent(ce CloseEvent, host string) model.Event {
	event := model.Event{
		ID:        model.NewID(),
		Type:      "network.close",
		Timestamp: ce.Timestamp,
		Host:      host,
		Process: &model.Process{
			PID:     int32(ce.PID),
			Command: ce.Comm,
		},
		Network: &model.Network{
			Protocol:      "tcp",
			Source:        model.Endpoint{Address: ce.SourceAddr.String(), Port: ce.SourcePort},
			Destination:   model.Endpoint{Address: ce.DestAddr.String(), Port: ce.DestPort},
			BytesSent:     ce.BytesSent,
			BytesReceived: ce.BytesReceived,
		},
	}

	if ce.SockError != 0 {
		event.Attributes = map[string]string{
			"tcp.sock_error": strconv.FormatUint(uint64(ce.SockError), 10),
		}
	}

	return event
}
