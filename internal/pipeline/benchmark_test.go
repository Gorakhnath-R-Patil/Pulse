package pipeline_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/pipeline"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// BenchmarkPipeline_Throughput measures end-to-end pipeline throughput —
// read, queue, worker dispatch, and process — for a fast in-memory
// source and a no-op processor. This isolates the pipeline's own
// concurrency overhead from any real capture/decode cost (measured
// separately per domain, e.g. internal/socket's BenchmarkToEvent); see
// docs/design/event-pipeline.md's Performance section for what this
// number does and doesn't claim.
//
// Run with: go test ./internal/pipeline/... -bench=BenchmarkPipeline_Throughput -benchmem
func BenchmarkPipeline_Throughput(b *testing.B) {
	events := make([]model.Event, b.N)
	for i := range events {
		events[i] = testEvent(i)
	}
	src := &sliceSource{events: events, terminalErr: errors.New("benchmark source exhausted")}
	proc := &countingProcessor{}

	p := pipeline.New(pipeline.Config{Name: "bench", Workers: 4, QueueSize: 256}, src, discardLogger(), proc)

	b.ReportAllocs()
	b.ResetTimer()
	p.Run(context.Background())
	b.StopTimer()

	if got := proc.count(); got != b.N {
		b.Fatalf("processed %d events, want %d", got, b.N)
	}
}
