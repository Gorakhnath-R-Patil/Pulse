package httpvis

import (
	"strconv"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// ToEvent converts an HTTPEvent into Pulse's canonical event model.
// Method, path, and status are HTTP-specific detail that doesn't
// warrant its own top-level Event field — see
// pkg/model.Event.Attributes's doc comment — so they're carried there
// instead. There is no Network on the resulting Event: this technique
// observes that a process wrote HTTP-shaped data and what it said, not
// which socket/connection it went out on — see
// docs/design/http-visibility.md's Limitations.
func ToEvent(he HTTPEvent, host string) model.Event {
	eventType := "http.request"
	if he.IsResponse {
		eventType = "http.response"
	}

	attrs := map[string]string{
		"http.observed_bytes": strconv.FormatUint(uint64(he.Size), 10),
	}
	if he.Method != "" {
		attrs["http.method"] = he.Method
	}
	if he.Path != "" {
		attrs["http.path"] = he.Path
	}
	if he.Status != "" {
		attrs["http.status"] = he.Status
	}

	return model.Event{
		ID:        model.NewID(),
		Type:      eventType,
		Timestamp: he.Timestamp,
		Host:      host,
		Process: &model.Process{
			PID:     int32(he.PID),
			Command: he.Comm,
		},
		Attributes: attrs,
	}
}
