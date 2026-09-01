package featurelayer

import (
	"context"
	"errors"
	"time"

	"github.com/bernardoforcillo/featurelayer/catalog"
	"github.com/bernardoforcillo/featurelayer/entitlement"
	"github.com/bernardoforcillo/featurelayer/flags"
)

// subCache loads the subscription at most once per top-level call,
// shared across prerequisite evaluations.
type subCache struct {
	loaded bool
	sub    *entitlement.Subscription
	err    error
}

// Evaluate answers: is the feature enabled for this context?
// It never returns an error: failures surface as an off decision with
// Reason store_error and Err set (fail closed).
func (e *Engine) Evaluate(ctx context.Context, key catalog.Key, ec EvalContext) Decision {
	start := time.Now()
	d := e.evaluate(ctx, e.snap.Load(), key, ec, &subCache{}, e.evalNow(ec))
	e.fireDecision(OpEvaluate, ec, d, time.Since(start), nil)
	return d
}

// IsEnabled is a boolean shortcut over Evaluate.
func (e *Engine) IsEnabled(ctx context.Context, key catalog.Key, ec EvalContext) bool {
	return e.Evaluate(ctx, key, ec).Enabled
}

// Variant returns the served variant, if any.
func (e *Engine) Variant(ctx context.Context, key catalog.Key, ec EvalContext) (flags.Variant, bool) {
	d := e.Evaluate(ctx, key, ec)
	if d.Variant == nil {
		return flags.Variant{}, false
	}
	return *d.Variant, true
}

func (e *Engine) evalNow(ec EvalContext) time.Time {
	if !ec.Now.IsZero() {
		return ec.Now
	}
	return e.clock()
}

// evaluate runs the pipeline: catalog → flag gate → entitlement →
// flag rules → prerequisites. Cheapest first; every branch fails closed.
func (e *Engine) evaluate(ctx context.Context, snap *Snapshot, key catalog.Key, ec EvalContext, sc *subCache, now time.Time) Decision {
	f, ok := snap.features[key]
	if !ok {
		return Decision{Feature: key, Reason: ReasonUnknownFeature}
	}
	d := Decision{Feature: key, Lifecycle: f.Lifecycle}
	if f.Lifecycle == catalog.Draft || f.Lifecycle == catalog.Retired {
		d.Reason, d.Detail = ReasonLifecycle, string(f.Lifecycle)
		return d
	}
	fl, hasFlag := snap.flagsByKey[key]
	if hasFlag {
		if active, out := fl.Active(now); !active {
			d.Reason = flagReason(out.Reason)
			return d
		}
	}
	var sub *entitlement.Subscription
	if !f.Free && e.subs != nil {
		if ec.TenantID == "" {
			d.Reason, d.Detail = ReasonNotEntitled, "no tenant"
			return d
		}
		if !sc.loaded {
			sc.sub, sc.err = e.subs.Subscription(ctx, ec.TenantID)
			sc.loaded = true
		}
		if sc.err != nil && !errors.Is(sc.err, entitlement.ErrNoSubscription) {
			d.Reason, d.Err = ReasonStoreError, sc.err
			return d
		}
		sub = sc.sub // nil when ErrNoSubscription
		res := snap.resolver.Resolve(sub, key, now)
		d.Entitlement = &res
		switch res.Kind {
		case entitlement.KindDenied:
			d.Reason, d.Detail = ReasonDenied, res.Source
			return d
		case entitlement.KindNone:
			d.Reason, d.Err = ReasonNotEntitled, res.Err
			return d
		}
	}
	attrs := make(map[string]any, len(ec.Attributes)+4)
	for k, val := range ec.Attributes {
		attrs[k] = val
	}
	if ec.TenantID != "" {
		attrs["tenant"] = ec.TenantID
	} else {
		delete(attrs, "tenant")
	}
	if ec.UserID != "" {
		attrs["user"] = ec.UserID
	} else {
		delete(attrs, "user")
	}
	if sub != nil {
		if pid := snap.resolver.EffectivePlan(sub, now); pid != "" {
			attrs["plan"] = string(pid)
		} else {
			delete(attrs, "plan")
		}
		attrs["addons"] = snap.resolver.EffectiveAddOns(sub, now)
	}
	if hasFlag {
		out := snap.evaluator.Serve(&fl, attrs)
		d.Reason, d.Detail = flagReason(out.Reason), out.Detail
		if !out.On {
			return d
		}
		d.Variant = out.Variant
	} else {
		d.Reason = ReasonNoFlag
	}
	for _, dep := range f.DependsOn {
		if pd := e.evaluate(ctx, snap, dep, ec, sc, now); !pd.Enabled {
			return Decision{Feature: key, Lifecycle: f.Lifecycle, Entitlement: d.Entitlement,
				Reason: ReasonPrerequisite, Detail: string(dep), Err: pd.Err}
		}
	}
	d.Enabled = true
	return d
}

func flagReason(r flags.Reason) Reason {
	switch r {
	case flags.ReasonOff:
		return ReasonFlagOff
	case flags.ReasonWindow:
		return ReasonFlagWindow
	case flags.ReasonRule:
		return ReasonFlagRule
	case flags.ReasonRollout:
		return ReasonFlagRollout
	default:
		return ReasonFlagDefault
	}
}
