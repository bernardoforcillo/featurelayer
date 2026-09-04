package entitlement_test

import (
	"testing"

	"github.com/bernardoforcillo/featurelayer/entitlement"
	"github.com/bernardoforcillo/featurelayer/entitlement/entitlementtest"
)

// The in-memory stores are the reference implementations: they run the
// exported contract suite that every other store runs.
func TestMemSubscriptionsContract(t *testing.T) {
	entitlementtest.RunSubscriptionStoreContract(t, func(*testing.T) (entitlement.SubscriptionStore, entitlement.Seeder) {
		s := entitlement.NewMemSubscriptions()
		return s, s.Seeder()
	})
}

func TestMemUsageContract(t *testing.T) {
	entitlementtest.RunUsageStoreContract(t, func(*testing.T) entitlement.UsageStore {
		return entitlement.NewMemUsage()
	})
}
