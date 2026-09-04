package entitlement

import (
	"errors"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func testResolver(t *testing.T) *Resolver {
	t.Helper()
	r, err := NewResolver(
		[]Plan{
			{ID: "free", Entitlements: []Entitlement{Limited("api.calls", 100, Month)}},
			{ID: "pro", Extends: "free", Entitlements: []Entitlement{
				{Feature: "export.csv"},           // unlimited
				Limited("api.calls", 1000, Month), // overrides free's 100
			}},
			{ID: "enterprise", Extends: "pro"},
		},
		[]AddOn{
			{ID: "extra-calls", Requires: []PlanID{"pro"}, Entitlements: []Entitlement{Limited("api.calls", 500, Month)}},
			{ID: "sso", Entitlements: []Entitlement{{Feature: "sso.saml"}}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNewResolverRejectsBadPlans(t *testing.T) {
	if _, err := NewResolver([]Plan{{ID: "a", Extends: "ghost"}}, nil); err == nil {
		t.Error("unknown Extends must fail")
	}
	if _, err := NewResolver([]Plan{{ID: "a", Extends: "b"}, {ID: "b", Extends: "a"}}, nil); err == nil {
		t.Error("cyclic Extends must fail")
	}
	if _, err := NewResolver(nil, []AddOn{{ID: "x", Requires: []PlanID{"ghost"}}}); err == nil {
		t.Error("unknown Requires must fail")
	}
}

func TestFlattening(t *testing.T) {
	r := testResolver(t)
	ents := r.Entitlements("pro")
	byFeat := map[string]*Limit{}
	for _, e := range ents {
		byFeat[string(e.Feature)] = e.Limit
	}
	if l := byFeat["api.calls"]; l == nil || l.Max != 1000 {
		t.Errorf("child overrides parent: %+v", l)
	}
	if _, ok := byFeat["export.csv"]; !ok {
		t.Error("own entitlement present")
	}
	ents = r.Entitlements("enterprise")
	found := false
	for _, e := range ents {
		if e.Feature == "api.calls" && e.Limit != nil && e.Limit.Max == 1000 {
			found = true
		}
	}
	if !found {
		t.Error("two-level inheritance must reach grandparent-through-parent values")
	}
}

func TestResolveOrder(t *testing.T) {
	r := testResolver(t)
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	sub := &Subscription{TenantID: "acme", Plan: "pro", AddOns: []AddOnID{"extra-calls", "sso", "ghost"}}

	if res := r.Resolve(sub, "export.csv", now); res.Kind != KindPlan || res.Source != "pro" || res.Limit != nil {
		t.Errorf("plan unlimited: %+v", res)
	}
	if res := r.Resolve(sub, "api.calls", now); res.Kind != KindPlan || res.Limit == nil || res.Limit.Max != 1500 || res.Limit.Period != Month {
		t.Errorf("plan+addon limits sum: %+v", res)
	}
	if res := r.Resolve(sub, "sso.saml", now); res.Kind != KindAddOn || res.Source != "sso" {
		t.Errorf("addon-only: %+v", res)
	}
	if res := r.Resolve(sub, "unknown.feature", now); res.Kind != KindNone {
		t.Errorf("not entitled: %+v", res)
	}

	denied := &Subscription{Plan: "pro", Grants: []Grant{{Feature: "export.csv", Deny: true, Reason: "abuse"}}}
	if res := r.Resolve(denied, "export.csv", now); res.Kind != KindDenied || res.Source != "abuse" {
		t.Errorf("deny wins: %+v", res)
	}

	// Deny grant priority: even if allow grant is listed first, deny wins
	allowBeforeDeny := &Subscription{Plan: "free", Grants: []Grant{
		Override("export.csv", &Limit{Max: 5}, "promo grant"),
		{Feature: "export.csv", Deny: true, Reason: "abuse"},
	}}
	if res := r.Resolve(allowBeforeDeny, "export.csv", now); res.Kind != KindDenied || res.Source != "abuse" {
		t.Errorf("deny wins even when allow listed first: %+v", res)
	}

	// Tie-break within allow grants: first active allow grant wins
	multiAllow := &Subscription{Plan: "free", Grants: []Grant{
		Override("export.csv", &Limit{Max: 5}, "first grant"),
		Override("export.csv", &Limit{Max: 10}, "second grant"),
	}}
	if res := r.Resolve(multiAllow, "export.csv", now); res.Kind != KindGrant || res.Limit.Max != 5 {
		t.Errorf("first allow grant wins: %+v", res)
	}

	granted := &Subscription{Plan: "free", Grants: []Grant{Override("export.csv", &Limit{Max: 5}, "enterprise deal")}}
	if res := r.Resolve(granted, "export.csv", now); res.Kind != KindGrant || res.Limit.Max != 5 {
		t.Errorf("grant replaces plan limit: %+v", res)
	}

	trialGrant := &Subscription{Grants: []Grant{Trial("export.csv", future, "trial")}}
	if res := r.Resolve(trialGrant, "export.csv", now); res.Kind != KindGrant {
		t.Errorf("active feature trial: %+v", res)
	}
	expired := &Subscription{Grants: []Grant{Trial("export.csv", past, "trial")}}
	if res := r.Resolve(expired, "export.csv", now); res.Kind != KindNone {
		t.Errorf("expired feature trial: %+v", res)
	}

	planTrial := &Subscription{Plan: "free", Trial: &PlanTrial{Plan: "pro", Until: future}}
	if res := r.Resolve(planTrial, "export.csv", now); res.Kind != KindPlan || res.Source != "pro" {
		t.Errorf("active plan trial: %+v", res)
	}
	if pid := r.EffectivePlan(planTrial, future); pid != "free" {
		t.Errorf("expired plan trial falls back: %q", pid)
	}

	// Requires satisfied through an ancestor: enterprise extends pro.
	ent := &Subscription{Plan: "enterprise", AddOns: []AddOnID{"extra-calls"}}
	if res := r.Resolve(ent, "api.calls", now); res.Limit == nil || res.Limit.Max != 1500 {
		t.Errorf("Requires via ancestor: %+v", res)
	}
	// Requires not satisfied on free: addon ignored, free's own limit applies.
	freeSub := &Subscription{Plan: "free", AddOns: []AddOnID{"extra-calls"}}
	if res := r.Resolve(freeSub, "api.calls", now); res.Limit == nil || res.Limit.Max != 100 {
		t.Errorf("unmet Requires ignored: %+v", res)
	}

	unknown := &Subscription{Plan: "ghost"}
	res := r.Resolve(unknown, "export.csv", now)
	if res.Kind != KindNone || !errors.Is(res.Err, ErrUnknownPlan) {
		t.Errorf("unknown plan: %+v", res)
	}
	if res := r.Resolve(nil, "export.csv", now); res.Kind != KindNone {
		t.Errorf("nil subscription: %+v", res)
	}

	addons := r.EffectiveAddOns(sub, now)
	if len(addons) != 2 || addons[0] != "extra-calls" || addons[1] != "sso" {
		t.Errorf("effective addons: %v", addons)
	}
}

func TestResolveCarriesLimitScope(t *testing.T) {
	r, err := NewResolver(
		[]Plan{{ID: "pro", Entitlements: []Entitlement{LimitedPer("ai.tokens", 10, Day, PerSubject)}}},
		[]AddOn{{ID: "more", Entitlements: []Entitlement{LimitedPer("ai.tokens", 5, Day, PerSubject)}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	sub := &Subscription{Plan: "pro", AddOns: []AddOnID{"more"}}
	res := r.Resolve(sub, "ai.tokens", now)
	if res.Kind != KindPlan || res.Limit == nil || res.Limit.Max != 15 || res.Limit.Per != PerSubject {
		t.Errorf("summed per-subject limit: %+v %+v", res, res.Limit)
	}
	// An explicit scope on one source wins over an empty one on another
	// (they mean the same thing; validation rejects real disagreement).
	r, _ = NewResolver(
		[]Plan{{ID: "pro", Entitlements: []Entitlement{Limited("api.calls", 10, Month)}}},
		[]AddOn{{ID: "more", Entitlements: []Entitlement{LimitedPer("api.calls", 5, Month, PerTenant)}}},
	)
	res = r.Resolve(&Subscription{Plan: "pro", AddOns: []AddOnID{"more"}}, "api.calls", now)
	if res.Limit == nil || res.Limit.Max != 15 || res.Limit.Scope() != PerTenant {
		t.Errorf("tenant-scoped sum: %+v", res.Limit)
	}
}
