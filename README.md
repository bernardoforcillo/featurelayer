# featurelayer

Feature management for Go SaaS applications: one library, one question —
**can tenant T (user U) use feature F right now, with which variant and limit?**

- **Catalog** — features with a lifecycle (`draft → beta → ga → deprecated → retired`), owners, tags, prerequisites.
- **Flags** — kill switch, time windows, attribute/regexp/semver targeting, reusable segments, deterministic percentage rollouts, variants.
- **Entitlements** — plans with inheritance, add-ons with plan prerequisites, per-tenant overrides and trials, metered limits with calendar- or billing-anchored periods.

Pure Go, standard library only, no server, no database. Definitions live in an
immutable hot-swappable snapshot; per-tenant state lives behind two small
interfaces (in-memory implementations included).

## Install

    go get github.com/bernardoforcillo/featurelayer

## Quick start

```go
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
```

## Fail-closed semantics

Every ambiguity resolves to "off": unknown features, draft/retired lifecycles,
missing tenants, store errors, unknown plans. **One deliberate exception:** if
you do not configure `WithSubscriptions`, the engine runs in *flags-only mode*
and gated features skip the commercial check. Configure the store in
production if you use plans.

## State model

| Definitions (snapshot) | Tenant state (interfaces) |
|---|---|
| features, segments, flags, plans, add-ons | subscriptions, usage counters |
| built in code or unmarshalled from JSON | `SubscriptionStore`, `UsageStore` |
| replaced atomically with `engine.Apply` | in-memory implementations included |

`json.Unmarshal` into `featurelayer.Config` + `NewSnapshot` **is** the config
file format — every type carries JSON tags.

## Observability

`WithDecisionHook` / `WithApplyHook` receive every decision (with reason,
entitlement source, usage and elapsed time) and every snapshot swap. Build
metrics and logging on top; the library ships none.

## Stability contracts

- Rollout bucketing is `fnv64a(seed + ":" + attr) % 10000 / 100` — stable
  across versions; changing it would reshuffle live rollouts.
- Usage period keys are the period start in RFC3339 UTC.

## License

MIT
