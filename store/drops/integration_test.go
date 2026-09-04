//go:build integration

// Live tests against a real PostgreSQL. Run them with:
//
//	FEATURELAYER_TEST_DSN='postgres://user:pass@localhost:5432/db?sslmode=disable' \
//	    go test -tags integration ./...
//
// Without the tag and DSN they are not built, so the default `go test`
// stays database-free. The two ports' contracts are not re-implemented
// here: the exported entitlementtest suites — the same checks the
// in-memory stores run — are driven against the live server, including
// the mass-race check that proves Increment's single statement admits
// exactly max units. What remains here is backend-specific: the DDL the
// suites cannot see and the drops ctx bridge end to end.
package dropsstore_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/bernardoforcillo/drops/pg"
	"github.com/bernardoforcillo/drops/stdlib"
	_ "github.com/jackc/pgx/v5/stdlib"

	featurelayer "github.com/bernardoforcillo/featurelayer"
	"github.com/bernardoforcillo/featurelayer/catalog"
	"github.com/bernardoforcillo/featurelayer/entitlement"
	"github.com/bernardoforcillo/featurelayer/entitlement/entitlementtest"
	dropsstore "github.com/bernardoforcillo/featurelayer/store/drops"
)

// liveDB opens FEATURELAYER_TEST_DSN or skips. Close is registered as
// a cleanup FIRST so it runs LAST: t.Cleanup runs LIFO, and the
// drop-tables cleanups registered afterwards still need the pool.
func liveDB(t *testing.T) *pg.DB {
	t.Helper()
	dsn := os.Getenv("FEATURELAYER_TEST_DSN")
	if dsn == "" {
		t.Skip("set FEATURELAYER_TEST_DSN to run the drops store integration tests")
	}
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return pg.New(stdlib.New(sqlDB))
}

// freshStores drops and recreates both tables so every subtest starts
// empty. Each subtest gets its own table names so the contract suites'
// subtests, which each call newStore, never see each other's rows.
func freshStores(t *testing.T, db *pg.DB, names dropsstore.Names) (*dropsstore.SubscriptionStore, *dropsstore.UsageStore) {
	t.Helper()
	ctx := context.Background()
	subs := dropsstore.NewSubscriptionStore(db, dropsstore.WithNames(names))
	usage := dropsstore.NewUsageStore(db, dropsstore.WithNames(names))
	drop := func() {
		for _, tbl := range []*pg.Table{subs.Schema().Subscriptions, usage.Schema().Usage} {
			if _, err := db.ExecExpr(ctx, pg.DropTableIfExists(tbl)); err != nil {
				t.Fatalf("drop %s: %v", tbl.Name(), err)
			}
		}
	}
	drop()
	if err := subs.CreateSchema(ctx); err != nil {
		t.Fatalf("SubscriptionStore.CreateSchema: %v", err)
	}
	if err := usage.CreateSchema(ctx); err != nil {
		t.Fatalf("UsageStore.CreateSchema: %v", err)
	}
	t.Cleanup(drop)
	return subs, usage
}

var testNames = dropsstore.Names{Subscriptions: "fl_test_subscriptions", Usage: "fl_test_usage"}

func TestSubscriptionStoreContractLive(t *testing.T) {
	db := liveDB(t)
	entitlementtest.RunSubscriptionStoreContract(t, func(t *testing.T) (entitlement.SubscriptionStore, entitlement.Seeder) {
		subs, _ := freshStores(t, db, testNames)
		return subs, subs
	})
}

func TestUsageStoreContractLive(t *testing.T) {
	db := liveDB(t)
	entitlementtest.RunUsageStoreContract(t, func(t *testing.T) entitlement.UsageStore {
		_, usage := freshStores(t, db, testNames)
		return usage
	})
}

