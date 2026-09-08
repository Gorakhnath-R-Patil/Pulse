package storage

import (
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/column"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// fakeBatch is a driver.Batch test double that only really implements
// Append — capturing exactly what appendEvent hands it — so
// appendEvent's field-mapping logic can be tested deterministically
// without a real ClickHouse connection, unlike the round-trip
// integration tests in writer_integration_test.go. Every other method
// exists only to satisfy the interface.
type fakeBatch struct {
	rows [][]any
}

func (f *fakeBatch) Append(v ...any) error {
	f.rows = append(f.rows, v)
	return nil
}
func (f *fakeBatch) AppendStruct(any) error        { return nil }
func (f *fakeBatch) Column(int) driver.BatchColumn { return nil }
func (f *fakeBatch) Flush() error                  { return nil }
func (f *fakeBatch) Send() error                   { return nil }
func (f *fakeBatch) Abort() error                  { return nil }
func (f *fakeBatch) IsSent() bool                  { return false }
func (f *fakeBatch) Rows() int                     { return len(f.rows) }
func (f *fakeBatch) Columns() []column.Interface   { return nil }
func (f *fakeBatch) Close() error                  { return nil }

func TestAppendEvent_FullEventMapsEveryColumn(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	event := model.Event{
		ID:        "evt-1",
		Type:      "network.connect",
		Timestamp: ts,
		Host:      "pulse-node-1",
		Process: &model.Process{
			PID:        100,
			Command:    "curl",
			Executable: "/usr/bin/curl",
			Container: &model.Container{
				ID:     "abc123",
				PodUID: "pod-uid-1",
			},
		},
		Network: &model.Network{
			Source:        model.Endpoint{Address: "10.0.0.1", Port: 5000},
			Destination:   model.Endpoint{Address: "10.0.0.2", Port: 443},
			BytesSent:     100,
			BytesReceived: 200,
		},
		Attributes: map[string]string{"key": "value"},
	}

	batch := &fakeBatch{}
	if err := appendEvent(batch, event); err != nil {
		t.Fatalf("appendEvent() returned error: %v", err)
	}
	if len(batch.rows) != 1 {
		t.Fatalf("len(batch.rows) = %d, want 1", len(batch.rows))
	}

	row := batch.rows[0]
	want := []any{
		"evt-1", "network.connect", ts, "pulse-node-1",
		uint32(100), "curl", "/usr/bin/curl", "abc123", "pod-uid-1",
		"10.0.0.1", uint16(5000), "10.0.0.2", uint16(443),
		uint64(100), uint64(200),
		map[string]string{"key": "value"},
	}
	if len(row) != len(want) {
		t.Fatalf("len(row) = %d, want %d", len(row), len(want))
	}
	for i := range want {
		if !equalCell(row[i], want[i]) {
			t.Errorf("row[%d] = %#v, want %#v", i, row[i], want[i])
		}
	}
}

func TestAppendEvent_MinimalEventZeroesAbsentFields(t *testing.T) {
	event := model.Event{
		ID:        "evt-2",
		Type:      "process.start",
		Timestamp: time.Now(),
		Host:      "pulse-node-1",
		// Process, Network, Attributes all left nil.
	}

	batch := &fakeBatch{}
	if err := appendEvent(batch, event); err != nil {
		t.Fatalf("appendEvent() returned error: %v", err)
	}
	row := batch.rows[0]

	if pid := row[4]; pid != uint32(0) {
		t.Errorf("pid = %#v, want 0", pid)
	}
	if command := row[5]; command != "" {
		t.Errorf("command = %#v, want empty", command)
	}
	if attrs := row[15]; !equalCell(attrs, map[string]string{}) {
		t.Errorf("attributes = %#v, want empty map, not nil", attrs)
	}
}

// equalCell compares two column values for the narrow set of types
// appendEvent ever produces — avoiding a reflect.DeepEqual dependency
// for what's otherwise a handful of comparable scalar types plus one
// map.
func equalCell(a, b any) bool {
	am, aok := a.(map[string]string)
	bm, bok := b.(map[string]string)
	if aok || bok {
		if !aok || !bok || len(am) != len(bm) {
			return false
		}
		for k, v := range am {
			if bm[k] != v {
				return false
			}
		}
		return true
	}
	return a == b
}
