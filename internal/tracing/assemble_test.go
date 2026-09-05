package tracing_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Gorakhnath-R-Patil/Pulse/internal/tracing"
	"github.com/Gorakhnath-R-Patil/Pulse/pkg/model"
)

func spanAt(traceID model.TraceID, id, parent model.SpanID, name string, start time.Time) model.Span {
	return model.Span{
		TraceID:      traceID,
		SpanID:       id,
		ParentSpanID: parent,
		Name:         name,
		Service:      "test-service",
		StartTime:    start,
	}
}

func TestAssembleTrace_LinearChain(t *testing.T) {
	traceID := model.NewTraceID()
	t0 := time.Now()
	root := spanAt(traceID, "root", "", "api.request", t0)
	child := spanAt(traceID, "child", "root", "order.process", t0.Add(time.Millisecond))
	grandchild := spanAt(traceID, "grandchild", "child", "payment.charge", t0.Add(2*time.Millisecond))

	trace, err := tracing.AssembleTrace([]model.Span{grandchild, root, child}) // deliberately out of order
	if err != nil {
		t.Fatalf("AssembleTrace() returned error: %v", err)
	}

	if len(trace.Roots) != 1 || trace.Roots[0].SpanID != "root" {
		t.Fatalf("Roots = %v, want exactly [root]", spanIDs(trace.Roots))
	}
	if len(trace.Orphans) != 0 {
		t.Errorf("Orphans = %v, want none", spanIDs(trace.Orphans))
	}

	rootNode := trace.Spans["root"]
	if len(rootNode.Children) != 1 || rootNode.Children[0].SpanID != "child" {
		t.Fatalf("root's Children = %v, want exactly [child]", spanIDs(rootNode.Children))
	}
	childNode := trace.Spans["child"]
	if len(childNode.Children) != 1 || childNode.Children[0].SpanID != "grandchild" {
		t.Fatalf("child's Children = %v, want exactly [grandchild]", spanIDs(childNode.Children))
	}
	if len(trace.Spans["grandchild"].Children) != 0 {
		t.Error("grandchild has children, want none (it's a leaf)")
	}
}

func TestAssembleTrace_MultipleChildrenOrderedByStartTime(t *testing.T) {
	traceID := model.NewTraceID()
	t0 := time.Now()
	root := spanAt(traceID, "root", "", "api.request", t0)
	second := spanAt(traceID, "second", "root", "b", t0.Add(2*time.Millisecond))
	first := spanAt(traceID, "first", "root", "a", t0.Add(time.Millisecond))

	trace, err := tracing.AssembleTrace([]model.Span{root, second, first})
	if err != nil {
		t.Fatalf("AssembleTrace() returned error: %v", err)
	}

	children := trace.Spans["root"].Children
	if len(children) != 2 {
		t.Fatalf("len(Children) = %d, want 2", len(children))
	}
	if children[0].SpanID != "first" || children[1].SpanID != "second" {
		t.Errorf("Children order = %v, want [first second] (by StartTime)", spanIDs(children))
	}
}

func TestAssembleTrace_Orphan(t *testing.T) {
	traceID := model.NewTraceID()
	t0 := time.Now()
	// "child"'s parent, "missing-root", was never included.
	child := spanAt(traceID, "child", "missing-root", "order.process", t0)

	trace, err := tracing.AssembleTrace([]model.Span{child})
	if err != nil {
		t.Fatalf("AssembleTrace() returned error: %v", err)
	}

	if len(trace.Roots) != 0 {
		t.Errorf("Roots = %v, want none (child has a ParentSpanID, just an absent one)", spanIDs(trace.Roots))
	}
	if len(trace.Orphans) != 1 || trace.Orphans[0].SpanID != "child" {
		t.Fatalf("Orphans = %v, want exactly [child]", spanIDs(trace.Orphans))
	}
}

func TestAssembleTrace_MultipleRoots(t *testing.T) {
	traceID := model.NewTraceID()
	t0 := time.Now()
	second := spanAt(traceID, "second", "", "b", t0.Add(time.Millisecond))
	first := spanAt(traceID, "first", "", "a", t0)

	trace, err := tracing.AssembleTrace([]model.Span{second, first})
	if err != nil {
		t.Fatalf("AssembleTrace() returned error: %v", err)
	}

	if len(trace.Roots) != 2 {
		t.Fatalf("len(Roots) = %d, want 2 (multiple roots are tolerated, not an error)", len(trace.Roots))
	}
	if trace.Roots[0].SpanID != "first" || trace.Roots[1].SpanID != "second" {
		t.Errorf("Roots order = %v, want [first second] (by StartTime)", spanIDs(trace.Roots))
	}
}

func TestAssembleTrace_SingleSpanIsItsOwnRoot(t *testing.T) {
	traceID := model.NewTraceID()
	only := spanAt(traceID, "only", "", "a", time.Now())

	trace, err := tracing.AssembleTrace([]model.Span{only})
	if err != nil {
		t.Fatalf("AssembleTrace() returned error: %v", err)
	}
	if len(trace.Roots) != 1 || trace.Roots[0].SpanID != "only" {
		t.Fatalf("Roots = %v, want exactly [only]", spanIDs(trace.Roots))
	}
	if len(trace.Spans["only"].Children) != 0 {
		t.Error("the only span has children, want none")
	}
}

func TestAssembleTrace_Empty(t *testing.T) {
	_, err := tracing.AssembleTrace(nil)
	if !errors.Is(err, tracing.ErrEmptyTrace) {
		t.Fatalf("AssembleTrace(nil) error = %v, want it to wrap ErrEmptyTrace", err)
	}
}

func TestAssembleTrace_MixedTraceIDs(t *testing.T) {
	t0 := time.Now()
	a := spanAt(model.NewTraceID(), "a", "", "x", t0)
	b := spanAt(model.NewTraceID(), "b", "", "y", t0)

	_, err := tracing.AssembleTrace([]model.Span{a, b})
	if !errors.Is(err, tracing.ErrMixedTraceIDs) {
		t.Fatalf("AssembleTrace() error = %v, want it to wrap ErrMixedTraceIDs", err)
	}
}

func spanIDs(nodes []*tracing.SpanNode) []model.SpanID {
	ids := make([]model.SpanID, len(nodes))
	for i, n := range nodes {
		ids[i] = n.SpanID
	}
	return ids
}
