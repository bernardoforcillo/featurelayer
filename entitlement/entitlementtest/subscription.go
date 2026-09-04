package entitlementtest

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/bernardoforcillo/featurelayer/entitlement"
)

// RunSubscriptionStoreContract runs the SubscriptionStore contract
// against stores built by newStore, which must return an empty store
// and the Seeder that writes to it (for a store that implements both,
// return it twice).
//
// The contract: an unknown tenant is ErrNoSubscription; Set stores the
// whole Subscription and a later Set for the same tenant replaces it;
// the value Subscription returns is a copy the caller may mutate;
// Delete makes the tenant unknown and is a no-op for one that already
// is; tenants never see each other's rows.
func RunSubscriptionStoreContract(t *testing.T, newStore func(t *testing.T) (entitlement.SubscriptionStore, entitlement.Seeder)) {
	t.Helper()
	ctx := context.Background()
	anchor := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	full := entitlement.Subscription{
		TenantID: "acme",
		Plan:     "pro",
		AddOns:   []entitlement.AddOnID{"extra-calls", "sso"},
		Trial:    &entitlement.PlanTrial{Plan: "enterprise", Until: anchor.AddDate(0, 1, 0)},
		Grants: []entitlement.Grant{
			entitlement.Override("export.csv", &entitlement.Limit{Max: 5, Period: entitlement.Month}, "deal"),
			entitlement.Trial("sso.saml", anchor.AddDate(0, 0, 14), "eval"),
			{Feature: "ai.tokens", Deny: true, Reason: "abuse"},
			entitlement.Override("seats", &entitlement.Limit{Max: 3, Per: entitlement.PerSubject}, "pilot"),
		},
		BillingAnchor: anchor,
	}

	t.Run("UnknownTenantIsErrNoSubscription", func(t *testing.T) {
		store, _ := newStore(t)
		sub, err := store.Subscription(ctx, "ghost")
		if !errors.Is(err, entitlement.ErrNoSubscription) {
			t.Fatalf("Subscription(ghost) err = %v, want ErrNoSubscription", err)
		}
		if sub != nil {
			t.Errorf("Subscription(ghost) = %+v, want nil", sub)
		}
	})

	t.Run("SetThenSubscriptionRoundTripsEveryField", func(t *testing.T) {
		store, seed := newStore(t)
		if err := seed.Set(ctx, full); err != nil {
			t.Fatalf("Set: %v", err)
		}
		got, err := store.Subscription(ctx, "acme")
		if err != nil {
			t.Fatalf("Subscription: %v", err)
		}
		assertSubscriptionEqual(t, *got, full)
	})

	t.Run("MinimalSubscriptionRoundTrips", func(t *testing.T) {
		// Zero anchor, no trial, no add-ons, no grants: the store must
		// hand back exactly that shape — a zero time, a nil trial — not
		// an epoch or an empty struct that the engine would then act on.
		store, seed := newStore(t)
		min := entitlement.Subscription{TenantID: "solo", Plan: "free"}
		if err := seed.Set(ctx, min); err != nil {
			t.Fatalf("Set: %v", err)
		}
		got, err := store.Subscription(ctx, "solo")
		if err != nil {
			t.Fatalf("Subscription: %v", err)
		}
		assertSubscriptionEqual(t, *got, min)
	})

	t.Run("SetReplacesWholesale", func(t *testing.T) {
		store, seed := newStore(t)
		if err := seed.Set(ctx, full); err != nil {
			t.Fatalf("Set: %v", err)
		}
		replacement := entitlement.Subscription{TenantID: "acme", Plan: "free"}
		if err := seed.Set(ctx, replacement); err != nil {
			t.Fatalf("second Set: %v", err)
		}
		got, err := store.Subscription(ctx, "acme")
		if err != nil {
			t.Fatalf("Subscription: %v", err)
		}
		// Nothing from the first write may survive: not the add-ons,
		// not the trial, not the grants, not the anchor.
		assertSubscriptionEqual(t, *got, replacement)
	})

	t.Run("ReturnedValueIsACopy", func(t *testing.T) {
		store, seed := newStore(t)
		if err := seed.Set(ctx, full); err != nil {
			t.Fatalf("Set: %v", err)
		}
		first, err := store.Subscription(ctx, "acme")
		if err != nil {
			t.Fatalf("Subscription: %v", err)
		}
		first.Plan = "hacked"
		first.AddOns = append(first.AddOns, "injected")
		first.Grants[0].Deny = true
		again, err := store.Subscription(ctx, "acme")
		if err != nil {
			t.Fatalf("Subscription: %v", err)
		}
		if again.Plan != "pro" || len(again.AddOns) != 2 || again.Grants[0].Deny {
			t.Errorf("mutating a returned subscription leaked into the store: %+v", again)
		}
	})

	t.Run("DeleteMakesTenantUnknownAndIsIdempotent", func(t *testing.T) {
		store, seed := newStore(t)
		if err := seed.Set(ctx, full); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if err := seed.Delete(ctx, "acme"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := store.Subscription(ctx, "acme"); !errors.Is(err, entitlement.ErrNoSubscription) {
			t.Errorf("after Delete err = %v, want ErrNoSubscription", err)
		}
		if err := seed.Delete(ctx, "acme"); err != nil {
			t.Errorf("second Delete must be a no-op, got %v", err)
		}
		if err := seed.Delete(ctx, "never-existed"); err != nil {
			t.Errorf("Delete of an unknown tenant must be a no-op, got %v", err)
		}
	})

	t.Run("TenantsAreIsolated", func(t *testing.T) {
		store, seed := newStore(t)
		a := entitlement.Subscription{TenantID: "a", Plan: "pro"}
		b := entitlement.Subscription{TenantID: "b", Plan: "free", AddOns: []entitlement.AddOnID{"x"}}
		for _, s := range []entitlement.Subscription{a, b} {
			if err := seed.Set(ctx, s); err != nil {
				t.Fatalf("Set(%s): %v", s.TenantID, err)
			}
		}
		gotA, err := store.Subscription(ctx, "a")
		if err != nil {
			t.Fatalf("Subscription(a): %v", err)
		}
		assertSubscriptionEqual(t, *gotA, a)
		if err := seed.Delete(ctx, "a"); err != nil {
			t.Fatalf("Delete(a): %v", err)
		}
		gotB, err := store.Subscription(ctx, "b")
		if err != nil {
			t.Fatalf("Subscription(b) after Delete(a): %v", err)
		}
		assertSubscriptionEqual(t, *gotB, b)
	})

	t.Run("SetRejectsEmptyTenantID", func(t *testing.T) {
		// A row with no tenant is unreachable through Subscription and
		// would be a silent write; the store must refuse it rather than
		// store it under "".
		_, seed := newStore(t)
		if err := seed.Set(ctx, entitlement.Subscription{Plan: "pro"}); !errors.Is(err, entitlement.ErrEmptyTenantID) {
			t.Errorf("Set with empty TenantID err = %v, want ErrEmptyTenantID", err)
		}
	})

	t.Run("SetArgumentIsCopied", func(t *testing.T) {
		store, seed := newStore(t)
		arg := entitlement.Subscription{
			TenantID: "acme",
			Plan:     "pro",
			Trial:    &entitlement.PlanTrial{Plan: "enterprise", Until: anchor},
			Grants:   []entitlement.Grant{entitlement.Override("export.csv", &entitlement.Limit{Max: 5}, "deal")},
		}
		if err := seed.Set(ctx, arg); err != nil {
			t.Fatalf("Set: %v", err)
		}
		arg.Grants[0].Deny = true
		arg.Grants[0].Limit.Max = 99
		arg.Trial.Plan = "changed"
		got, err := store.Subscription(ctx, "acme")
		if err != nil {
			t.Fatalf("Subscription: %v", err)
		}
		if got.Grants[0].Deny || got.Grants[0].Limit.Max != 5 || got.Trial.Plan != "enterprise" {
			t.Errorf("mutating the Set argument leaked into the store: %+v", got)
		}
	})
}

// assertSubscriptionEqual compares field by field so time values are
// compared as instants (a store may hand back a different Location)
// and empty and nil slices count as the same absence.
func assertSubscriptionEqual(t *testing.T, got, want entitlement.Subscription) {
	t.Helper()
	if got.TenantID != want.TenantID || got.Plan != want.Plan {
		t.Errorf("tenant/plan = %q/%q, want %q/%q", got.TenantID, got.Plan, want.TenantID, want.Plan)
	}
	if len(got.AddOns) != len(want.AddOns) || (len(want.AddOns) > 0 && !reflect.DeepEqual(got.AddOns, want.AddOns)) {
		t.Errorf("addons = %v, want %v", got.AddOns, want.AddOns)
	}
	switch {
	case got.Trial == nil && want.Trial == nil:
	case got.Trial == nil || want.Trial == nil:
		t.Errorf("trial = %+v, want %+v", got.Trial, want.Trial)
	case got.Trial.Plan != want.Trial.Plan || !got.Trial.Until.Equal(want.Trial.Until):
		t.Errorf("trial = %+v, want %+v", got.Trial, want.Trial)
	}
	if len(got.Grants) != len(want.Grants) {
		t.Fatalf("grants = %+v, want %+v", got.Grants, want.Grants)
	}
	for i := range want.Grants {
		g, w := got.Grants[i], want.Grants[i]
		if g.Feature != w.Feature || g.Deny != w.Deny || g.Reason != w.Reason || !g.Until.Equal(w.Until) {
			t.Errorf("grants[%d] = %+v, want %+v", i, g, w)
		}
		switch {
		case g.Limit == nil && w.Limit == nil:
		case g.Limit == nil || w.Limit == nil || *g.Limit != *w.Limit:
			t.Errorf("grants[%d].limit = %+v, want %+v", i, g.Limit, w.Limit)
		}
	}
	if !got.BillingAnchor.Equal(want.BillingAnchor) || got.BillingAnchor.IsZero() != want.BillingAnchor.IsZero() {
		t.Errorf("billing anchor = %v, want %v", got.BillingAnchor, want.BillingAnchor)
	}
}
