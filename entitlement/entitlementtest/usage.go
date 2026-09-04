package entitlementtest

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bernardoforcillo/featurelayer/entitlement"
)

// RunUsageStoreContract runs the UsageStore contract against stores
// built by newStore, which must return an empty store.
//
// The contract: Get on an unknown key is 0; Increment adds delta when
// total+delta <= max and returns the new total with allowed=true;
// a refused Increment returns the UNCHANGED total with allowed=false
// and writes nothing; max < 0 is unlimited; every field of UsageKey —
// tenant, feature, period and subject — separates counters; and under
// concurrency exactly max units are ever admitted for one key.
func RunUsageStoreContract(t *testing.T, newStore func(t *testing.T) entitlement.UsageStore) {
	t.Helper()
	ctx := context.Background()
	key := entitlement.UsageKey{Tenant: "acme", Feature: "api.calls", Period: "2026-09-01T00:00:00Z"}

	t.Run("GetUnknownKeyIsZero", func(t *testing.T) {
		store := newStore(t)
		if got, err := store.Get(ctx, key); err != nil || got != 0 {
			t.Errorf("Get(unknown) = %d, %v; want 0, nil", got, err)
		}
		// Reading must not create anything a later Increment would see.
		total, allowed, err := store.Increment(ctx, key, 1, 1)
		if err != nil || !allowed || total != 1 {
			t.Errorf("Increment after Get = %d %v %v; want 1 true nil", total, allowed, err)
		}
	})

	t.Run("IncrementAddsUpToMaxInclusive", func(t *testing.T) {
		store := newStore(t)
		total, allowed, err := store.Increment(ctx, key, 3, 5)
		if err != nil || !allowed || total != 3 {
			t.Fatalf("first Increment = %d %v %v; want 3 true nil", total, allowed, err)
		}
		total, allowed, err = store.Increment(ctx, key, 2, 5)
		if err != nil || !allowed || total != 5 {
			t.Errorf("exact fit = %d %v %v; want 5 true nil", total, allowed, err)
		}
		if got, _ := store.Get(ctx, key); got != 5 {
			t.Errorf("Get = %d, want 5", got)
		}
	})

	t.Run("RefusalLeavesTotalUnchanged", func(t *testing.T) {
		store := newStore(t)
		if _, _, err := store.Increment(ctx, key, 4, 5); err != nil {
			t.Fatal(err)
		}
		total, allowed, err := store.Increment(ctx, key, 2, 5)
		if err != nil || allowed || total != 4 {
			t.Errorf("over max = %d %v %v; want 4 false nil", total, allowed, err)
		}
		if got, _ := store.Get(ctx, key); got != 4 {
			t.Errorf("refused Increment wrote something: Get = %d, want 4", got)
		}
		// A refusal is not sticky: a smaller delta that fits still lands.
		total, allowed, err = store.Increment(ctx, key, 1, 5)
		if err != nil || !allowed || total != 5 {
			t.Errorf("fit after refusal = %d %v %v; want 5 true nil", total, allowed, err)
		}
	})

	t.Run("FirstIncrementOverMaxIsRefused", func(t *testing.T) {
		// The no-row case: a fresh key must not be created with a total
		// above max just because there was nothing to compare against.
		store := newStore(t)
		total, allowed, err := store.Increment(ctx, key, 6, 5)
		if err != nil || allowed || total != 0 {
			t.Errorf("fresh over-max = %d %v %v; want 0 false nil", total, allowed, err)
		}
		if got, _ := store.Get(ctx, key); got != 0 {
			t.Errorf("Get after refused first Increment = %d, want 0", got)
		}
	})

	t.Run("MaxZeroAdmitsNothing", func(t *testing.T) {
		store := newStore(t)
		total, allowed, err := store.Increment(ctx, key, 1, 0)
		if err != nil || allowed || total != 0 {
			t.Errorf("max 0 = %d %v %v; want 0 false nil", total, allowed, err)
		}
	})

	t.Run("NegativeMaxIsUnlimited", func(t *testing.T) {
		store := newStore(t)
		total, allowed, err := store.Increment(ctx, key, 1_000_000, -1)
		if err != nil || !allowed || total != 1_000_000 {
			t.Fatalf("unlimited = %d %v %v", total, allowed, err)
		}
		total, allowed, err = store.Increment(ctx, key, 1_000_000, -1)
		if err != nil || !allowed || total != 2_000_000 {
			t.Errorf("unlimited again = %d %v %v", total, allowed, err)
		}
	})

	t.Run("KeysAreIsolated", func(t *testing.T) {
		store := newStore(t)
		base := entitlement.UsageKey{Tenant: "acme", Feature: "api.calls", Period: "2026-09-01T00:00:00Z", Subject: "u-1"}
		variants := map[string]entitlement.UsageKey{
			"tenant":     {Tenant: "other", Feature: base.Feature, Period: base.Period, Subject: base.Subject},
			"feature":    {Tenant: base.Tenant, Feature: "export.csv", Period: base.Period, Subject: base.Subject},
			"period":     {Tenant: base.Tenant, Feature: base.Feature, Period: "2026-10-01T00:00:00Z", Subject: base.Subject},
			"subject":    {Tenant: base.Tenant, Feature: base.Feature, Period: base.Period, Subject: "u-2"},
			"tenantwide": {Tenant: base.Tenant, Feature: base.Feature, Period: base.Period},
			"noperiod":   {Tenant: base.Tenant, Feature: base.Feature, Subject: base.Subject},
		}
		if _, _, err := store.Increment(ctx, base, 5, 5); err != nil {
			t.Fatal(err)
		}
		for name, k := range variants {
			if got, _ := store.Get(ctx, k); got != 0 {
				t.Errorf("%s: key differing only in %s shares the counter (Get = %d)", name, name, got)
			}
			total, allowed, err := store.Increment(ctx, k, 1, 5)
			if err != nil || !allowed || total != 1 {
				t.Errorf("%s: Increment on sibling key = %d %v %v; want 1 true nil", name, total, allowed, err)
			}
		}
		if got, _ := store.Get(ctx, base); got != 5 {
			t.Errorf("base counter disturbed by siblings: %d", got)
		}
	})

	t.Run("MassRaceAdmitsExactlyMax", func(t *testing.T) {
		// G goroutines each try N single-unit increments against a cap of
		// max < G*N. The store must admit exactly max of them and refuse
		// the rest, and the final total must be max: a read-then-write
		// implementation over-admits here.
		store := newStore(t)
		const goroutines, perGoroutine, max = 32, 20, 250
		var admitted, refused atomic.Int64
		var firstErr atomic.Pointer[error]
		var wg sync.WaitGroup
		start := make(chan struct{})
		for g := 0; g < goroutines; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for i := 0; i < perGoroutine; i++ {
					_, ok, err := store.Increment(ctx, key, 1, max)
					if err != nil {
						firstErr.CompareAndSwap(nil, &err)
						return
					}
					if ok {
						admitted.Add(1)
					} else {
						refused.Add(1)
					}
				}
			}()
		}
		close(start)
		wg.Wait()
		if e := firstErr.Load(); e != nil {
			t.Fatalf("Increment under concurrency: %v", *e)
		}
		if admitted.Load() != max {
			t.Errorf("admitted %d increments, want exactly %d", admitted.Load(), max)
		}
		if want := int64(goroutines*perGoroutine - max); refused.Load() != want {
			t.Errorf("refused %d, want %d", refused.Load(), want)
		}
		if got, err := store.Get(ctx, key); err != nil || got != max {
			t.Errorf("final total = %d, %v; want %d", got, err, max)
		}
	})

	t.Run("MassRaceWithMixedDeltasNeverExceedsMax", func(t *testing.T) {
		// Deltas of 1..7 racing against a cap: the sum of admitted
		// deltas must equal the stored total and never exceed max.
		store := newStore(t)
		const goroutines, perGoroutine, max = 16, 25, 400
		var admittedUnits atomic.Int64
		var firstErr atomic.Pointer[error]
		var wg sync.WaitGroup
		for g := 0; g < goroutines; g++ {
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				for i := 0; i < perGoroutine; i++ {
					delta := int64((g+i)%7 + 1)
					total, ok, err := store.Increment(ctx, key, delta, max)
					if err != nil {
						firstErr.CompareAndSwap(nil, &err)
						return
					}
					if total > max {
						err := fmt.Errorf("total %d exceeded max %d", total, max)
						firstErr.CompareAndSwap(nil, &err)
						return
					}
					if ok {
						admittedUnits.Add(delta)
					}
				}
			}(g)
		}
		wg.Wait()
		if e := firstErr.Load(); e != nil {
			t.Fatalf("Increment under concurrency: %v", *e)
		}
		got, err := store.Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if got != admittedUnits.Load() {
			t.Errorf("stored total %d != sum of admitted deltas %d", got, admittedUnits.Load())
		}
		if got > max {
			t.Errorf("stored total %d exceeds max %d", got, max)
		}
	})
}
