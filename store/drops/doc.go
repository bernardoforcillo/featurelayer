// Package dropsstore implements featurelayer's two persistence ports —
// entitlement.SubscriptionStore and entitlement.UsageStore — on top of
// the drops PostgreSQL toolkit, and bridges drops' request context
// (tenant and subject) to a featurelayer.EvalContext.
//
// It is a separate Go module so the root featurelayer module stays
// standard-library only: importing this package pulls in drops, and
// nothing else in featurelayer does.
//
//	db := pg.New(stdlib.New(sqlDB))
//	subs := dropsstore.NewSubscriptionStore(db)
//	usage := dropsstore.NewUsageStore(db)
//	if err := subs.CreateSchema(ctx); err != nil { ... }
//	if err := usage.CreateSchema(ctx); err != nil { ... }
//	engine := featurelayer.New(snap,
//	    featurelayer.WithSubscriptions(subs),
//	    featurelayer.WithUsage(usage))
//
// Both stores are pure persistence: they resolve nothing, meter nothing
// beyond the one atomic statement the port requires, and read no
// snapshot. The engine owns every decision.
package dropsstore
