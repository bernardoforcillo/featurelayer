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

func NewMemSubscriptions() *MemSubscriptions {
	return &MemSubscriptions{m: make(map[string]Subscription)}
}

func (s *MemSubscriptions) Set(sub Subscription) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[sub.TenantID] = sub
}

func (s *MemSubscriptions) Delete(tenantID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, tenantID)
}

// Subscription returns a shallow copy: callers must not mutate values
// referenced through pointers (grant limits, trial).
func (s *MemSubscriptions) Subscription(_ context.Context, tenantID string) (*Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sub, ok := s.m[tenantID]
	if !ok {
		return nil, ErrNoSubscription
	}
	return &sub, nil
}

// MemUsage is an in-memory UsageStore.
type MemUsage struct {
	mu sync.Mutex
	m  map[UsageKey]int64
}

func NewMemUsage() *MemUsage {
	return &MemUsage{m: make(map[UsageKey]int64)}
}

func (u *MemUsage) Get(_ context.Context, key UsageKey) (int64, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.m[key], nil
}

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
