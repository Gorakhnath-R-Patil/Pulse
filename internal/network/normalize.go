package network

import (
	"strconv"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// ToEvent converts a ConnectEvent into Pulse's canonical event model.
// Whether the connect ultimately succeeded is TCP-connect-specific
// detail that doesn't warrant its own top-level Event field — see
// pkg/model.Event.Attributes's doc comment — so it's carried there
// instead, under the "tcp.connect_success" key.
func ToEvent(ce ConnectEvent, host string) model.Event {
	return model.Event{
		ID:        model.NewID(),
		Type:      "network.connect",
		Timestamp: ce.Timestamp,
		Host:      host,
		Process: &model.Process{
			PID:     int32(ce.PID),
			Command: ce.Comm,
		},
		Network: &model.Network{
			Protocol:    "tcp",
			Source:      model.Endpoint{Address: ce.SourceAddr.String(), Port: ce.SourcePort},
			Destination: model.Endpoint{Address: ce.DestAddr.String(), Port: ce.DestPort},
		},
		Attributes: map[string]string{
			"tcp.connect_success": strconv.FormatBool(ce.Success),
		},
	}
}
