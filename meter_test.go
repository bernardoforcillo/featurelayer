package featurelayer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bernardoforcillo/featurelayer/entitlement"
)

func TestConsume(t *testing.T) {
	e, subs, _ := testEngine(t)
	ctx := context.Background()
	ec := EvalContext{TenantID: "acme"} // pro + extra-calls → api.calls 1500/month

	d, err := e.Consume(ctx, "api.calls", ec, 100)
	if err != nil || !d.Enabled || d.Usage == nil {
		t.Fatalf("consume: %+v %v", d, err)
	}
	u := d.Usage
	if u.Used != 100 || u.Max != 1500 || u.Remaining != 1400 {
		t.Errorf("usage: %+v", u)
	}
	// calendar month period for tNow = 2026-09-15
	if u.Period != "2026-09-01T00:00:00Z" || !u.ResetsAt.Equal(d2("2026-10-01T00:00:00Z")) {
		t.Errorf("period: %+v", u)
	}

	// fill to the cap, then get refused
	if d, _ = e.Consume(ctx, "api.calls", ec, 1400); !d.Enabled {
		t.Fatalf("exact fit: %+v", d)
	}
	d, err = e.Consume(ctx, "api.calls", ec, 1)
	if err != nil {
		t.Fatal(err)
	}
	if d.Enabled || d.Reason != ReasonLimitReached || d.Usage.Used != 1500 || d.Usage.Remaining != 0 {
		t.Errorf("refusal: %+v usage=%+v", d, d.Usage)
	}

	// unlimited entitlement is still recorded
	d, err = e.Consume(ctx, "export.csv", EvalContext{TenantID: "acme"}, 1)
	if err != nil || !d.Enabled || d.Usage == nil || d.Usage.Max != -1 || d.Usage.Remaining != -1 {
		t.Errorf("unlimited: %+v %v", d.Usage, err)
	}

	// disabled decision consumes nothing and returns no error
	d, err = e.Consume(ctx, "export.csv", EvalContext{TenantID: "freeloader"}, 1)
	if err != nil || d.Enabled || d.Reason != ReasonNotEntitled || d.Usage != nil {
		t.Errorf("off decision: %+v %v", d, err)
	}

	// billing-anchored period key
	subs.Set(entitlement.Subscription{TenantID: "anchored", Plan: "pro",
		BillingAnchor: d2("2026-08-20T10:00:00Z")})
	d, err = e.Consume(ctx, "api.calls", EvalContext{TenantID: "anchored"}, 1)
	if err != nil || d.Usage == nil || d.Usage.Period != "2026-08-20T10:00:00Z" {
		t.Errorf("anchored period: %+v %v", d.Usage, err)
	}

	// misuse
	if _, err := e.Consume(ctx, "api.calls", ec, 0); !errors.Is(err, ErrInvalidDelta) {
		t.Errorf("zero delta: %v", err)
	}
	bare := New(e.Snapshot(), WithClock(func() time.Time { return tNow }))
	if _, err := bare.Consume(ctx, "api.calls", ec, 1); !errors.Is(err, ErrNoUsageStore) {
		t.Errorf("no usage store: %v", err)
	}
}

func TestUsageReadOnly(t *testing.T) {
	e, _, _ := testEngine(t)
	ctx := context.Background()
	ec := EvalContext{TenantID: "acme"}
	if _, err := e.Consume(ctx, "api.calls", ec, 7); err != nil {
		t.Fatal(err)
	}
	d, err := e.Usage(ctx, "api.calls", ec)
	if err != nil || d.Usage == nil || d.Usage.Used != 7 || d.Usage.Remaining != 1493 {
		t.Errorf("usage read: %+v %v", d.Usage, err)
	}
	again, err := e.Usage(ctx, "api.calls", ec)
	if err != nil || again.Usage.Used != 7 {
		t.Error("Usage must not consume")
	}
}

type failingUsage struct{}

func (failingUsage) Get(context.Context, entitlement.UsageKey) (int64, error) {
	return 0, errors.New("usage db down")
}
func (failingUsage) Increment(context.Context, entitlement.UsageKey, int64, int64) (int64, bool, error) {
	return 0, false, errors.New("usage db down")
}

