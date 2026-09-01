package featurelayer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bernardoforcillo/featurelayer/catalog"
	"github.com/bernardoforcillo/featurelayer/entitlement"
)

func testEngine(t *testing.T, opts ...Option) (*Engine, *entitlement.MemSubscriptions, *entitlement.MemUsage) {
	t.Helper()
	snap, err := NewSnapshot(fullTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	subs := entitlement.NewMemSubscriptions()
	subs.Set(entitlement.Subscription{TenantID: "acme", Plan: "pro", AddOns: []entitlement.AddOnID{"extra-calls"}})
	subs.Set(entitlement.Subscription{TenantID: "tenant-1", Plan: "pro"})
	subs.Set(entitlement.Subscription{TenantID: "tenant-3", Plan: "pro"})
	subs.Set(entitlement.Subscription{TenantID: "freeloader", Plan: "free"})
	subs.Set(entitlement.Subscription{TenantID: "banned", Plan: "pro",
		Grants: []entitlement.Grant{{Feature: "export.csv", Deny: true, Reason: "abuse"}}})
	subs.Set(entitlement.Subscription{TenantID: "vip", Plan: "free",
		Grants: []entitlement.Grant{entitlement.Override("export.csv", nil, "deal")}})
	usage := entitlement.NewMemUsage()
	all := append([]Option{WithSubscriptions(subs), WithUsage(usage), WithClock(func() time.Time { return tNow })}, opts...)
	return New(snap, all...), subs, usage
}

func TestEvaluateMatrix(t *testing.T) {
	e, _, _ := testEngine(t)
	ctx := context.Background()
	tests := []struct {
		name    string
		key     catalog.Key
		ec      EvalContext
		enabled bool
		reason  Reason
		detail  string
	}{
		{"unknown feature", "nope", EvalContext{TenantID: "acme"}, false, ReasonUnknownFeature, ""},
		{"draft", "secret", EvalContext{TenantID: "acme"}, false, ReasonLifecycle, "draft"},
		{"retired", "gone", EvalContext{TenantID: "acme"}, false, ReasonLifecycle, "retired"},
		{"kill switch", "killed.feature", EvalContext{TenantID: "acme"}, false, ReasonFlagOff, ""},
		{"window", "windowed.feature", EvalContext{TenantID: "acme"}, false, ReasonFlagWindow, ""},
		{"segment rule on", "export.csv", EvalContext{TenantID: "acme"}, true, ReasonFlagRule, "beta"},
		{"rollout in", "export.csv", EvalContext{TenantID: "tenant-1"}, true, ReasonFlagRollout, ""},
		{"rollout out", "export.csv", EvalContext{TenantID: "tenant-3"}, false, ReasonFlagRollout, ""},
		{"not entitled by plan", "export.csv", EvalContext{TenantID: "freeloader"}, false, ReasonNotEntitled, ""},
		{"no subscription", "export.csv", EvalContext{TenantID: "ghost"}, false, ReasonNotEntitled, ""},
		{"no tenant", "export.csv", EvalContext{}, false, ReasonNotEntitled, "no tenant"},
		{"denied grant", "export.csv", EvalContext{TenantID: "banned"}, false, ReasonDenied, "abuse"},
		{"granted", "export.csv", EvalContext{TenantID: "vip"}, true, ReasonFlagRollout, ""}, // "vip" is not a golden bucket: skip enabled/reason, assert Entitlement.Kind below
		{"no flag", "old.widget", EvalContext{TenantID: "acme"}, true, ReasonNoFlag, ""},
		{"flag default", "plain.feature", EvalContext{TenantID: "acme"}, true, ReasonFlagDefault, ""},
		{"prerequisite off", "editor.pro", EvalContext{TenantID: "tenant-1"}, false, ReasonPrerequisite, "new-editor"},
	}
	for _, tt := range tests {
		d := e.Evaluate(ctx, tt.key, tt.ec)
		if tt.name == "granted" {
			if d.Entitlement == nil || d.Entitlement.Kind != entitlement.KindGrant {
				t.Errorf("%s: entitlement = %+v, want grant", tt.name, d.Entitlement)
			}
			continue
		}
		if d.Enabled != tt.enabled || d.Reason != tt.reason {
			t.Errorf("%s: enabled=%v reason=%q (want %v %q)", tt.name, d.Enabled, d.Reason, tt.enabled, tt.reason)
		}
		if tt.detail != "" && d.Detail != tt.detail {
			t.Errorf("%s: detail=%q want %q", tt.name, d.Detail, tt.detail)
		}
	}
	// deprecated is reported but enabled
	if d := e.Evaluate(ctx, "old.widget", EvalContext{TenantID: "acme"}); d.Lifecycle != catalog.Deprecated || !d.Enabled {
		t.Errorf("deprecated: %+v", d)
	}
	// entitlement source and summed limit surface on the decision
	if d := e.Evaluate(ctx, "api.calls", EvalContext{TenantID: "acme"}); d.Entitlement == nil ||
		d.Entitlement.Kind != entitlement.KindPlan || d.Entitlement.Limit == nil || d.Entitlement.Limit.Max != 1500 {
		t.Errorf("entitlement on decision: %+v", d.Entitlement)
	}
}

func TestDerivedAttributes(t *testing.T) {
	// The "pro-only-check" rule serves OFF when plan == "legacy". tenant-1
	// is NOT in the beta-testers segment, so a spoofed Attributes["plan"] =
	// "legacy" would match that rule — unless the derived value ("pro")
	// overrides it, in which case evaluation falls through to the default
	// 20% rollout (tenant-1 bucket 18.31 → on).
	e, _, _ := testEngine(t)
	ctx := context.Background()
	d := e.Evaluate(ctx, "export.csv", EvalContext{TenantID: "tenant-1", Attributes: map[string]any{"plan": "legacy"}})
	if !d.Enabled || d.Reason != ReasonFlagRollout {
		t.Errorf("derived plan must override caller attrs: %+v", d)
	}
}

func TestSpoofedIdentityAttributes(t *testing.T) {
	ctx := context.Background()

	// Anonymous context on a Free feature: a spoofed Attributes["tenant"]
	// must not flow into bucketing unguarded. "tenant-2" would land
	// in-bucket (0.42) on new-editor's 20% rollout if the spoof worked.
	t.Run("anonymous free feature rollout", func(t *testing.T) {
		e, _, _ := testEngine(t)
		d := e.Evaluate(ctx, "new-editor", EvalContext{Attributes: map[string]any{"tenant": "tenant-2"}})
		if d.Enabled || d.Reason != ReasonFlagRollout || d.Detail != "no bucket attribute" {
			t.Errorf("spoofed tenant on anonymous context: %+v", d)
		}
	})

	// Flags-only engine (no WithSubscriptions): a spoofed Attributes["tenant"]
	// must not let the caller impersonate a segment member (the "beta" rule
	// matches tenant "acme" via the beta-testers segment).
	t.Run("flags-only spoofed segment membership", func(t *testing.T) {
		snap, err := NewSnapshot(fullTestConfig())
		if err != nil {
			t.Fatal(err)
		}
		e := New(snap, WithClock(func() time.Time { return tNow }))
		d := e.Evaluate(ctx, "export.csv", EvalContext{Attributes: map[string]any{"tenant": "acme"}})
		if d.Enabled || d.Reason != ReasonFlagRollout || d.Detail != "no bucket attribute" {
			t.Errorf("spoofed tenant on flags-only engine: %+v", d)
		}
	})
}

type failingSubs struct{}

func (failingSubs) Subscription(context.Context, string) (*entitlement.Subscription, error) {
	return nil, errors.New("db down")
}

func TestStoreErrorFailsClosed(t *testing.T) {
	snap, err := NewSnapshot(fullTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	e := New(snap, WithSubscriptions(failingSubs{}), WithClock(func() time.Time { return tNow }))
	d := e.Evaluate(context.Background(), "export.csv", EvalContext{TenantID: "acme"})
	if d.Enabled || d.Reason != ReasonStoreError || d.Err == nil {
		t.Errorf("store error: %+v", d)
	}
}

func TestFlagsOnlyMode(t *testing.T) {
	// No SubscriptionStore configured → entitlement step skipped even for gated features.
	snap, err := NewSnapshot(fullTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	e := New(snap, WithClock(func() time.Time { return tNow }))
	d := e.Evaluate(context.Background(), "export.csv", EvalContext{TenantID: "acme"})
	if !d.Enabled || d.Entitlement != nil {
		t.Errorf("flags-only mode: %+v", d)
	}
}

func TestIsEnabledAndVariant(t *testing.T) {
	e, _, _ := testEngine(t)
	ctx := context.Background()
	if !e.IsEnabled(ctx, "plain.feature", EvalContext{TenantID: "acme"}) {
		t.Error("IsEnabled")
	}
	if _, ok := e.Variant(ctx, "plain.feature", EvalContext{TenantID: "acme"}); ok {
		t.Error("boolean flag has no variant")
	}
}
