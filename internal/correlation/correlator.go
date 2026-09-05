// Package correlation implements Pulse's trace correlation: turning a
// stream of raw pkg/model.Event values into pkg/model.Span values that
// internal/tracing.AssembleTrace could organize into a tree.
//
// "When sufficient metadata exists" (this day's own framing) is the
// operative phrase. What Days 04–10 actually capture gives exactly one
// strong, universally-available correlation signal: which process (PID)
// produced an event, and when. It does not give trace-context
// propagation (no capture day parses an application-level trace
// header) or even a shared network/socket identity across event types —
// internal/httpvis and internal/dns events carry no address information
// at all, by their own documented design (see those packages' Design
// docs). So this package correlates by process and time proximity: an
// honest, useful signal, not the same thing as knowing two events are
// truly causally related.
//
// This is also why it does not attempt the master example's
// "API → Order → Payment → PostgreSQL" cross-service chain: that needs
// seeing both sides of a network call (this project only captures
// outbound connects — see docs/design/network-connect.md's
// Limitations) or aggregating multiple hosts' agents together (Kafka
// transport doesn't exist until Day 14). What this package builds is
// real and useful on its own — a single process's own sequence of
// observed activity, correlated into one trace — just not that. See
// docs/design/trace-correlation.md for the full reasoning.
package correlation

import (
	"sync"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// defaultWindow is how much time may pass between two events from the
// same process before Correlator starts a new trace rather than
// chaining onto the previous one.
const defaultWindow = 30 * time.Second

// sessionTTL bounds how long a process's session is remembered with no
// activity at all, so a long-running agent watching many short-lived
// processes doesn't accumulate state forever. Deliberately much larger
// than the correlation window: window governs whether one event
// continues an existing trace; sessionTTL only governs when a
// completely inactive session is forgotten.
const sessionTTL = 10 * time.Minute

// Correlator groups a stream of events into best-effort traces — see
// the package doc comment for what "best-effort" means and why.
//
// Unlike this project's per-capability Loaders (each read by exactly
// one goroutine and documented as unsafe for concurrent use), a
// Correlator is meant to be shared across every capability's pipeline
// so their events land in the same trace when they share a process —
// see internal/agent's wiring. It is safe for concurrent use.
type Correlator struct {
	mu       sync.Mutex
	window   time.Duration
	sessions map[int32]*session
}

type session struct {
	traceID       model.TraceID
	lastSpanID    model.SpanID
	lastEventTime time.Time
}

// New returns a Correlator that chains events from the same process
// into one trace as long as no two consecutive events are more than
// window apart. A non-positive window uses defaultWindow.
func New(window time.Duration) *Correlator {
	if window <= 0 {
		window = defaultWindow
	}
	return &Correlator{window: window, sessions: make(map[int32]*session)}
}

// Observe turns event into a Span, correlating it with whatever trace
// its process is already part of — if one exists and was active
// recently enough, see New — or starting a new trace otherwise.
//
// It never fails: an event with no Process to correlate by becomes the
// root of its own singleton trace rather than being rejected. That's
// this package's answer to "handle incomplete traces gracefully" —
// thin metadata degrades the quality of correlation, not the ability
// to produce a span at all.
func (c *Correlator) Observe(event model.Event) model.Span {
	span := model.Span{
		SpanID:     model.NewSpanID(),
		Name:       event.Type,
		Service:    serviceLabel(event),
		StartTime:  event.Timestamp,
		Attributes: event.Attributes,
	}

	if event.Process == nil {
		span.TraceID = model.NewTraceID()
		return span
	}
	pid := event.Process.PID

	c.mu.Lock()
	defer c.mu.Unlock()

	c.evictStale(event.Timestamp)

	sess, ok := c.sessions[pid]
	if !ok || event.Timestamp.Sub(sess.lastEventTime) > c.window {
		// First event ever seen for this process, or too long a gap
		// since its last one to treat this as a continuation.
		span.TraceID = model.NewTraceID()
		sess = &session{traceID: span.TraceID}
		c.sessions[pid] = sess
	} else {
		span.TraceID = sess.traceID
		span.ParentSpanID = sess.lastSpanID
	}

	sess.lastSpanID = span.SpanID
	sess.lastEventTime = event.Timestamp

	return span
}

func (c *Correlator) evictStale(now time.Time) {
	for pid, sess := range c.sessions {
		if now.Sub(sess.lastEventTime) > sessionTTL {
			delete(c.sessions, pid)
		}
	}
}

// serviceLabel derives a best-available label for whichever process
// produced event: a container ID if internal/discovery resolved one
// (see internal/agent's containerEnrichingSource, which runs before any
// EventProcessor sees the event), else the process's command name,
// else "unknown". This is not a Day 19 Kubernetes-resolved service
// *name* — see docs/design/service-identity.md — just the best label
// available from what's already been captured.
func serviceLabel(event model.Event) string {
	if event.Process == nil {
		return "unknown"
	}
	if event.Process.Container != nil && event.Process.Container.ID != "" {
		return event.Process.Container.ID
	}
	if event.Process.Command != "" {
		return event.Process.Command
	}
	return "unknown"
}
