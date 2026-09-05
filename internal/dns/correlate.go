package dns

import "time"

// queryCorrelator matches a DNS response to its earlier query by (PID,
// transaction ID) so a response event can carry a real latency. It is
// not safe for concurrent use — Loader.Read calls it from one goroutine
// only, the same "not safe for concurrent use" contract every Loader in
// this project already has.
//
// This is deliberately its own type, not inlined into Loader: unlike
// the actual kernel capture, matching a response to a query is pure
// logic with no OS dependency, and is exactly the kind of thing worth
// being able to test with synthetic timestamps rather than a real
// clock — see correlate_test.go.
type queryCorrelator struct {
	pending map[pendingKey]time.Time
	ttl     time.Duration
}

type pendingKey struct {
	pid uint32
	id  uint16
}

// newQueryCorrelator returns a queryCorrelator that forgets a query it
// never saw a response for after ttl, bounding pending's size even if
// a query is never answered.
func newQueryCorrelator(ttl time.Duration) *queryCorrelator {
	return &queryCorrelator{pending: make(map[pendingKey]time.Time), ttl: ttl}
}

// observeQuery records a query's timestamp so a later matching response
// can compute latency against it.
func (c *queryCorrelator) observeQuery(pid uint32, id uint16, now time.Time) {
	c.evictStale(now)
	c.pending[pendingKey{pid, id}] = now
}

// observeResponse reports the latency since the matching query, if one
// is still pending. ok is false if no matching query was recorded — it
// was never observed, already matched by an earlier response, or
// evicted as stale — in which case latency is meaningless and the
// caller should leave it unset rather than report a zero.
func (c *queryCorrelator) observeResponse(pid uint32, id uint16, now time.Time) (latency time.Duration, ok bool) {
	c.evictStale(now)
	key := pendingKey{pid, id}
	queryTime, found := c.pending[key]
	if !found {
		return 0, false
	}
	delete(c.pending, key)
	return now.Sub(queryTime), true
}

func (c *queryCorrelator) evictStale(now time.Time) {
	for k, t := range c.pending {
		if now.Sub(t) > c.ttl {
			delete(c.pending, k)
		}
	}
}
