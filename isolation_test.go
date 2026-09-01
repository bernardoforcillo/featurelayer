package featurelayer

import (
	"context"
	"testing"
	"time"

	"github.com/bernardoforcillo/featurelayer/entitlement"
)

// assertDecision compares the fields of a Decision that determine its
// outcome. Decision.Entitlement and Decision.Usage are pointers that
// are freshly allocated on every call (see sumLimits/usageInfo), so a
// plain != comparison on Decision would fail even when nothing leaked;
// comparing the meaningful fields avoids that false positive.
func assertDecision(t *testing.T, name string, before, after Decision) {
	t.Helper()
	if before.Enabled != after.Enabled || before.Reason != after.Reason || before.Detail != after.Detail {
		t.Errorf("%s: decision changed after a mutation that must not reach the snapshot\n before: enabled=%v reason=%q detail=%q\n after:  enabled=%v reason=%q detail=%q",
			name, before.Enabled, before.Reason, before.Detail, after.Enabled, after.Reason, after.Detail)
	}
}

// TestSnapshotIsolatedFromConfigMutation reproduces the reviewer's
// empirical finding: after NewSnapshot, mutating the Config used to
// build it (at any nesting depth) must never change a live engine's
// decisions. It also covers the return boundary: mutating what a
// Snapshot accessor hands back must not reach the snapshot either.
func TestSnapshotIsolatedFromConfigMutation(t *testing.T) {
	cfg := fullTestConfig()
	snap, err := NewSnapshot(cfg)
	if err != nil {
		t.Fatal(err)
	}

	subs := entitlement.NewMemSubscriptions()
	subs.Set(entitlement.Subscription{TenantID: "acme", Plan: "pro", AddOns: []entitlement.AddOnID{"extra-calls"}})
	usage := entitlement.NewMemUsage()
	e := New(snap, WithSubscriptions(subs), WithUsage(usage), WithClock(func() time.Time { return tNow }))
	ctx := context.Background()
	ec := EvalContext{TenantID: "acme"}

	// Record decisions before any mutation. "acme" is in the
	// beta-testers segment, so export.csv is on via the "beta" flag
	// rule; the summed api.calls limit is 1000 (pro) + 500 (add-on).
	flagBefore := e.Evaluate(ctx, "export.csv", ec)
	if !flagBefore.Enabled || flagBefore.Reason != ReasonFlagRule {
		t.Fatalf("precondition: export.csv decision = %+v", flagBefore)
	}
	usageBefore, err := e.Usage(ctx, "api.calls", ec)
	if err != nil {
		t.Fatal(err)
	}
	if usageBefore.Usage == nil || usageBefore.Usage.Max != 1500 {
		t.Fatalf("precondition: api.calls usage = %+v", usageBefore.Usage)
	}

	// --- Ingest boundary: deeply mutate cfg after NewSnapshot returned. ---
	cfg.Flags[0].Rules[0].Serve.On = false                                       // would flip export.csv off for acme
	cfg.Plans[1].Entitlements[1].Limit.Max = 0                                   // would zero acme's api.calls limit
	cfg.Features[0].DependsOn = append(cfg.Features[0].DependsOn, "nonexistent") // would break export.csv's prerequisites

	flagAfterCfgMutation := e.Evaluate(ctx, "export.csv", ec)
	assertDecision(t, "cfg mutation (flag rule)", flagBefore, flagAfterCfgMutation)

	usageAfterCfgMutation, err := e.Usage(ctx, "api.calls", ec)
	if err != nil {
		t.Fatal(err)
	}
	if usageAfterCfgMutation.Usage == nil || usageAfterCfgMutation.Usage.Max != usageBefore.Usage.Max {
		t.Errorf("cfg mutation leaked into plan/add-on limit: before max=%d after max=%+v",
			usageBefore.Usage.Max, usageAfterCfgMutation.Usage)
	}

	// --- Return boundary: mutate what accessors handed back. ---
	p := snap.PlanEntitlements("pro")
	for i := range p {
		if p[i].Feature == "api.calls" && p[i].Limit != nil {
			p[i].Limit.Max = 0
		}
	}
	f, ok := snap.Flag("export.csv")
	if !ok {
		t.Fatal("Flag(export.csv) not found")
	}
	if len(f.Rules) == 0 {
		t.Fatal("Flag(export.csv) has no rules")
	}
	f.Rules[0].Serve.On = false

	flagAfterAccessorMutation := e.Evaluate(ctx, "export.csv", ec)
	assertDecision(t, "Flag() mutation", flagBefore, flagAfterAccessorMutation)

	usageAfterAccessorMutation, err := e.Usage(ctx, "api.calls", ec)
	if err != nil {
		t.Fatal(err)
	}
	if usageAfterAccessorMutation.Usage == nil || usageAfterAccessorMutation.Usage.Max != usageBefore.Usage.Max {
		t.Errorf("PlanEntitlements() mutation leaked back into snapshot: before max=%d after=%+v",
			usageBefore.Usage.Max, usageAfterAccessorMutation.Usage)
	}

	// --- Accessors must also be isolated from each other: two calls
	// must not share backing arrays or Limit pointers.
	p1 := snap.PlanEntitlements("pro")
	p2 := snap.PlanEntitlements("pro")
	for i := range p1 {
		if p1[i].Limit != nil && p2[i].Limit != nil && p1[i].Limit == p2[i].Limit {
			t.Errorf("PlanEntitlements: two calls share the same *Limit at index %d", i)
		}
	}
	f1, _ := snap.Flag("export.csv")
	f2, _ := snap.Flag("export.csv")
	if len(f1.Rules) > 0 && len(f2.Rules) > 0 {
		f1.Rules[0].Serve.On = false
		if !f2.Rules[0].Serve.On {
			t.Error("Flag: two calls share the Rules backing array")
		}
	}
}
