package dns

import (
	"strconv"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// ToEvent converts a DNSEvent into Pulse's canonical event model. Name,
// query type, response code, answer count, and latency are all DNS-
// specific detail that doesn't warrant its own top-level Event field —
// see pkg/model.Event.Attributes's doc comment — so they're carried
// there instead. There is no Network on the resulting Event, matching
// internal/httpvis: this technique observes syscall buffers, not socket
// structures, so no address/port information is available.
func ToEvent(de DNSEvent, host string) model.Event {
	eventType := "dns.query"
	if de.IsResponse {
		eventType = "dns.response"
	}

	attrs := map[string]string{
		"dns.name":  de.Name,
		"dns.qtype": strconv.FormatUint(uint64(de.QType), 10),
	}
	if de.IsResponse {
		attrs["dns.response_code"] = strconv.FormatUint(uint64(de.ResponseCode), 10)
		attrs["dns.answer_count"] = strconv.FormatUint(uint64(de.AnswerCount), 10)
		if de.Latency > 0 {
			attrs["dns.latency_ms"] = strconv.FormatFloat(de.Latency.Seconds()*1000, 'f', 3, 64)
		}
	}

	return model.Event{
		ID:        model.NewID(),
		Type:      eventType,
		Timestamp: de.Timestamp,
		Host:      host,
		Process: &model.Process{
			PID:     int32(de.PID),
			Command: de.Comm,
		},
		Attributes: attrs,
	}
}