func TestCreateSchemaIsIdempotent(t *testing.T) {
	db := liveDB(t)
	subs, usage := freshStores(t, db, testNames)
	ctx := context.Background()
	// A second run against existing tables — including the composite
	// primary key's guarded ALTER TABLE — must be a clean no-op.
	for i := 0; i < 2; i++ {
		if err := subs.CreateSchema(ctx); err != nil {
			t.Fatalf("SubscriptionStore.CreateSchema rerun %d: %v", i, err)
		}
		if err := usage.CreateSchema(ctx); err != nil {
			t.Fatalf("UsageStore.CreateSchema rerun %d: %v", i, err)
		}
	}
	// The key is really there: a second row for the same key must
	// conflict into the update path rather than insert a duplicate.
	key := entitlement.UsageKey{Tenant: "acme", Feature: "api.calls", Period: "p"}
	for i := 0; i < 3; i++ {
		if _, _, err := usage.Increment(ctx, key, 1, -1); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := usage.Get(ctx, key); got != 3 {
		t.Errorf("total after 3 increments = %d; a missing primary key would leave three rows", got)
	}
	n, err := db.Select().From(usage.Schema().Usage).Count(ctx)
	if err != nil || n != 1 {
		t.Errorf("rows = %d, %v; want 1", n, err)
	}
}

func TestEngineOverLiveStoresWithDropsContext(t *testing.T) {
	db := liveDB(t)
	subs, usage := freshStores(t, db, testNames)
	ctx := context.Background()
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

	snap, err := featurelayer.NewSnapshot(featurelayer.Config{
		Features: []catalog.Feature{{Key: "ai.tokens", Lifecycle: catalog.GA}, {Key: "api.calls", Lifecycle: catalog.GA}},
		Plans: []entitlement.Plan{{ID: "pro", Entitlements: []entitlement.Entitlement{
			entitlement.LimitedPer("ai.tokens", 5, entitlement.Day, entitlement.PerSubject),
			entitlement.Limited("api.calls", 10, entitlement.Month),
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	anchor := now.AddDate(0, -1, -5)
	if err := subs.Set(ctx, entitlement.Subscription{TenantID: "acme", Plan: "pro", BillingAnchor: anchor}); err != nil {
		t.Fatal(err)
	}
	engine := featurelayer.New(snap,
		featurelayer.WithSubscriptions(subs),
		featurelayer.WithUsage(usage),
		featurelayer.WithClock(func() time.Time { return now }),
	)

	// The drops ctx an authlayer request carries is enough to evaluate.
	reqCtx := pg.WithSubject(pg.WithTenant(ctx, "acme"), "u-1")
	ec, ok := dropsstore.EvalContextFrom(reqCtx)
	if !ok {
		t.Fatal("EvalContextFrom")
	}
	reqCtx = featurelayer.NewContext(reqCtx, ec)

	d, err := engine.ConsumeCtx(reqCtx, "ai.tokens", 5)
	if err != nil || !d.Enabled || d.Usage.Used != 5 || d.Usage.Remaining != 0 {
		t.Fatalf("per-subject consume: %+v %v", d.Usage, err)
	}
	d, err = engine.ConsumeCtx(reqCtx, "ai.tokens", 1)
	if err != nil || d.Enabled || d.Reason != featurelayer.ReasonLimitReached {
		t.Errorf("per-subject refusal: %+v %v", d, err)
	}
	// Another subject of the same tenant has its own row.
	other := featurelayer.NewContext(ctx, featurelayer.EvalContext{TenantID: "acme", UserID: "u-2"})
	if d, err := engine.ConsumeCtx(other, "ai.tokens", 1); err != nil || !d.Enabled || d.Usage.Used != 1 {
		t.Errorf("second subject: %+v %v", d.Usage, err)
	}
	// A tenant-only ctx cannot meter a per-subject limit, and can meter
	// a tenant one, against the billing-anchored period.
	tenantOnly, _ := dropsstore.EvalContextFrom(pg.WithTenant(ctx, "acme"))
	tenantCtx := featurelayer.NewContext(ctx, tenantOnly)
	if d, err := engine.ConsumeCtx(tenantCtx, "ai.tokens", 1); err != nil || d.Enabled || d.Detail != "no subject" {
		t.Errorf("no subject: %+v %v", d, err)
	}
	d, err = engine.ConsumeCtx(tenantCtx, "api.calls", 4)
	if err != nil || !d.Enabled || d.Usage.Used != 4 || d.Usage.Period != "2026-09-10T12:00:00Z" {
		t.Errorf("tenant consume: %+v %v", d.Usage, err)
	}
	// Direct check of the rows the engine wrote: the day period is
	// anchored too, so its key is the anchor's time of day.
	period := entitlement.PeriodKey(entitlement.Day, anchor, now)
	if got, _ := usage.Get(ctx, entitlement.UsageKey{Tenant: "acme", Feature: "ai.tokens", Period: period, Subject: "u-1"}); got != 5 {
		t.Errorf("u-1 row = %d", got)
	}
	if got, _ := usage.Get(ctx, entitlement.UsageKey{Tenant: "acme", Feature: "ai.tokens", Period: period}); got != 0 {
		t.Errorf("tenant-wide ai.tokens row must not exist, got %d", got)
	}
}