func TestUsageStoreErrorSurface(t *testing.T) {
	e, _, _ := testEngine(t, WithUsage(failingUsage{}))
	ctx := context.Background()
	ec := EvalContext{TenantID: "acme"}
	d, err := e.Consume(ctx, "api.calls", ec, 1)
	if err == nil || d.Enabled || d.Reason != ReasonStoreError || d.Err == nil {
		t.Errorf("Consume store error: %+v %v", d, err)
	}
	d, err = e.Usage(ctx, "api.calls", ec)
	if err == nil || d.Enabled || d.Reason != ReasonStoreError || d.Err == nil {
		t.Errorf("Usage store error: %+v %v", d, err)
	}
}

func d2(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestConsumePerSubject(t *testing.T) {
	e, subs, usage := testEngine(t)
	ctx := context.Background()
	u1 := EvalContext{TenantID: "acme", UserID: "u-1"} // pro → ai.tokens 10/day per subject
	u2 := EvalContext{TenantID: "acme", UserID: "u-2"}

	d, err := e.Consume(ctx, "ai.tokens", u1, 10)
	if err != nil || !d.Enabled || d.Usage == nil || d.Usage.Used != 10 || d.Usage.Remaining != 0 {
		t.Fatalf("u-1 fills own counter: %+v %v", d, err)
	}
	d, err = e.Consume(ctx, "ai.tokens", u1, 1)
	if err != nil || d.Enabled || d.Reason != ReasonLimitReached || d.Usage.Used != 10 {
		t.Errorf("u-1 refused: %+v %v", d, err)
	}
	// u-2 has a counter of their own: the tenant's other user is untouched.
	d, err = e.Consume(ctx, "ai.tokens", u2, 3)
	if err != nil || !d.Enabled || d.Usage.Used != 3 || d.Usage.Remaining != 7 {
		t.Errorf("u-2 isolated: %+v %v", d.Usage, err)
	}
	// The counters are keyed by subject; the tenant-level key is untouched.
	period := entitlement.PeriodKey(entitlement.Day, time.Time{}, tNow)
	if got, _ := usage.Get(ctx, entitlement.UsageKey{Tenant: "acme", Feature: "ai.tokens", Period: period, Subject: "u-1"}); got != 10 {
		t.Errorf("u-1 key = %d", got)
	}
	if got, _ := usage.Get(ctx, entitlement.UsageKey{Tenant: "acme", Feature: "ai.tokens", Period: period}); got != 0 {
		t.Errorf("tenant key must stay unused, got %d", got)
	}
	// Usage reads the same counter.
	d, err = e.Usage(ctx, "ai.tokens", u2)
	if err != nil || d.Usage == nil || d.Usage.Used != 3 {
		t.Errorf("usage read u-2: %+v %v", d.Usage, err)
	}

	// No subject: fail closed, consume nothing, no error (a business "no").
	anon := EvalContext{TenantID: "acme"}
	d, err = e.Consume(ctx, "ai.tokens", anon, 1)
	if err != nil || d.Enabled || d.Reason != ReasonNotEntitled || d.Detail != "no subject" || d.Usage != nil {
		t.Errorf("no subject consume: %+v %v", d, err)
	}
	d, err = e.Usage(ctx, "ai.tokens", anon)
	if err != nil || d.Enabled || d.Reason != ReasonNotEntitled || d.Detail != "no subject" || d.Usage != nil {
		t.Errorf("no subject usage: %+v %v", d, err)
	}
	if got, _ := usage.Get(ctx, entitlement.UsageKey{Tenant: "acme", Feature: "ai.tokens", Period: period}); got != 0 {
		t.Errorf("no-subject call must not charge the tenant counter, got %d", got)
	}
	// Evaluate itself is unaffected: the scope only matters when metering.
	if d := e.Evaluate(ctx, "ai.tokens", anon); !d.Enabled {
		t.Errorf("Evaluate without subject: %+v", d)
	}

	// A grant carrying an unknown scope never passed snapshot validation:
	// fail closed rather than guess a counter.
	subs.Set(entitlement.Subscription{TenantID: "odd", Plan: "pro",
		Grants: []entitlement.Grant{entitlement.Override("ai.tokens", &entitlement.Limit{Max: 5, Per: "bogus"}, "typo")}})
	d, err = e.Consume(ctx, "ai.tokens", EvalContext{TenantID: "odd", UserID: "u-1"}, 1)
	if err != nil || d.Enabled || d.Reason != ReasonNotEntitled || d.Detail != "invalid limit scope" {
		t.Errorf("invalid scope: %+v %v", d, err)
	}
}
