package flags

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func TestActive(t *testing.T) {
	if on, out := (&Flag{Enabled: false}).Active(t0); on || out.Reason != ReasonOff {
		t.Errorf("kill switch: on=%v reason=%q", on, out.Reason)
	}
	w := &Window{From: t0.Add(time.Hour), Until: t0.Add(2 * time.Hour)}
	if on, out := (&Flag{Enabled: true, Window: w}).Active(t0); on || out.Reason != ReasonWindow {
		t.Errorf("before window: on=%v reason=%q", on, out.Reason)
	}
	if on, _ := (&Flag{Enabled: true, Window: w}).Active(t0.Add(time.Hour)); !on {
		t.Error("From is inclusive")
	}
	if on, out := (&Flag{Enabled: true, Window: w}).Active(t0.Add(2 * time.Hour)); on || out.Reason != ReasonWindow {
		t.Errorf("Until is exclusive: on=%v reason=%q", on, out.Reason)
	}
	if on, _ := (&Flag{Enabled: true}).Active(t0); !on {
		t.Error("no window: active")
	}
	if on, _ := (&Flag{Enabled: true, Window: &Window{From: t0.Add(-time.Hour)}}).Active(t0); !on {
		t.Error("open-ended window: active")
	}
}

func TestServeRulesAndDefault(t *testing.T) {
	f := &Flag{
		Feature: "export.csv", Enabled: true,
		Rules: []Rule{
			{Name: "block-legacy", Conditions: []Condition{{Attribute: "plan", Op: Eq, Value: "legacy"}}, Serve: Serve{On: false}},
			{Name: "pro-on", Conditions: []Condition{{Attribute: "plan", Op: Eq, Value: "pro"}}, Serve: Serve{On: true}},
		},
		Default: Serve{On: false},
	}
	ev := NewEvaluator()
	if out := ev.Serve(f, map[string]any{"plan": "legacy"}); out.On || out.Reason != ReasonRule || out.Detail != "block-legacy" {
		t.Errorf("first match wins: %+v", out)
	}
	if out := ev.Serve(f, map[string]any{"plan": "pro"}); !out.On || out.Reason != ReasonRule || out.Detail != "pro-on" {
		t.Errorf("rule on: %+v", out)
	}
	if out := ev.Serve(f, map[string]any{"plan": "free"}); out.On || out.Reason != ReasonDefault {
		t.Errorf("default off: %+v", out)
	}
	empty := &Flag{Feature: "x", Enabled: true, Rules: []Rule{{Name: "all", Serve: Serve{On: true}}}}
	if out := ev.Serve(empty, nil); !out.On || out.Detail != "all" {
		t.Errorf("empty conditions match everyone: %+v", out)
	}
}

func TestServeRollout(t *testing.T) {
	ev := NewEvaluator()
	// Boolean 20% rollout, default bucketBy tenant, seed = feature key.
	// Golden buckets: tenant-1=18.31 (in), tenant-3=82.53 (out).
	f := &Flag{Feature: "export.csv", Enabled: true, Default: Serve{Rollout: Percent(20)}}
	if out := ev.Serve(f, map[string]any{"tenant": "tenant-1"}); !out.On || out.Reason != ReasonRollout {
		t.Errorf("tenant-1 in 20%%: %+v", out)
	}
	if out := ev.Serve(f, map[string]any{"tenant": "tenant-3"}); out.On || out.Reason != ReasonRollout {
		t.Errorf("tenant-3 out of 20%%: %+v", out)
	}
	if out := ev.Serve(f, map[string]any{}); out.On || out.Detail != "no bucket attribute" {
		t.Errorf("missing bucket attribute: %+v", out)
	}
	// Multivariate: a=[0,50) b=[50,80) off=[80,100). Golden: tenant-1=18.31→a, acme=54.83→b, tenant-3=82.53→off.
	mv := &Flag{
		Feature: "export.csv", Enabled: true,
		Variants: []Variant{{Key: "a", Value: "A"}, {Key: "b", Value: "B"}},
		Default:  Serve{Rollout: &Rollout{Split: []Portion{{Variant: "a", Percent: 50}, {Variant: "b", Percent: 30}}}},
	}
	if out := ev.Serve(mv, map[string]any{"tenant": "tenant-1"}); !out.On || out.Variant == nil || out.Variant.Key != "a" {
		t.Errorf("tenant-1 → a: %+v", out)
	}
	if out := ev.Serve(mv, map[string]any{"tenant": "acme"}); !out.On || out.Variant == nil || out.Variant.Key != "b" {
		t.Errorf("acme → b: %+v", out)
	}
	if out := ev.Serve(mv, map[string]any{"tenant": "tenant-3"}); out.On {
		t.Errorf("tenant-3 → off: %+v", out)
	}
	// BucketBy override and rollout inside a rule.
	ruled := &Flag{
		Feature: "export.csv", Enabled: true,
		Rules:   []Rule{{Name: "users", Conditions: []Condition{{Attribute: "user", Op: Exists}}, Serve: Serve{Rollout: &Rollout{BucketBy: "user", Split: []Portion{{Percent: 20}}}}}},
		Default: Serve{On: false},
	}
	// bucketOf("export.csv","tenant-1")=18.31 → in; used here as a user id.
	if out := ev.Serve(ruled, map[string]any{"user": "tenant-1"}); !out.On || out.Reason != ReasonRollout || out.Detail != "users" {
		t.Errorf("rule rollout by user: %+v", out)
	}
}

func TestServeFixedVariant(t *testing.T) {
	ev := NewEvaluator()
	f := &Flag{
		Feature: "theme", Enabled: true,
		Variants: []Variant{{Key: "dark", Value: map[string]any{"bg": "#000"}}},
		Default:  Serve{On: true, Variant: "dark"},
	}
	out := ev.Serve(f, nil)
	if !out.On || out.Variant == nil || out.Variant.Key != "dark" {
		t.Errorf("fixed variant: %+v", out)
	}
	if out := ev.Evaluate(&Flag{Enabled: false}, nil, t0); out.On || out.Reason != ReasonOff {
		t.Errorf("Evaluate = Active + Serve: %+v", out)
	}
}
