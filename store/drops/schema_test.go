package dropsstore

import (
	"strings"
	"testing"

	"github.com/bernardoforcillo/drops"
	"github.com/bernardoforcillo/drops/pg"

	"github.com/bernardoforcillo/featurelayer/entitlement"
)

func TestNamesDefaultsAndOverride(t *testing.T) {
	s := NewSchema()
	if s.Subscriptions.Name() != "feature_subscriptions" || s.Usage.Name() != "feature_usage" {
		t.Errorf("defaults = %q %q", s.Subscriptions.Name(), s.Usage.Name())
	}
	s = NewSchema(WithNames(Names{Usage: "billing_usage"}))
	if s.Subscriptions.Name() != "feature_subscriptions" || s.Usage.Name() != "billing_usage" {
		t.Errorf("partial override = %q %q", s.Subscriptions.Name(), s.Usage.Name())
	}
	if pk := s.Usage.CompositePrimaryKey(); len(pk) != 4 {
		t.Errorf("usage composite pk = %d columns, want 4", len(pk))
	}
}

func TestIncrementSQL(t *testing.T) {
	st := NewUsageStore(nil, WithNames(Names{Usage: "billing_usage"}))
	key := entitlement.UsageKey{Tenant: "acme", Feature: "api.calls", Period: "2026-09-01T00:00:00Z", Subject: "u-1"}
	sql, args := drops.String(st.incrementExpr(key, 3, 10))
	for _, want := range []string{
		`INSERT INTO "billing_usage" ("tenant", "feature", "period", "subject", "total") SELECT `,
		`::bigint WHERE $6::bigint < 0 OR $7::bigint <= $8::bigint`,
		`ON CONFLICT ("tenant", "feature", "period", "subject") DO UPDATE SET "total" = "billing_usage"."total" + EXCLUDED."total"`,
		`WHERE $9::bigint < 0 OR "billing_usage"."total" + EXCLUDED."total" <= $10::bigint RETURNING "total"`,
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL missing %q:\n%s", want, sql)
		}
	}
	wantArgs := []any{"acme", "api.calls", "2026-09-01T00:00:00Z", "u-1", int64(3), int64(10), int64(3), int64(10), int64(10), int64(10)}
	if len(args) != len(wantArgs) {
		t.Fatalf("args = %v", args)
	}
	for i := range wantArgs {
		if args[i] != wantArgs[i] {
			t.Errorf("args[%d] = %v, want %v", i, args[i], wantArgs[i])
		}
	}
}

func TestCreateSchemaDDL(t *testing.T) {
	s := NewSchema()
	sql, _ := drops.String(pg.CreateTableIfNotExists(s.Usage))
	for _, want := range []string{`CREATE TABLE IF NOT EXISTS "feature_usage"`, `"subject" text NOT NULL`, `"total" bigint NOT NULL`} {
		if !strings.Contains(sql, want) {
			t.Errorf("usage DDL missing %q:\n%s", want, sql)
		}
	}
	if strings.Contains(sql, "PRIMARY KEY") {
		t.Errorf("drops' CREATE TABLE must not carry the composite key (or createTable would emit it twice):\n%s", sql)
	}
	sql, _ = drops.String(addPrimaryKey(s.Usage, s.Usage.CompositePrimaryKey()))
	if !strings.Contains(sql, `ALTER TABLE "feature_usage" ADD CONSTRAINT "feature_usage_pkey" PRIMARY KEY ("tenant", "feature", "period", "subject")`) ||
		!strings.Contains(sql, "WHEN duplicate_table OR duplicate_object OR invalid_table_definition THEN NULL") {
		t.Errorf("pk DDL:\n%s", sql)
	}
	sql, _ = drops.String(pg.CreateTableIfNotExists(s.Subscriptions))
	for _, want := range []string{`"tenant_id" text PRIMARY KEY`, `"addons" jsonb NOT NULL`, `"trial" jsonb NOT NULL`, `"grants" jsonb NOT NULL`, `"billing_anchor" timestamptz`, `"updated_at" timestamptz NOT NULL`} {
		if !strings.Contains(sql, want) {
			t.Errorf("subscriptions DDL missing %q:\n%s", want, sql)
		}
	}
	if strings.Contains(sql, `"billing_anchor" timestamptz NOT NULL`) {
		t.Error("billing_anchor must be nullable: NULL is the zero anchor")
	}
}

func TestEncodeLists(t *testing.T) {
	a, tr, g, err := encodeLists(entitlement.Subscription{TenantID: "x"})
	if err != nil || a != "[]" || tr != "null" || g != "[]" {
		t.Errorf("empty subscription encodes as %q %q %q, %v", a, tr, g, err)
	}
	a, tr, g, err = encodeLists(entitlement.Subscription{
		AddOns: []entitlement.AddOnID{"x"},
		Trial:  &entitlement.PlanTrial{Plan: "pro"},
		Grants: []entitlement.Grant{{Feature: "f", Limit: &entitlement.Limit{Max: 1, Per: entitlement.PerSubject}}},
	})
	if err != nil || a != `["x"]` || !strings.Contains(tr, `"plan":"pro"`) || !strings.Contains(g, `"per":"subject"`) {
		t.Errorf("encoded %q %q %q, %v", a, tr, g, err)
	}
}
