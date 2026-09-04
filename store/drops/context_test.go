package dropsstore

import (
	"context"
	"testing"

	"github.com/bernardoforcillo/drops/pg"

	featurelayer "github.com/bernardoforcillo/featurelayer"
)

type stringerID string

func (s stringerID) String() string { return "id:" + string(s) }

func TestEvalContextFrom(t *testing.T) {
	ctx := context.Background()
	if _, ok := EvalContextFrom(ctx); ok {
		t.Error("no tenant must be absent")
	}
	if _, ok := EvalContextFrom(pg.WithSubject(ctx, "u-1")); ok {
		t.Error("a subject without a tenant must be absent")
	}
	if _, ok := EvalContextFrom(pg.WithTenant(ctx, "")); ok {
		t.Error("an empty tenant must be absent")
	}
	if _, ok := EvalContextFrom(pg.WithTenant(ctx, 42)); ok {
		t.Error("a non-string tenant must be absent rather than guessed")
	}
	ec, ok := EvalContextFrom(pg.WithTenant(ctx, "acme"))
	if !ok || ec.TenantID != "acme" || ec.UserID != "" || ec.Attributes != nil {
		t.Errorf("tenant only = %+v %v", ec, ok)
	}
	ec, ok = EvalContextFrom(pg.WithSubject(pg.WithTenant(ctx, stringerID("t")), stringerID("u")))
	if !ok || ec.TenantID != "id:t" || ec.UserID != "id:u" {
		t.Errorf("stringers = %+v %v", ec, ok)
	}
	// A subject of an unusable type is dropped, the tenant still resolves.
	ec, ok = EvalContextFrom(pg.WithSubject(pg.WithTenant(ctx, "acme"), 7))
	if !ok || ec.UserID != "" {
		t.Errorf("bad subject = %+v %v", ec, ok)
	}
}

func TestResolver(t *testing.T) {
	ctx := pg.WithSubject(pg.WithTenant(context.Background(), "acme"), "sa-1")
	calls := 0
	resolve := Resolver(func(_ context.Context, ec *featurelayer.EvalContext) {
		calls++
		ec.Attributes = map[string]any{"principal_kind": "service_account", "api_key": "key-1"}
	})
	ec, ok := resolve(ctx)
	if !ok || ec.TenantID != "acme" || ec.UserID != "sa-1" || ec.Attributes["principal_kind"] != "service_account" {
		t.Errorf("enriched = %+v %v", ec, ok)
	}
	if _, ok := resolve(context.Background()); ok || calls != 1 {
		t.Errorf("enrich must not run without a tenant: ok=%v calls=%d", ok, calls)
	}
	if ec, ok := Resolver(nil)(ctx); !ok || ec.UserID != "sa-1" {
		t.Errorf("nil enrich = %+v %v", ec, ok)
	}
	// The result plugs straight into the root context adapter.
	if got, ok := featurelayer.FromContext(featurelayer.NewContext(ctx, ec)); !ok || got.TenantID != "acme" {
		t.Errorf("round trip through featurelayer.NewContext: %+v %v", got, ok)
	}
}
