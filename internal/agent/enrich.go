package agent

import (
	"github.com/Gorakhnath-R-Patil/Pulse/internal/discovery"
	"github.com/Gorakhnath-R-Patil/Pulse/internal/pipeline"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// containerEnrichingSource wraps another pipeline.EventSource, adding
// container identity (internal/discovery) to every event that has a
// Process — regardless of which capability produced it, since process
// discovery, network, and socket events all carry one. This is a
// decorator over EventSource rather than logic duplicated into each of
// process.go/network.go/socket.go's own source adapters, and rather
// than a pipeline.EventProcessor: EventProcessor receives events by
// value (see internal/pipeline's doc comment on why — it's a terminal
// consumer, not a transform step), so it has no way to hand a later
// processor an enriched copy. Wrapping the source is what lets
// enrichment happen exactly once, before anything downstream sees the
// event.
//
// Resolution failure (most commonly: the process isn't running in a
// recognized container, or has already exited) is not treated as a
// source read failure — it just leaves Process.Container unset, the
// same "unknown" outcome internal/process.ResolveExecutable's callers
// already treat this way.
type containerEnrichingSource struct {
	inner pipeline.EventSource
}

func (s containerEnrichingSource) Read() (model.Event, error) {
	event, err := s.inner.Read()
	if err != nil {
		return model.Event{}, err
	}

	if event.Process != nil {
		if container, err := discovery.ResolveContainer(uint32(event.Process.PID)); err == nil {
			event.Process.Container = container
		}
	}

	return event, nil
}
