package dropsstore

import (
	"context"
	"fmt"

	"github.com/bernardoforcillo/drops/pg"

	featurelayer "github.com/bernardoforcillo/featurelayer"
)

// EvalContextFrom builds a featurelayer.EvalContext from the tenant and
// subject drops carries on ctx — pg.WithTenant and pg.WithSubject. It
// returns false when there is no tenant: without one the engine has
// nothing to evaluate against, and every entitled feature would fail
// closed anyway. The subject is optional (a tenant-level job has none);
// it becomes EvalContext.UserID.
//
// This is the bridge to an authlayer application without importing
// authlayer: authlayer's scope.WithScope and scope.WithSubject are
// drops' pg.WithTenant and pg.WithSubject, so the ctx an authlayer
// request already carries is enough:
//
//	if ec, ok := dropsstore.EvalContextFrom(ctx); ok {
//	    ctx = featurelayer.NewContext(ctx, ec)
//	}
//	d := engine.EvaluateCtx(ctx, "export.csv")
//
// Values are accepted as string or fmt.Stringer (a UUID type, say);
// anything else is treated as absent, since guessing at a
// representation would silently evaluate the wrong tenant.
func EvalContextFrom(ctx context.Context) (featurelayer.EvalContext, bool) {
	tenant, ok := idString(pg.TenantFrom(ctx))
	if !ok || tenant == "" {
		return featurelayer.EvalContext{}, false
	}
	ec := featurelayer.EvalContext{TenantID: tenant}
	if subject, ok := idString(pg.SubjectFrom(ctx)); ok {
		ec.UserID = subject
	}
	return ec, true
}

// Resolver returns an EvalContextFrom variant that lets enrich add
// attributes (or adjust anything else) before the context is handed
// back. Use it to carry the attribute conventions flags target —
// principal_kind, api_key, client_id — from whatever your
// authentication layer puts on ctx:
//
//	resolve := dropsstore.Resolver(func(ctx context.Context, ec *featurelayer.EvalContext) {
//	    if p, ok := apikey.PrincipalFrom(ctx); ok {
//	        ec.Attributes = map[string]any{"principal_kind": "service_account", "api_key": p.KeyID}
//	    }
//	})
//	ec, ok := resolve(ctx)
//
// enrich runs only when a tenant was found; a nil enrich makes Resolver
// equivalent to EvalContextFrom.
func Resolver(enrich func(context.Context, *featurelayer.EvalContext)) func(context.Context) (featurelayer.EvalContext, bool) {
	return func(ctx context.Context) (featurelayer.EvalContext, bool) {
		ec, ok := EvalContextFrom(ctx)
		if !ok {
			return featurelayer.EvalContext{}, false
		}
		if enrich != nil {
			enrich(ctx, &ec)
		}
		return ec, true
	}
}

// idString renders a drops ctx value as an id. Only strings and
// Stringers qualify; see EvalContextFrom.
func idString(v any, present bool) (string, bool) {
	if !present {
		return "", false
	}
	switch x := v.(type) {
	case string:
		return x, true
	case fmt.Stringer:
		return x.String(), true
	}
	return "", false
}
