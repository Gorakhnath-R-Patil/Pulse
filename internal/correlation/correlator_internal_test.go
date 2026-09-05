package correlation

import (
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// TestCorrelator_EvictsStaleSessions verifies sessionTTL actually
// bounds memory: a process that goes quiet forever doesn't keep its
// session around indefinitely just because Observe never returns an
// error for it.
func TestCorrelator_EvictsStaleSessions(t *testing.T) {
	c := New(30 * time.Second)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for pid := int32(0); pid < 100; pid++ {
		c.Observe(model.Event{
			Type:      "network.connect",
			Timestamp: t0,
			Process:   &model.Process{PID: pid},
		})
	}
	if len(c.sessions) != 100 {
		t.Fatalf("len(sessions) = %d, want 100 before any eviction", len(c.sessions))
	}

	// Any later event triggers a sweep, regardless of whose PID it is.
	c.Observe(model.Event{
		Type:      "network.connect",
		Timestamp: t0.Add(sessionTTL + time.Second),
		Process:   &model.Process{PID: 9999},
	})

	if len(c.sessions) != 1 {
		t.Errorf("len(sessions) = %d, want 1 (only the just-added session) after the sweep", len(c.sessions))
	}
}
