// Package entitlementtest is the exported contract suite for the two
// entitlement persistence ports. Every SubscriptionStore and UsageStore
// implementation — the in-memory ones in package entitlement, the
// PostgreSQL ones in store/drops, and any the application writes — runs
// the same checks:
//
//	func TestMyStores(t *testing.T) {
//		entitlementtest.RunSubscriptionStoreContract(t, func(t *testing.T) (entitlement.SubscriptionStore, entitlement.Seeder) {
//			s := newMySubscriptionStore(t)
//			return s, s
//		})
//		entitlementtest.RunUsageStoreContract(t, func(t *testing.T) entitlement.UsageStore {
//			return newMyUsageStore(t)
//		})
//	}
//
// newStore is called once per subtest and must return an EMPTY store;
// register any teardown with t.Cleanup. The suites are self-contained:
// they need no snapshot, no engine and no clock, and they never reach
// past the two interfaces, so a store that passes here behaves the same
// under the engine as the reference implementation does.
//
// The usage suite's mass-race check is the one that matters most: it
// proves Increment admits exactly max units under concurrency, which is
// the property the engine's limit_reached decision rests on and the one
// a read-then-write implementation gets wrong.
package entitlementtest
