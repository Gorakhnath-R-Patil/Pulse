// Package storage writes pkg/model.Event values consumed by
// pulse-collector into a real ClickHouse table, batched for
// ClickHouse's own well-documented preference for infrequent, large
// inserts over frequent, small ones. See
// docs/design/clickhouse-storage.md.
package storage

import (
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// appendEvent appends one model.Event to batch as a row, in exactly
// the column order eventsDDL declares. Fields absent from an event
// (e.g. Process on a non-process event, Network on a non-network one)
// are written as their zero value — ClickHouse has no concept of a Go
// nil pointer, and a zero PID/empty string/zero port is already how
// this project's own JSON encoding (pkg/model.Marshal) represents an
// absent optional field, so this doesn't introduce a new convention.
// e.Timestamp is converted to UTC before writing: eventsDDL declares
// the column DateTime64(9) (nanosecond precision, no stored timezone),
// so every row needs to already agree on a timezone before it's
// written, not rely on ClickHouse to reconcile one later.
func appendEvent(batch driver.Batch, e model.Event) error {
	var pid uint32
	var command, executable, containerID, podUID string
	if e.Process != nil {
		pid = uint32(e.Process.PID)
		command = e.Process.Command
		executable = e.Process.Executable
		if e.Process.Container != nil {
			containerID = e.Process.Container.ID
			podUID = e.Process.Container.PodUID
		}
	}

	var srcAddr, dstAddr string
	var srcPort, dstPort uint16
	var bytesSent, bytesReceived uint64
	if e.Network != nil {
		srcAddr = e.Network.Source.Address
		srcPort = e.Network.Source.Port
		dstAddr = e.Network.Destination.Address
		dstPort = e.Network.Destination.Port
		bytesSent = e.Network.BytesSent
		bytesReceived = e.Network.BytesReceived
	}

	attrs := e.Attributes
	if attrs == nil {
		attrs = map[string]string{}
	}

	return batch.Append(
		e.ID,
		e.Type,
		e.Timestamp.UTC(),
		e.Host,
		pid,
		command,
		executable,
		containerID,
		podUID,
		srcAddr,
		srcPort,
		dstAddr,
		dstPort,
		bytesSent,
		bytesReceived,
		attrs,
	)
}
