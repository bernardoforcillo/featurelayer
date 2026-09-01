package entitlement

import (
	"errors"
	"fmt"
	"time"

	"github.com/bernardoforcillo/featurelayer/catalog"
)

// ErrUnknownPlan marks a subscription referencing a plan that is not
// in the snapshot. Resolution fails closed and carries this in Err.
var ErrUnknownPlan = errors.New("entitlement: unknown plan")

type Kind string

const (
	KindNone   Kind = "none"
	KindDenied Kind = "denied"
	KindGrant  Kind = "grant"
	KindPlan   Kind = "plan"
	KindAddOn  Kind = "addon"
)

// Resolution says whether and how a tenant is entitled to a feature.
type Resolution struct {
	Kind   Kind
	Source string
	Limit  *Limit // nil = unlimited (meaningful only when entitled)
	Err    error
}

// Resolver resolves entitlements against flattened plan definitions.
type Resolver struct {
	flat      map[PlanID][]Entitlement
	ancestors map[PlanID]map[PlanID]bool // includes self
	addons    map[AddOnID]AddOn
}

// NewResolver indexes and flattens plans. It fails on unknown or
// cyclic Extends and on unknown Requires references.
func NewResolver(plans []Plan, addons []AddOn) (*Resolver, error) {
	byID := make(map[PlanID]Plan, len(plans))
	for _, p := range plans {
		byID[p.ID] = p
	}
	r := &Resolver{
		flat:      make(map[PlanID][]Entitlement, len(plans)),
		ancestors: make(map[PlanID]map[PlanID]bool, len(plans)),
		addons:    make(map[AddOnID]AddOn, len(addons)),
	}
	for _, p := range plans {
		chain := []Plan{}
		anc := map[PlanID]bool{}
		for cur, ok := p, true; ok; cur, ok = byID[cur.Extends] {
			if anc[cur.ID] {
				return nil, fmt.Errorf("entitlement: cyclic Extends at plan %q", cur.ID)
			}
			anc[cur.ID] = true
			chain = append(chain, cur)
			if cur.Extends == "" {
				break
			}
			if _, exists := byID[cur.Extends]; !exists {
				return nil, fmt.Errorf("entitlement: plan %q extends unknown plan %q", cur.ID, cur.Extends)
			}
		}
		// chain is leaf→root; apply root first, descendants override.
		merged := map[catalog.Key]int{}
		var flat []Entitlement
		for i := len(chain) - 1; i >= 0; i-- {
			for _, e := range chain[i].Entitlements {
				if idx, seen := merged[e.Feature]; seen {
					flat[idx] = e
				} else {
					merged[e.Feature] = len(flat)
					flat = append(flat, e)
				}
			}
		}
		r.flat[p.ID] = flat
		r.ancestors[p.ID] = anc
	}
	for _, a := range addons {
		for _, req := range a.Requires {
			if _, ok := byID[req]; !ok {
				return nil, fmt.Errorf("entitlement: addon %q requires unknown plan %q", a.ID, req)
			}
		}
		r.addons[a.ID] = a
	}
	return r, nil
}

// Entitlements returns the flattened entitlements of a plan, deep
// copied so a caller mutating the result (including through a Limit
// pointer) can never reach the resolver's internal state or another
// caller's copy.
func (r *Resolver) Entitlements(id PlanID) []Entitlement { return cloneEntitlements(r.flat[id]) }

// cloneEntitlements deep-copies a flattened entitlement slice: a fresh
// backing array, and a fresh Limit allocation per non-nil pointer.
func cloneEntitlements(es []Entitlement) []Entitlement {
	if es == nil {
		return nil
	}
	out := make([]Entitlement, len(es))
	for i, e := range es {
		if e.Limit != nil {
			lim := *e.Limit
			e.Limit = &lim
		}
		out[i] = e
	}
	return out
}

// EffectivePlan is the trial plan while active, else the base plan.
func (r *Resolver) EffectivePlan(sub *Subscription, now time.Time) PlanID {
	if sub == nil {
		return ""
	}
	if t := sub.Trial; t != nil && now.Before(t.Until) {
		return t.Plan
	}
	return sub.Plan
}

// EffectiveAddOns returns the ids of the subscription's add-ons that
// exist and whose Requires is satisfied by the effective plan.
func (r *Resolver) EffectiveAddOns(sub *Subscription, now time.Time) []string {
	if sub == nil {
		return nil
	}
	pid := r.EffectivePlan(sub, now)
	var out []string
	for _, aid := range sub.AddOns {
		a, ok := r.addons[aid]
		if !ok {
			continue
		}
		if len(a.Requires) > 0 && !r.requiresSatisfied(a.Requires, pid) {
			continue
		}
		out = append(out, string(aid))
	}
	return out
}

func (r *Resolver) requiresSatisfied(req []PlanID, effective PlanID) bool {
	anc := r.ancestors[effective]
	for _, p := range req {
		if anc[p] {
			return true
		}
	}
	return false
}

// Resolve applies the resolution order: active Deny grant (always wins
// over allow grant for the same feature, regardless of slice order), active
// allow grant (its limit replaces), effective plan + effective add-ons
// (limits summed, unlimited dominates), otherwise none.
func (r *Resolver) Resolve(sub *Subscription, feature catalog.Key, now time.Time) Resolution {
	if sub == nil {
		return Resolution{Kind: KindNone}
	}
	// First pass: check for active Deny grants (they always win)
	for _, g := range sub.Grants {
		if g.Feature != feature || !g.active(now) || !g.Deny {
			continue
		}
		return Resolution{Kind: KindDenied, Source: g.Reason}
	}
	// Second pass: check for active allow grants
	for _, g := range sub.Grants {
		if g.Feature != feature || !g.active(now) || g.Deny {
			continue
		}
		return Resolution{Kind: KindGrant, Source: g.Reason, Limit: g.Limit}
	}
	var res Resolution
	var limits []*Limit
	pid := r.EffectivePlan(sub, now)
	if pid != "" {
		ents, known := r.flat[pid]
		if !known {
			res.Err = fmt.Errorf("%w: %q", ErrUnknownPlan, pid)
		}
		for _, e := range ents {
			if e.Feature == feature {
				res.Kind, res.Source = KindPlan, string(pid)
				limits = append(limits, e.Limit)
			}
		}
	}
	for _, aid := range r.EffectiveAddOns(sub, now) {
		for _, e := range r.addons[AddOnID(aid)].Entitlements {
			if e.Feature == feature {
				if res.Kind == "" {
					res.Kind, res.Source = KindAddOn, aid
				}
				limits = append(limits, e.Limit)
			}
		}
	}
	if res.Kind == "" {
		res.Kind = KindNone
		return res
	}
	res.Limit = sumLimits(limits)
	return res
}

// sumLimits sums Max across sources; any nil (unlimited) dominates.
// Periods are equal by snapshot validation.
func sumLimits(limits []*Limit) *Limit {
	var total int64
	var period Period
	for _, l := range limits {
		if l == nil {
			return nil
		}
		total += l.Max
		period = l.Period
	}
	return &Limit{Max: total, Period: period}
}
