// Package pipeline implements Pulse's userspace event processing
// pipeline: read from a source, hand each event to a pool of workers,
// each of which runs it through one or more processors — with a
// bounded queue between reading and processing that applies real
// backpressure (the reader blocks, rather than dropping events or
// growing memory unboundedly, once the queue is full) and a graceful
// shutdown that lets in-flight work finish rather than abandoning it.
//
// This generalizes what internal/agent's process/network/socket wiring
// each implemented separately through Day 06: read a domain-specific
// event, normalize it, do something with it. Decoding and normalizing
// stay in each domain package (internal/process, internal/network,
// internal/socket) — see EventSource's doc comment for how they're
// adapted into this package's generic shape — because that logic is
// genuinely domain-specific; only the concurrency machinery around it
// is shared.
package pipeline

import (
	"context"
	"log/slog"
	"sync"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// EventSource produces normalized telemetry events, one at a time,
// blocking until the next one is available. It is the same shape as
// internal/ebpf's Loader.Read and friends, but returning an already-
// normalized pkg/model.Event rather than a domain-specific type — each
// domain package (internal/process, internal/network, internal/socket)
// is expected to provide a small adapter satisfying this interface by
// wrapping its own Loader and calling its own ToEvent, keeping decode
// and normalize logic in the package that owns it rather than moving it
// here.
type EventSource interface {
	Read() (model.Event, error)
}

// EventProcessor does something with a normalized event: log it today;
// later days may enrich, correlate, export, or store it. Process must
// respect ctx — the pipeline's graceful shutdown waits for in-flight
// Process calls to return, so a processor that ignores ctx and blocks
// forever will make shutdown hang forever too. Returning an error
// reports a problem processing this one event; it does not stop the
// pipeline or affect other events.
type EventProcessor interface {
	Process(ctx context.Context, event model.Event) error
}

// Config controls a Pipeline's concurrency.
type Config struct {
	// Name identifies this pipeline in its own log lines, so multiple
	// pipelines running in the same process (as internal/agent runs
	// one per capability) are distinguishable.
	Name string

	// Workers is the number of goroutines concurrently pulling events
	// off the queue and running them through every processor. Values
	// less than 1 are treated as 1.
	Workers int

	// QueueSize bounds how many decoded events may be waiting for a
	// free worker before the reader blocks. Values less than 1 are
	// treated as 1.
	QueueSize int
}

// Pipeline reads events from a Source and runs each one through every
// Processor, using Config's Workers and QueueSize to bound concurrency
// and memory.
type Pipeline struct {
	cfg        Config
	source     EventSource
	processors []EventProcessor
	logger     *slog.Logger

	queue chan model.Event
}

// New constructs a Pipeline. It does not start reading — call Run.
func New(cfg Config, source EventSource, logger *slog.Logger, processors ...EventProcessor) *Pipeline {
	if cfg.Workers < 1 {
		cfg.Workers = 1
	}
	if cfg.QueueSize < 1 {
		cfg.QueueSize = 1
	}
	return &Pipeline{
		cfg:        cfg,
		source:     source,
		processors: processors,
		logger:     logger,
		queue:      make(chan model.Event, cfg.QueueSize),
	}
}

// Run drains source into the bounded queue and fans it out across
// cfg.Workers worker goroutines, each running every processor over each
// event in turn. It blocks until source.Read fails (the expected
// outcome of the caller closing whatever loader backs source once ctx
// is done — see internal/agent) or ctx is canceled while the reader is
// blocked applying backpressure, then waits for every worker to finish
// its current event before returning.
func (p *Pipeline) Run(ctx context.Context) {
	var workers sync.WaitGroup
	workers.Add(p.cfg.Workers)
	for range p.cfg.Workers {
		go func() {
			defer workers.Done()
			p.work(ctx)
		}()
	}

	p.read(ctx)
	workers.Wait()
}

// read pulls events from source and hands them to the queue until
// source.Read fails or ctx is canceled, then closes the queue so
// workers can drain what remains and stop.
func (p *Pipeline) read(ctx context.Context) {
	defer close(p.queue)

	for {
		event, err := p.source.Read()
		if err != nil {
			if ctx.Err() == nil {
				p.logger.Warn("pipeline read failed", "pipeline", p.cfg.Name, "error", err)
			}
			return
		}

		select {
		case p.queue <- event: // backpressure: blocks while the queue is full
		case <-ctx.Done():
			return
		}
	}
}

// work runs every processor over each event the queue delivers, until
// the queue is closed (by read, once it stops) and drained.
func (p *Pipeline) work(ctx context.Context) {
	for event := range p.queue {
		for _, proc := range p.processors {
			if err := proc.Process(ctx, event); err != nil {
				p.logger.Warn("pipeline processor error", "pipeline", p.cfg.Name, "error", err)
			}
		}
	}
}
