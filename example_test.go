package featurelayer_test

import (
	"context"
	"fmt"
	"time"

	featurelayer "github.com/bernardoforcillo/featurelayer"
	"github.com/bernardoforcillo/featurelayer/catalog"
	"github.com/bernardoforcillo/featurelayer/entitlement"
	"github.com/bernardoforcillo/featurelayer/flags"
)

func Example() {
	snap, err := featurelayer.NewSnapshot(featurelayer.Config{
		Features: []catalog.Feature{
			{Key: "export.csv", Lifecycle: catalog.GA},
			{Key: "api.calls", Lifecycle: catalog.GA},
		},
		Segments: []flags.Segment{
			{Key: "design-partners", Rules: []flags.SegmentRule{
				{Conditions: []flags.Condition{{Attribute: "tenant", Op: flags.In, Values: []any{"acme"}}}},
			}},
		},
		Flags: []flags.Flag{
			{Feature: "export.csv", Enabled: true,
				Rules: []flags.Rule{
					{Name: "partners", Conditions: []flags.Condition{{Op: flags.InSegment, Value: "design-partners"}}, Serve: flags.Serve{On: true}},
				},
				Default: flags.Serve{Rollout: flags.Percent(20)},
			},
		},
		Plans: []entitlement.Plan{
			{ID: "free", Entitlements: []entitlement.Entitlement{entitlement.Limited("api.calls", 100, entitlement.Month)}},
			{ID: "pro", Extends: "free", Entitlements: []entitlement.Entitlement{
				{Feature: "export.csv"},
				entitlement.Limited("api.calls", 1000, entitlement.Month),
			}},
		},
	})
	if err != nil {
		panic(err)
	}

	subs := entitlement.NewMemSubscriptions()
	subs.Set(entitlement.Subscription{
		TenantID:      "acme",
		Plan:          "pro",
		BillingAnchor: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
	})

	engine := featurelayer.New(snap,
		featurelayer.WithSubscriptions(subs),
		featurelayer.WithUsage(entitlement.NewMemUsage()),
		featurelayer.WithClock(func() time.Time { return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC) }),
	)

	ctx := context.Background()
	ec := featurelayer.EvalContext{TenantID: "acme", UserID: "u-1"}

	d := engine.Evaluate(ctx, "export.csv", ec)
	fmt.Println("export.csv enabled:", d.Enabled, "reason:", d.Reason)

	d, _ = engine.Consume(ctx, "api.calls", ec, 25)
	fmt.Println("api.calls used:", d.Usage.Used, "remaining:", d.Usage.Remaining, "resets:", d.Usage.ResetsAt.Format(time.RFC3339))

	// Output:
	// export.csv enabled: true reason: flag_rule
	// api.calls used: 25 remaining: 975 resets: 2026-09-20T10:00:00Z
}
