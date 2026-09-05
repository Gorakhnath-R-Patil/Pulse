package tracing

import (
	"fmt"
	"sort"

	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

// SpanNode is one span plus links to its children, as assembled by
// AssembleTrace.
type SpanNode struct {
	model.Span
	Children []*SpanNode
}

// Trace is a fully or partially assembled distributed trace: every
// span sharing one TraceID, organized by parent-child relationship.
type Trace struct {
	ID model.TraceID

	// Spans holds every span AssembleTrace was given, keyed by its own
	// SpanID, each already linked to whatever children were found
	// among the same input.
	Spans map[model.SpanID]*SpanNode

	// Roots holds every span with no ParentSpanID at all, ordered by
	// StartTime. Ordinarily exactly one; more than one is tolerated,
	// not an error — see AssembleTrace's doc comment.
	Roots []*SpanNode

	// Orphans holds spans whose ParentSpanID doesn't match any span in
	// Spans, ordered by StartTime: present in the input, but missing
	// the piece of the trace that would explain where they attach. A
	// non-empty Orphans means the trace is incomplete, not necessarily
	// wrong.
	Orphans []*SpanNode
}

// AssembleTrace organizes spans — which must all share one TraceID —
// into a tree by ParentSpanID. Each node's Children, and Trace's Roots
// and Orphans, are ordered by StartTime.
//
// It does not require a complete trace, and does not itself decide
// which raw telemetry events belong together (that's a correlation
// concern, not this function's) — it only organizes whatever spans
// it's given. More than one span with no ParentSpanID is tolerated
// (all become Roots, not an error): a partial trace assembled from
// incomplete data may genuinely have more than one span nobody else
// claims as a child, and treating that as a hard error would make this
// function far less useful for exactly the incomplete-trace case it
// needs to handle gracefully.
func AssembleTrace(spans []model.Span) (Trace, error) {
	if len(spans) == 0 {
		return Trace{}, ErrEmptyTrace
	}

	traceID := spans[0].TraceID
	nodes := make(map[model.SpanID]*SpanNode, len(spans))
	for _, s := range spans {
		if s.TraceID != traceID {
			return Trace{}, fmt.Errorf("%w: span %s has trace ID %s, want %s", ErrMixedTraceIDs, s.SpanID, s.TraceID, traceID)
		}
		nodes[s.SpanID] = &SpanNode{Span: s}
	}

	trace := Trace{ID: traceID, Spans: nodes}

	for _, node := range nodes {
		if node.ParentSpanID == "" {
			trace.Roots = append(trace.Roots, node)
			continue
		}
		parent, ok := nodes[node.ParentSpanID]
		if !ok {
			trace.Orphans = append(trace.Orphans, node)
			continue
		}
		parent.Children = append(parent.Children, node)
	}

	sortByStartTime(trace.Roots)
	sortByStartTime(trace.Orphans)
	for _, node := range nodes {
		sortByStartTime(node.Children)
	}

	return trace, nil
}

func sortByStartTime(nodes []*SpanNode) {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].StartTime.Before(nodes[j].StartTime)
	})
}
