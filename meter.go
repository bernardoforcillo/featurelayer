package featurelayer

import (
	"context"
	"errors"
	"time"

	"github.com/bernardoforcillo/featurelayer/catalog"
	"github.com/bernardoforcillo/featurelayer/entitlement"
)

var (
	// ErrNoUsageStore: Consume/Usage need WithUsage.
	ErrNoUsageStore = errors.New("featurelayer: no usage store configured")
	// ErrInvalidDelta: Consume needs n > 0.
	ErrInvalidDelta = errors.New("featurelayer: delta must be positive")
)

// Consume evaluates the feature and, when enabled, atomically records
// n units against the effective limit. A refused increment turns the
// decision off with Reason limit_reached. Errors are infrastructure
// or misuse only — a business "no" is a Decision, not an error.
func (e *Engine) Consume(ctx context.Context, key catalog.Key, ec EvalContext, n int64) (Decision, error) {
	start := time.Now()
	d, err := e.consume(ctx, key, ec, n)
	e.fireDecision(OpConsume, ec, d, time.Since(start), err)
	return d, err
}

func (e *Engine) consume(ctx context.Context, key catalog.Key, ec EvalContext, n int64) (Decision, error) {
	if e.usage == nil {
		return Decision{Feature: key}, ErrNoUsageStore
	}
	if n <= 0 {
		return Decision{Feature: key}, ErrInvalidDelta
	}
	snap := e.snap.Load()
	sc := &subCache{}
	now := e.evalNow(ec)
	d := e.evaluate(ctx, snap, key, ec, sc, now)
	if !d.Enabled {
		return d, nil
	}
	uk, max, period, anchor, ok := e.meterKey(&d, ec, sc, now)
	if !ok {
		return d, nil
	}
	total, allowed, err := e.usage.Increment(ctx, uk, n, max)
	if err != nil {
		d.Enabled, d.Reason, d.Err = false, ReasonStoreError, err
		return d, err
	}
	d.Usage = usageInfo(total, max, period, anchor, now, uk.Period)
	if !allowed {
		d.Enabled, d.Reason, d.Variant = false, ReasonLimitReached, nil
	}
	return d, nil
}

// Usage is the read-only twin of Consume.
func (e *Engine) Usage(ctx context.Context, key catalog.Key, ec EvalContext) (Decision, error) {
	start := time.Now()
	d, err := e.usageRead(ctx, key, ec)
	e.fireDecision(OpUsage, ec, d, time.Since(start), err)
	return d, err
}

func (e *Engine) usageRead(ctx context.Context, key catalog.Key, ec EvalContext) (Decision, error) {
	if e.usage == nil {
		return Decision{Feature: key}, ErrNoUsageStore
	}
	snap := e.snap.Load()
	sc := &subCache{}
	now := e.evalNow(ec)
	d := e.evaluate(ctx, snap, key, ec, sc, now)
	if !d.Enabled {
		return d, nil
	}
	uk, max, period, anchor, ok := e.meterKey(&d, ec, sc, now)
	if !ok {
		return d, nil
	}
	total, err := e.usage.Get(ctx, uk)
	if err != nil {
		d.Enabled, d.Reason, d.Err = false, ReasonStoreError, err
		return d, err
	}
	d.Usage = usageInfo(total, max, period, anchor, now, uk.Period)
	return d, nil
}

// meterKey derives the usage counter for an enabled decision: the
// effective limit and billing anchor, plus the counter's key. No
// entitlement (Free / flags-only) = unlimited, tenant-scoped.
//
// A PerSubject limit meters EvalContext.UserID. Without a subject
// there is no counter to charge, and charging the tenant's shared
// counter instead would silently widen the limit, so the decision is
// turned off (Reason not_entitled, Detail "no subject") and ok is
// false. An unknown scope (only reachable through a Grant, which
// snapshot validation never sees) fails closed the same way.
func (e *Engine) meterKey(d *Decision, ec EvalContext, sc *subCache, now time.Time) (uk entitlement.UsageKey, max int64, period entitlement.Period, anchor time.Time, ok bool) {
	period, max = entitlement.None, -1
	scope := entitlement.PerTenant
	if d.Entitlement != nil && d.Entitlement.Limit != nil {
		l := d.Entitlement.Limit
		period, max, scope = l.Period, l.Max, l.Scope()
	}
	if sc.sub != nil {
		anchor = sc.sub.BillingAnchor
	}
	uk = entitlement.UsageKey{Tenant: ec.TenantID, Feature: d.Feature, Period: entitlement.PeriodKey(period, anchor, now)}
	switch scope {
	case entitlement.PerTenant:
	case entitlement.PerSubject:
		if ec.UserID == "" {
			d.Enabled, d.Reason, d.Detail, d.Variant = false, ReasonNotEntitled, "no subject", nil
			return uk, max, period, anchor, false
		}
		uk.Subject = ec.UserID
	default:
		d.Enabled, d.Reason, d.Detail, d.Variant = false, ReasonNotEntitled, "invalid limit scope", nil
		return uk, max, period, anchor, false
	}
	return uk, max, period, anchor, true
}

func usageInfo(total, max int64, period entitlement.Period, anchor, now time.Time, key string) *UsageInfo {
	u := &UsageInfo{Used: total, Max: max, Remaining: -1, Period: key}
	if max >= 0 {
		u.Remaining = max - total
		if u.Remaining < 0 {
			u.Remaining = 0
		}
	}
	if period != entitlement.None {
		u.PeriodStart = entitlement.PeriodStart(period, anchor, now)
		u.ResetsAt = entitlement.PeriodEnd(period, anchor, now)
	}
	return u
}
