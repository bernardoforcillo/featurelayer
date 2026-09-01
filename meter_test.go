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

func d2(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
