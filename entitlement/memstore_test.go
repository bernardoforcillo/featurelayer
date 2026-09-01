package entitlement

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestMemSubscriptions(t *testing.T) {
	ctx := context.Background()
	s := NewMemSubscriptions()
	if _, err := s.Subscription(ctx, "ghost"); !errors.Is(err, ErrNoSubscription) {
		t.Errorf("unknown tenant: %v", err)
	}
	s.Set(Subscription{TenantID: "acme", Plan: "pro"})
	sub, err := s.Subscription(ctx, "acme")
	if err != nil || sub.Plan != "pro" {
		t.Errorf("got %+v, %v", sub, err)
	}
	sub.Plan = "hacked" // returned value must be a copy
	again, _ := s.Subscription(ctx, "acme")
	if again.Plan != "pro" {
		t.Error("store must return copies")
	}
	s.Delete("acme")
	if _, err := s.Subscription(ctx, "acme"); !errors.Is(err, ErrNoSubscription) {
		t.Error("deleted tenant must be unknown")
	}
}

func TestMemUsageIncrement(t *testing.T) {
	ctx := context.Background()
	u := NewMemUsage()
	k := UsageKey{Tenant: "acme", Feature: "api.calls", Period: "2026-09-01T00:00:00Z"}
	total, allowed, err := u.Increment(ctx, k, 3, 5)
	if err != nil || !allowed || total != 3 {
		t.Errorf("first increment: %d %v %v", total, allowed, err)
	}
	total, allowed, _ = u.Increment(ctx, k, 3, 5)
	if allowed || total != 3 {
		t.Errorf("over max must refuse without adding: %d %v", total, allowed)
	}
	total, allowed, _ = u.Increment(ctx, k, 2, 5)
	if !allowed || total != 5 {
		t.Errorf("exact fit allowed: %d %v", total, allowed)
	}
	total, allowed, _ = u.Increment(ctx, k, 100, -1)
	if !allowed || total != 105 {
		t.Errorf("max<0 is uncapped: %d %v", total, allowed)
	}
	if got, _ := u.Get(ctx, k); got != 105 {
		t.Errorf("Get = %d", got)
	}
	if got, _ := u.Get(ctx, UsageKey{Tenant: "other"}); got != 0 {
		t.Errorf("unknown key = %d, want 0", got)
	}
}

func TestMemUsageConcurrent(t *testing.T) {
	ctx := context.Background()
	u := NewMemUsage()
	k := UsageKey{Tenant: "acme", Feature: "api.calls"}
	var wg sync.WaitGroup
	granted := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ok, _ := u.Increment(ctx, k, 1, 50)
			granted <- ok
		}()
	}
	wg.Wait()
	close(granted)
	n := 0
	for ok := range granted {
		if ok {
			n++
		}
	}
	total, _ := u.Get(ctx, k)
	if n != 50 || total != 50 {
		t.Errorf("granted=%d total=%d, want 50/50", n, total)
	}
}
