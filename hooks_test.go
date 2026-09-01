package featurelayer

import (
	"context"
	"testing"
)

func TestDecisionHooks(t *testing.T) {
	var events []DecisionEvent
	e, _, _ := testEngine(t, WithDecisionHook(func(ev DecisionEvent) { events = append(events, ev) }))
	ctx := context.Background()

	e.Evaluate(ctx, "editor.pro", EvalContext{TenantID: "tenant-1"}) // has a prerequisite: still ONE event
	if len(events) != 1 || events[0].Op != OpEvaluate || events[0].Decision.Reason != ReasonPrerequisite {
		t.Fatalf("events: %+v", events)
	}
	if events[0].Elapsed < 0 {
		t.Error("elapsed must be measured")
	}
	e.IsEnabled(ctx, "plain.feature", EvalContext{TenantID: "acme"})
	if len(events) != 2 || events[1].Op != OpEvaluate {
		t.Fatalf("IsEnabled fires evaluate: %+v", events)
	}
	if _, err := e.Consume(ctx, "api.calls", EvalContext{TenantID: "acme"}, 1); err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[2].Op != OpConsume {
		t.Fatalf("consume event: %+v", events)
	}
	if _, err := e.Usage(ctx, "api.calls", EvalContext{TenantID: "acme"}); err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 || events[3].Op != OpUsage {
		t.Fatalf("usage event: %+v", events)
	}
}

func TestMultipleHooksInOrder(t *testing.T) {
	var order []int
	e, _, _ := testEngine(t,
		WithDecisionHook(func(DecisionEvent) { order = append(order, 1) }),
		WithDecisionHook(func(DecisionEvent) { order = append(order, 2) }),
	)
	e.Evaluate(context.Background(), "plain.feature", EvalContext{TenantID: "acme"})
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Errorf("registration order: %v", order)
	}
}

func TestApplyHook(t *testing.T) {
	var got []ApplyEvent
	e, _, _ := testEngine(t, WithApplyHook(func(ev ApplyEvent) { got = append(got, ev) }))
	next, err := NewSnapshot(fullTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	prev := e.Snapshot()
	e.Apply(next)
	if len(got) != 1 || got[0].Prev != prev || got[0].Next != next || got[0].At.IsZero() {
		t.Errorf("apply event: %+v", got)
	}
	if e.Snapshot() != next {
		t.Error("snapshot swapped")
	}
}
