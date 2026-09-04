# featurelayer/store/drops

PostgreSQL persistence for featurelayer's two tenant-state ports —
`entitlement.SubscriptionStore` and `entitlement.UsageStore` — on top of
[drops](https://github.com/bernardoforcillo/drops), plus the bridge from
drops' request context to a `featurelayer.EvalContext`.

It is a **separate Go module**. The root `featurelayer` module is standard
library only and stays that way; importing this package is what brings drops
(and, for the integration tests, pgx) into your build.

    go get github.com/bernardoforcillo/featurelayer/store/drops

## Wiring

```go
import (
	"database/sql"

	"github.com/bernardoforcillo/drops/pg"
	"github.com/bernardoforcillo/drops/stdlib"
	_ "github.com/jackc/pgx/v5/stdlib"

	featurelayer "github.com/bernardoforcillo/featurelayer"
	dropsstore "github.com/bernardoforcillo/featurelayer/store/drops"
)

sqlDB, _ := sql.Open("pgx", dsn)
db := pg.New(stdlib.New(sqlDB))

subs := dropsstore.NewSubscriptionStore(db)
usage := dropsstore.NewUsageStore(db)
if err := subs.CreateSchema(ctx); err != nil { /* ... */ }
if err := usage.CreateSchema(ctx); err != nil { /* ... */ }

engine := featurelayer.New(snap,
	featurelayer.WithSubscriptions(subs),
	featurelayer.WithUsage(usage))

// Billing state is written through the same store the engine reads.
_ = subs.Set(ctx, entitlement.Subscription{TenantID: "acme", Plan: "pro"})
```

`WithNames(Names{Subscriptions: ..., Usage: ...})` renames the tables;
`WithClock` overrides the `updated_at` stamp. Both stores expose `Schema()`
for deployments that emit their own DDL.

## Tables

```
feature_subscriptions   tenant_id text PRIMARY KEY, plan text, addons jsonb, trial jsonb,
                        grants jsonb, billing_anchor timestamptz NULL, updated_at timestamptz
feature_usage           tenant text, feature text, period text, subject text, total bigint,
                        PRIMARY KEY (tenant, feature, period, subject)
```

**Why `addons`, `trial` and `grants` are JSONB.** `entitlement.Subscription`
is small and application-shaped: a plan id, a few add-on ids, at most one
trial, a handful of grants. The engine reads it whole on every gated
evaluation and the application writes it whole when billing changes; the port
has no method that queries *across* tenants by grant or add-on, so
normalising the lists into child tables would cost three joins per read and a
multi-statement write for a query nobody makes. The scalars an operator does
filter by — `tenant_id`, `plan`, `billing_anchor` — are columns. The JSONB
columns hold exactly the `encoding/json` form of the corresponding fields
(the same shape the config file uses), so a row reads and edits cleanly in
psql, and PostgreSQL's jsonb operators are there if you ever need to report
over grants. Lists are stored as `[]` and a missing trial as JSON `null`, so
the columns are `NOT NULL` and there is no NULL-versus-empty ambiguity.
`billing_anchor` is nullable: NULL is the zero anchor (UTC-calendar periods).

**The usage primary key is the whole `UsageKey`.** `(tenant, feature, period,
subject)` with `""` standing for "no period" and "no subject": a primary key
cannot hold NULL and two NULLs would not conflict, so the tenant-scoped
counter is the row whose `subject` is `""`. That key is the `ON CONFLICT`
arbiter `Increment` relies on. drops' `CREATE TABLE` carries single-column
keys only, so `CreateSchema` emits it itself as a guarded `ALTER TABLE`.

## `Increment` is one statement

```sql
INSERT INTO feature_usage (tenant, feature, period, subject, total)
SELECT $1, $2, $3, $4, $delta
 WHERE $max < 0 OR $delta <= $max
ON CONFLICT (tenant, feature, period, subject) DO UPDATE
   SET total = feature_usage.total + EXCLUDED.total
 WHERE $max < 0 OR feature_usage.total + EXCLUDED.total <= $max
RETURNING total
```

The two `WHERE` clauses are the compare-and-set, one per path. PostgreSQL
evaluates the `ON CONFLICT ... WHERE` against the row's latest committed
version under the row lock it takes, so of any number of concurrent callers
each sees every earlier winner's total and **exactly `max` units are ever
admitted** — the property the engine's `limit_reached` decision rests on and
the one a `SELECT` then `UPDATE` gets wrong. The `INSERT ... SELECT ... WHERE`
guards the no-row path: a first delta above `max` must not create the counter,
and a plain `VALUES` insert would. When the statement returns no row, the
unchanged total is read back with a plain `SELECT` (0 when the counter does
not exist) so the caller gets what the port promises.

The statement is raw SQL rendered through drops' builder — identifiers quoted,
values bound — because the builder writes `VALUES`, not `SELECT ... WHERE`.

## `CreateSchema`

Idempotent: `CREATE TABLE IF NOT EXISTS` plus, for the usage table, the
composite key wrapped in a plpgsql block that swallows the "already there"
SQLSTATEs (PostgreSQL has no `ADD CONSTRAINT IF NOT EXISTS`). It adds what is
missing and **never alters what exists** — it will not migrate a table whose
columns differ, and it says nothing when they do. Production deployments that
own their migrations should take the DDL from `Schema()` and skip the call.

## Context bridge

```go
// drops' pg.WithTenant / pg.WithSubject are what authlayer's
// scope.WithScope / scope.WithSubject put on ctx.
if ec, ok := dropsstore.EvalContextFrom(ctx); ok {
	ctx = featurelayer.NewContext(ctx, ec)
}
d := engine.EvaluateCtx(ctx, "export.csv")
```

`EvalContextFrom` reads `pg.TenantFrom` (required — no tenant, no context)
and `pg.SubjectFrom` (optional; becomes `UserID`). Values must be strings or
`fmt.Stringer`s; anything else is treated as absent rather than guessed at.
This is the authlayer bridge without importing authlayer.

`Resolver(enrich)` wraps it so you can add the attributes flags target —
`principal_kind`, `api_key`, `client_id` — from whatever your auth layer puts
on ctx:

```go
resolve := dropsstore.Resolver(func(ctx context.Context, ec *featurelayer.EvalContext) {
	if p, ok := apikey.PrincipalFrom(ctx); ok {
		ec.Attributes = map[string]any{"principal_kind": "service_account", "api_key": p.KeyID}
	}
})
```

## Tests

`go test ./...` is database-free. The live lane runs both exported contract
suites (`entitlement/entitlementtest`) against a real server — including the
mass-race check on `Increment` — plus the DDL and the context bridge end to
end:

    FEATURELAYER_TEST_DSN='postgres://user:pass@localhost:5432/db?sslmode=disable' \
        go test -tags integration ./...

Without the tag and DSN the file is not built; with the tag and no DSN it
skips.

## What is not here

- No caching. Every gated evaluation reads the subscription row; put a cache
  in front of `SubscriptionStore` if that matters, and remember the engine
  fails closed on any error it returns.
- No usage reset or purge. Rows for past periods stay; the period key is part
  of the primary key, so they never collide with the current period, and a
  `DELETE ... WHERE period < $1` is the application's to schedule.
- No foreign keys to your tenants table: the store does not know it.
