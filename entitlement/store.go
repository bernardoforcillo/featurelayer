package entitlement

import (
	"context"
	"errors"

	"github.com/bernardoforcillo/featurelayer/catalog"
)

var (
	// ErrNoSubscription is returned by SubscriptionStore implementations
	// when the tenant is unknown.
	ErrNoSubscription = errors.New("entitlement: no subscription")
	// ErrEmptyTenantID is returned by Seeder.Set for a Subscription with
	// no TenantID: such a row could never be read back and the engine
	// refuses empty tenants anyway, so storing it would be a silent leak.
	ErrEmptyTenantID = errors.New("entitlement: empty tenant id")
)

// SubscriptionStore resolves a tenant's subscription. Implementations
// must be safe for concurrent use.
type SubscriptionStore interface {
	Subscription(ctx context.Context, tenantID string) (*Subscription, error)
}

// Seeder is the write side a SubscriptionStore may offer. It exists so
// the entitlementtest contract suite (and application code seeding
// tenants) can write through the same store it reads from. Set
// replaces the tenant's subscription wholesale (upsert) and MUST
// return ErrEmptyTenantID for an empty TenantID; Delete removes it and
// is a no-op for an unknown tenant. Both MUST copy what they are handed
// and what they return: a caller mutating a Subscription after Set, or
// one returned by Subscription, must never reach the stored state.
type Seeder interface {
	Set(ctx context.Context, sub Subscription) error
	Delete(ctx context.Context, tenantID string) error
}

// UsageKey identifies one usage counter. Subject is empty for
// tenant-scoped counters and the EvalContext.UserID for PerSubject
// limits; two keys differing only in Subject are distinct counters.
type UsageKey struct {
	Tenant  string
	Feature catalog.Key
	Period  string // "" or a PeriodKey value
	Subject string // "" for PerTenant limits
}

// UsageStore holds metering counters. Implementations must make
// Increment's check-and-add atomic and be safe for concurrent use.
type UsageStore interface {
	Get(ctx context.Context, key UsageKey) (int64, error)
	// Increment adds delta if total+delta <= max (or unconditionally
	// when max < 0) and returns the resulting total and whether the
	// increment was applied. On refusal the total is unchanged.
	Increment(ctx context.Context, key UsageKey, delta, max int64) (total int64, allowed bool, err error)
}
