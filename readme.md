# featurelayer

Feature management for Go SaaS applications: one library, one question —
**can tenant T (user U) use feature F right now, with which variant and limit?**

- **Catalog** — features with a lifecycle (`draft → beta → ga → deprecated → retired`), owners, tags, prerequisites.
- **Flags** — kill switch, time windows, attribute/regexp/semver targeting, reusable segments, deterministic percentage rollouts, variants.
- **Entitlements** — plans with inheritance, add-ons with plan prerequisites, per-tenant overrides and trials, metered limits with calendar- or billing-anchored periods, counted per tenant or per user.

Pure Go, standard library only, no server. Definitions live in an immutable
hot-swappable snapshot; per-tenant state lives behind two small interfaces —
in-memory implementations included, a PostgreSQL implementation in the
separate [`store/drops`](store/drops/readme.md) module.

## Install

    go get github.com/bernardoforcillo/featurelayer

    # optional: PostgreSQL stores (separate module, pulls in drops)
    go get github.com/bernardoforcillo/featurelayer/store/drops

    # optional: CI validator for the JSON config
    go install github.com/bernardoforcillo/featurelayer/cmd/featurelayer-validate@latest

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
		AddOns: []entitlement.AddOn{
			{ID: "extra-calls", Requires: []entitlement.PlanID{"pro"}, Entitlements: []entitlement.Entitlement{
				entitlement.Limited("api.calls", 500, entitlement.Month),
			}},
		},
	})
	if err != nil {
		panic(err)
	}

	subs := entitlement.NewMemSubscriptions()
	subs.Set(entitlement.Subscription{
		TenantID:      "acme",
		Plan:          "free",
		Trial:         &entitlement.PlanTrial{Plan: "pro", Until: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
		AddOns:        []entitlement.AddOnID{"extra-calls"},
		BillingAnchor: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
	})

	var decisions int
	engine := featurelayer.New(snap,
		featurelayer.WithSubscriptions(subs),
		featurelayer.WithUsage(entitlement.NewMemUsage()),
		featurelayer.WithClock(func() time.Time { return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC) }),
		featurelayer.WithDecisionHook(func(featurelayer.DecisionEvent) { decisions++ }),
	)

	ctx := context.Background()
	ec := featurelayer.EvalContext{TenantID: "acme", UserID: "u-1"}

	d := engine.Evaluate(ctx, "export.csv", ec)
	fmt.Println("export.csv enabled:", d.Enabled, "reason:", d.Reason)

	d, _ = engine.Consume(ctx, "api.calls", ec, 25)
	fmt.Println("api.calls used:", d.Usage.Used, "remaining:", d.Usage.Remaining, "resets:", d.Usage.ResetsAt.Format(time.RFC3339))

	fmt.Println("decisions observed:", decisions)

	// Output:
	// export.csv enabled: true reason: flag_rule
	// api.calls used: 25 remaining: 1475 resets: 2026-09-20T10:00:00Z
	// decisions observed: 2
}
```

## Fail-closed semantics

Every ambiguity resolves to "off": unknown features, draft/retired lifecycles,
missing tenants, store errors, unknown plans, a per-subject limit metered
without a subject, a `*Ctx` call on a context that carries no `EvalContext`.
**One deliberate exception:** if
you do not configure `WithSubscriptions`, the engine runs in *flags-only mode*
and gated features skip the commercial check. Configure the store in
production if you use plans.

## State model

| Definitions (snapshot) | Tenant state (interfaces) |
|---|---|
| features, segments, flags, plans, add-ons | subscriptions, usage counters |
| built in code or loaded from JSON | `SubscriptionStore`, `UsageStore` |
| replaced atomically with `engine.Apply` / `Reload` | in-memory and PostgreSQL implementations |

`json.Unmarshal` into `featurelayer.Config` + `NewSnapshot` **is** the config
file format — every type carries JSON tags. `LoadJSON` / `LoadFile` do the
two steps in one, strictly (see [Loading & validation](#loading--validation)).

## Loading & validation

```go
snap, err := featurelayer.LoadFile("features.json") // or LoadJSON(r)
```

`LoadJSON` decodes one JSON `Config` and runs `NewSnapshot`. Decoding is
**strict**: an unknown field anywhere is an error, and so is anything after
the first document. A typo such as `"entitlments"` would otherwise be silently
dropped and ship a plan with no entitlements — exactly the class of mistake the
validation exists to catch. Validation problems come back as `NewSnapshot`
returns them: an `errors.Join` of `*ValidationError`, one per problem, so you
see all of them at once.

Hot reload keeps a broken file from taking a running engine down:

```go
if err := engine.Reload(func() (*featurelayer.Snapshot, error) {
	return featurelayer.LoadFile("features.json")
}); err != nil {
	log.Printf("config not applied, still serving the previous snapshot: %v", err)
}
```

`Reload` applies the new snapshot on success (firing `WithApplyHook` as
`Apply` does) and on failure returns the error and changes nothing.

For CI, `cmd/featurelayer-validate` runs the same load and prints either a
one-line summary or every problem, exiting non-zero on failure:

    $ featurelayer-validate features.json
    features.json: ok — 4 features, 1 segments, 2 flags, 2 plans, 1 addons

    $ featurelayer-validate broken.json
    broken.json: features[1].key: invalid key "Bad Key"
    broken.json: plans[0].entitlements[0].limit.per: invalid scope "user"
    broken.json: invalid — 2 problem(s)

It reads stdin when given no path (or `-`); `-q` silences the success line.
Exit status: 0 valid, 1 invalid or unreadable, 2 usage error.

## Per-subject limits

A `Limit` meters one counter per tenant by default. `Per: "subject"` meters
one counter per **user of the tenant** instead — AI tokens per seat, exports
per user, API calls per service account:

```go
entitlement.LimitedPer("ai.tokens", 5000, entitlement.Day, entitlement.PerSubject)
// JSON: {"feature": "ai.tokens", "limit": {"max": 5000, "period": "day", "per": "subject"}}
```

`Consume` and `Usage` then key the counter by `(tenant, feature, period,
subject)` with `subject = EvalContext.UserID`; two users of one tenant never
share a counter and the tenant-wide counter is untouched. Plans and add-ons
that limit the same feature still sum, but must agree on the scope (as they
must on the period) — `NewSnapshot` rejects a per-tenant limit summed with a
per-subject one, and rejects any `per` value other than `""`, `"tenant"` or
`"subject"`.

**A per-subject limit metered without a subject fails closed.** `Consume` and
`Usage` with an empty `UserID` return an off decision — `Reason
not_entitled`, `Detail "no subject"` — and charge nothing. Charging the
tenant's shared counter instead would silently widen the limit. `Evaluate`
itself is unaffected: the scope only matters when metering, and whether the
feature is *on* for the tenant is a separate question.

The default is unchanged: `Limited(...)` and a `Limit` without `per` behave
exactly as before, and marshal without the field.

## Context

The `*Ctx` variants of the engine methods read the `EvalContext` from
`context.Context`, so the identity is established once — where the request is
authenticated — and every feature check downstream is a one-liner:

```go
ctx = featurelayer.NewContext(ctx, featurelayer.EvalContext{TenantID: tenant, UserID: user})

