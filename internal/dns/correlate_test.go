package dns

import (
	"testing"
	"time"
)

func TestQueryCorrelator_MatchesQueryAndResponse(t *testing.T) {
	c := newQueryCorrelator(5 * time.Second)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	c.observeQuery(100, 0x1234, t0)

	latency, ok := c.observeResponse(100, 0x1234, t0.Add(50*time.Millisecond))
	if !ok {
		t.Fatal("observeResponse() ok = false, want true for a matching query")
	}
	if latency != 50*time.Millisecond {
		t.Errorf("latency = %v, want 50ms", latency)
	}
}

func TestQueryCorrelator_NoMatchingQuery(t *testing.T) {
	c := newQueryCorrelator(5 * time.Second)

	_, ok := c.observeResponse(100, 0x1234, time.Now())
	if ok {
		t.Error("observeResponse() ok = true, want false with no prior query observed")
	}
}

func TestQueryCorrelator_DifferentPIDsDontCollide(t *testing.T) {
	c := newQueryCorrelator(5 * time.Second)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	c.observeQuery(100, 0x1234, t0) // pid 100's query

	// A different process happening to pick the same transaction ID
	// must not be treated as a match for pid 100's query.
	_, ok := c.observeResponse(200, 0x1234, t0.Add(time.Millisecond))
	if ok {
		t.Error("observeResponse() ok = true for a different PID, want false")
	}

	// pid 100's own response still matches.
	latency, ok := c.observeResponse(100, 0x1234, t0.Add(10*time.Millisecond))
	if !ok {
		t.Fatal("observeResponse() ok = false for the correct PID, want true")
	}
	if latency != 10*time.Millisecond {
		t.Errorf("latency = %v, want 10ms", latency)
	}
}

func TestQueryCorrelator_ResponseConsumesMatchOnlyOnce(t *testing.T) {
	c := newQueryCorrelator(5 * time.Second)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	c.observeQuery(100, 0x1234, t0)
	if _, ok := c.observeResponse(100, 0x1234, t0.Add(time.Millisecond)); !ok {
		t.Fatal("first observeResponse() ok = false, want true")
	}

	// A second, e.g. duplicate/retransmitted, response with the same ID
	// has nothing left to match against.
	if _, ok := c.observeResponse(100, 0x1234, t0.Add(2*time.Millisecond)); ok {
		t.Error("second observeResponse() ok = true, want false (query already matched)")
	}
}

func TestQueryCorrelator_EvictsStaleEntries(t *testing.T) {
	c := newQueryCorrelator(5 * time.Second)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	c.observeQuery(100, 0x1234, t0)

	// A response arriving after the TTL has elapsed finds nothing: the
	// query was evicted as stale, not held onto forever.
	_, ok := c.observeResponse(100, 0x1234, t0.Add(10*time.Second))
	if ok {
		t.Error("observeResponse() ok = true for a response after the TTL elapsed, want false")
	}
}

func TestQueryCorrelator_EvictionBoundsMapSize(t *testing.T) {
	c := newQueryCorrelator(time.Second)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 1000; i++ {
		c.observeQuery(100, uint16(i), t0)
	}
	if len(c.pending) != 1000 {
		t.Fatalf("len(pending) = %d, want 1000 before any eviction", len(c.pending))
	}

	// Any call past the TTL sweeps every stale entry, regardless of
	// which key triggered it.
	c.observeQuery(100, 9999, t0.Add(2*time.Second))

	if len(c.pending) != 1 {
		t.Errorf("len(pending) = %d, want 1 (only the just-added entry) after the sweep", len(c.pending))
	}
}
