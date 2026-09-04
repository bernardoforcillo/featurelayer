package entitlement

import (
	"context"
	"sync"
)

// MemSubscriptions is an in-memory SubscriptionStore for tests, demos
// and small deployments.
type MemSubscriptions struct {
	mu sync.RWMutex
	m  map[string]Subscription
}

// NewMemSubscriptions returns an empty store.
func NewMemSubscriptions() *MemSubscriptions {
	return &MemSubscriptions{m: make(map[string]Subscription)}
}

// Set stores (or replaces) a deep copy of sub under sub.TenantID, so
// mutating sub afterwards never reaches the store.
func (s *MemSubscriptions) Set(sub Subscription) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[sub.TenantID] = sub.clone()
}

// Delete forgets the tenant; unknown tenants are a no-op.
func (s *MemSubscriptions) Delete(tenantID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, tenantID)
}

// Subscription returns a deep copy: the caller may mutate it freely,
// including through the grant limits and the trial, without reaching
// the stored value or another caller's copy.
func (s *MemSubscriptions) Subscription(_ context.Context, tenantID string) (*Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sub, ok := s.m[tenantID]
	if !ok {
		return nil, ErrNoSubscription
	}
	sub = sub.clone()
	return &sub, nil
}

// Seeder adapts the store to the Seeder interface: the context-taking
// Set and Delete the entitlementtest contract suite writes through.
// Go has no overloading, so the existing Set/Delete keep their
// signatures and this adapter carries the ctx-taking pair.
func (s *MemSubscriptions) Seeder() Seeder { return memSeeder{s} }

type memSeeder struct{ s *MemSubscriptions }

func (m memSeeder) Set(_ context.Context, sub Subscription) error {
	if sub.TenantID == "" {
		return ErrEmptyTenantID
	}
	m.s.Set(sub)
	return nil
}

func (m memSeeder) Delete(_ context.Context, tenantID string) error {
	m.s.Delete(tenantID)
	return nil
}

// MemUsage is an in-memory UsageStore.
type MemUsage struct {
	mu sync.Mutex
	m  map[UsageKey]int64
}

// NewMemUsage returns an empty usage store.
func NewMemUsage() *MemUsage {
	return &MemUsage{m: make(map[UsageKey]int64)}
}

// Get returns the counter, 0 for an unknown key.
func (u *MemUsage) Get(_ context.Context, key UsageKey) (int64, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.m[key], nil
}

// Increment implements UsageStore.Increment under one mutex.
func (u *MemUsage) Increment(_ context.Context, key UsageKey, delta, max int64) (int64, bool, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	cur := u.m[key]
	if max >= 0 && cur+delta > max {
		return cur, false, nil
	}
	cur += delta
	u.m[key] = cur
	return cur, true, nil
}