d := engine.EvaluateCtx(ctx, "export.csv")
ok := engine.IsEnabledCtx(ctx, "export.csv")
d, err := engine.ConsumeCtx(ctx, "api.calls", 1)
d, err = engine.UsageCtx(ctx, "api.calls")
```

`FromContext(ctx)` reads it back. A context that carries no `EvalContext` is a
wiring bug, and it fails closed like every other ambiguity: the decision is
off with `Reason no_context` and `Err ErrNoEvalContext`, `ConsumeCtx` /
`UsageCtx` also return `ErrNoEvalContext`, nothing is charged, and decision
hooks still fire so the miss is observable.

The library reads nothing else from `ctx`; how the tenant and user get there
is the application's business. If your stack already carries drops' tenant
and subject — which is what authlayer's `scope.WithScope` / `WithSubject`
put on `ctx` — `store/drops` offers `EvalContextFrom(ctx)` and
`Resolver(enrich)` to build the `EvalContext` from them without importing
authlayer.

### Attribute conventions

Flags target `EvalContext.Attributes` freely; the engine derives `tenant`,
`user`, `plan` and `addons` itself and overrides any caller-supplied value of
those four. Three more names are **conventions**, documented so applications
and shared flag configs agree on them. Nothing enforces them:

| attribute | values | meaning |
|---|---|---|
| `principal_kind` | `"user"`, `"service_account"`, `"delegated"` | who is calling: an interactive user, an API key's service account, or a token acting on behalf of a user |
| `api_key` | key id | the API key that authenticated, when `principal_kind` is `"service_account"` |
| `client_id` | OAuth client id | the client that obtained the token, when there is one |

A flag can then keep a feature away from automation
(`{"attribute": "principal_kind", "op": "eq", "value": "service_account"}`
serving off), pilot it with one client, or roll it out to API keys before
users. `store/drops.Resolver` is the hook for filling them from your
authentication layer.

## Storage

Per-tenant state lives behind two ports in package `entitlement`:

```go
type SubscriptionStore interface {
	Subscription(ctx context.Context, tenantID string) (*Subscription, error) // ErrNoSubscription when unknown
}
type UsageStore interface {
	Get(ctx context.Context, key UsageKey) (int64, error)                                     // 0 when unknown
	Increment(ctx context.Context, key UsageKey, delta, max int64) (total int64, allowed bool, err error)
}
type Seeder interface { // the write side a SubscriptionStore may offer
	Set(ctx context.Context, sub Subscription) error       // upsert; ErrEmptyTenantID for an empty tenant
	Delete(ctx context.Context, tenantID string) error     // no-op when unknown
}
```

`Increment` is the one method with a hard requirement: the check-and-add
**must be atomic**, admitting exactly `max` units under concurrency, refusing
without writing, and treating `max < 0` as unlimited. Everything the engine
says about limits rests on it.

Two implementations ship:

- **In-memory** — `entitlement.NewMemSubscriptions()` (with `Seeder()` for the
  ctx-taking write side) and `entitlement.NewMemUsage()`. Tests, demos, single
  process. Both deep-copy on the way in and out.
- **PostgreSQL** — the separate module
  [`store/drops`](store/drops/readme.md): `dropsstore.NewSubscriptionStore(db)`
  (also a `Seeder`) and `dropsstore.NewUsageStore(db)` over
  [drops](https://github.com/bernardoforcillo/drops), with an idempotent
  `CreateSchema`, `WithNames`, and `Increment` as a single
  `INSERT … ON CONFLICT DO UPDATE … WHERE … RETURNING` statement. It is its own
  module so this one stays standard-library only.

Writing your own: run the exported contract suite, which is what both shipped
stores run —

```go
import "github.com/bernardoforcillo/featurelayer/entitlement/entitlementtest"

func TestMyStores(t *testing.T) {
	entitlementtest.RunSubscriptionStoreContract(t, func(t *testing.T) (entitlement.SubscriptionStore, entitlement.Seeder) {
		s := newMySubscriptionStore(t) // empty; register teardown with t.Cleanup
		return s, s
	})
	entitlementtest.RunUsageStoreContract(t, func(t *testing.T) entitlement.UsageStore {
		return newMyUsageStore(t)
	})
}
```

The usage suite includes a mass-race check that proves exactly `max` units
are ever admitted; a read-then-write implementation fails it.

## Observability

`WithDecisionHook` / `WithApplyHook` receive every decision (with reason,
entitlement source, usage and elapsed time) and every snapshot swap. Build
metrics and logging on top; the library ships none.

## Stability contracts

- Rollout bucketing is `fnv64a(seed + ":" + attr) % 10000 / 100` — stable
  across versions; changing it would reshuffle live rollouts.
- Usage period keys are the period start in RFC3339 UTC.
- Usage counters are keyed by `(tenant, feature, period, subject)`; the
  subject is `""` for per-tenant limits, so existing counters are unchanged.

## License

MIT
