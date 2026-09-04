package featurelayer

import (
	"context"
	"errors"
	"testing"
)

func TestContextRoundTrip(t *testing.T) {
	ctx := context.Background()
	if _, ok := FromContext(ctx); ok {
		t.Fatal("empty ctx must carry no EvalContext")
	}
	ec := EvalContext{TenantID: "acme", UserID: "u-1", Attributes: map[string]any{"principal_kind": "user"}}
	got, ok := FromContext(NewContext(ctx, ec))
	if !ok || got.TenantID != "acme" || got.UserID != "u-1" || got.Attributes["principal_kind"] != "user" {
		t.Errorf("FromContext = %+v, %v", got, ok)
	}
	// The last NewContext wins, as with any context value.
	got, _ = FromContext(NewContext(NewContext(ctx, ec), EvalContext{TenantID: "other"}))
	if got.TenantID != "other" {
		t.Errorf("inner value must win: %+v", got)
	}
}

func TestCtxVariantsDelegate(t *testing.T) {
	var events []DecisionEvent
	e, _, _ := testEngine(t, WithDecisionHook(func(ev DecisionEvent) { events = append(events, ev) }))
	ctx := NewContext(context.Background(), EvalContext{TenantID: "acme", UserID: "u-1"})

	if d := e.EvaluateCtx(ctx, "export.csv"); !d.Enabled || d.Reason != ReasonFlagRule {
		t.Errorf("EvaluateCtx: %+v", d)
	}
	if !e.IsEnabledCtx(ctx, "plain.feature") {
		t.Error("IsEnabledCtx must be true for the default-on flag")
	}
	d, err := e.ConsumeCtx(ctx, "ai.tokens", 4)
	if err != nil || !d.Enabled || d.Usage == nil || d.Usage.Used != 4 {
		t.Errorf("ConsumeCtx: %+v %v", d, err)
	}
	d, err = e.UsageCtx(ctx, "ai.tokens")
	if err != nil || d.Usage == nil || d.Usage.Used != 4 {
		t.Errorf("UsageCtx: %+v %v", d, err)
	}
	if len(events) != 4 || events[0].Context.TenantID != "acme" || events[2].Op != OpConsume || events[3].Op != OpUsage {
		t.Errorf("hooks see the ctx-derived EvalContext: %+v", events)
	}
}

func TestCtxVariantsFailClosedWithoutContext(t *testing.T) {
	var events []DecisionEvent
	e, _, usage := testEngine(t, WithDecisionHook(func(ev DecisionEvent) { events = append(events, ev) }))
	ctx := context.Background()

	d := e.EvaluateCtx(ctx, "plain.feature")
	if d.Enabled || d.Reason != ReasonNoContext || !errors.Is(d.Err, ErrNoEvalContext) || d.Feature != "plain.feature" {
		t.Errorf("EvaluateCtx without ctx: %+v", d)
	}
	if e.IsEnabledCtx(ctx, "plain.feature") {
		t.Error("IsEnabledCtx without ctx must be false")
	}
	d, err := e.ConsumeCtx(ctx, "api.calls", 1)
	if !errors.Is(err, ErrNoEvalContext) || d.Enabled || d.Reason != ReasonNoContext || d.Usage != nil {
		t.Errorf("ConsumeCtx without ctx: %+v %v", d, err)
	}
	d, err = e.UsageCtx(ctx, "api.calls")
	if !errors.Is(err, ErrNoEvalContext) || d.Enabled || d.Reason != ReasonNoContext {
		t.Errorf("UsageCtx without ctx: %+v %v", d, err)
	}
	// Nothing was charged anywhere.
	if got, _ := usage.Get(ctx, usageKeyFor("acme", "api.calls")); got != 0 {
		t.Errorf("no-context Consume charged a counter: %d", got)
	}
	// Every miss is observable through the decision hook.
	if len(events) != 4 {
		t.Fatalf("events = %d, want 4", len(events))
	}
	for i, ev := range events {
		if ev.Decision.Reason != ReasonNoContext {
			t.Errorf("events[%d].Reason = %q", i, ev.Decision.Reason)
		}
	}
	if events[2].Err == nil || events[0].Err != nil {
		t.Errorf("Evaluate reports the miss on the decision only, Consume also as Err: %+v", events)
	}
}
