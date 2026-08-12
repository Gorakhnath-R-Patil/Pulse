package process

import "github.com/Gorakhnath-R-Patil/Pulse/pkg/model"

// ToEvent converts a ProcessEvent into Pulse's canonical event model.
//
// executable is the resolved path to the running binary, if known.
// Resolution requires a live /proc lookup this function does not
// perform itself (see ResolveExecutable) — keeping ToEvent a pure,
// deterministic mapping that's fully unit-testable without touching the
// filesystem. An empty executable is valid: it means resolution wasn't
// attempted, or the process was already gone by the time it was (see
// ResolveExecutable's doc comment).
func ToEvent(pe ProcessEvent, host, executable string) model.Event {
	eventType := "process.start"
	if pe.Type == EventExit {
		eventType = "process.exit"
	}

	return model.Event{
		ID:        model.NewID(),
		Type:      eventType,
		Timestamp: pe.Timestamp,
		Host:      host,
		Process: &model.Process{
			PID:        int32(pe.PID),
			Command:    pe.Comm,
			Executable: executable,
		},
	}
}
