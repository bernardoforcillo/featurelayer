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
	period, max, anchor := meterParams(d, sc)
	uk := entitlement.UsageKey{Tenant: ec.TenantID, Feature: key, Period: entitlement.PeriodKey(period, anchor, now)}
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
	period, max, anchor := meterParams(d, sc)
	uk := entitlement.UsageKey{Tenant: ec.TenantID, Feature: key, Period: entitlement.PeriodKey(period, anchor, now)}
	total, err := e.usage.Get(ctx, uk)
	if err != nil {
		d.Enabled, d.Reason, d.Err = false, ReasonStoreError, err
		return d, err
	}
	d.Usage = usageInfo(total, max, period, anchor, now, uk.Period)
	return d, nil
}

// meterParams extracts the effective limit and billing anchor from an
// enabled decision. No entitlement (Free / flags-only) = unlimited.
func meterParams(d Decision, sc *subCache) (entitlement.Period, int64, time.Time) {
	period, max := entitlement.None, int64(-1)
	if d.Entitlement != nil && d.Entitlement.Limit != nil {
		period, max = d.Entitlement.Limit.Period, d.Entitlement.Limit.Max
	}
	var anchor time.Time
	if sc.sub != nil {
		anchor = sc.sub.BillingAnchor
	}
	return period, max, anchor
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
