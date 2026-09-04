# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project aims to follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
once a 1.0 is cut. Until then, minor versions may break API.

## [Unreleased]

### Added

- **Per-subject limits** (`entitlement`). `Limit` gains `Per LimitScope`
  (`json:"per,omitempty"`): `""` or `"tenant"` (`PerTenant`) meters one counter
  per tenant as before, `"subject"` (`PerSubject`) meters one per
  `EvalContext.UserID`. `UsageKey` gains `Subject`, empty for tenant-scoped
  counters, so existing counters keep their keys. `LimitedPer(feature, max,
  period, scope)` sits beside `Limited`; `Limit.Scope()` returns the effective
  scope; `LimitScope.Valid()` reports the three accepted spellings.
  `Consume`/`Usage` set `Subject` for per-subject limits and **fail closed on
  an empty `UserID`** — an off decision with `Reason not_entitled` and `Detail
  "no subject"`, nothing charged — because charging the tenant's shared counter
  would silently widen the limit. `Evaluate` is unaffected. `NewSnapshot`
  rejects unknown `per` values (`…limit.per`) and, as it already does for the
  period, a scope disagreement between plan and add-on limits summed for one
  feature. A grant carrying an invalid scope (which validation never sees)
  fails closed with `Detail "invalid limit scope"`.
- **`entitlement.Seeder`** — the write side a `SubscriptionStore` may offer:
  `Set(ctx, Subscription) error` (upsert; `ErrEmptyTenantID` for an empty
  tenant) and `Delete(ctx, tenantID) error` (no-op when unknown).
  `MemSubscriptions.Seeder()` adapts the in-memory store to it; the existing
  `Set(sub)` / `Delete(tenantID)` are unchanged.
- **`entitlement/entitlementtest`** — the exported contract suite both shipped
  stores run: `RunSubscriptionStoreContract(t, newStore)` and
  `RunUsageStoreContract(t, newStore)`. The usage suite includes a mass-race
  check proving `Increment` admits exactly `max` units under concurrency, plus
  refusal-leaves-total-unchanged, first-increment-over-max, unlimited
  (`max < 0`), per-key isolation across tenant/feature/period/subject, and
  `Get` on an unknown key is 0.
- **Context adapter** (root package). `NewContext(ctx, ec)` / `FromContext(ctx)`
  and the `*Ctx` engine methods `EvaluateCtx`, `IsEnabledCtx`, `ConsumeCtx`,
  `UsageCtx`. A context without an `EvalContext` yields a fail-closed decision
  with the new `Reason no_context` (`ReasonNoContext`) and `ErrNoEvalContext`
  (also returned as the error by `ConsumeCtx`/`UsageCtx`); decision hooks still
  fire. Attribute conventions `principal_kind`, `api_key`, `client_id`
  documented in the readme — conventions only, nothing enforces them.
- **Loaders.** `LoadJSON(r)` and `LoadFile(path)` decode one JSON `Config`
  strictly — unknown fields and trailing data are errors — and build the
  snapshot; `(*Engine).Reload(load)` applies on success (apply hooks fire) and
  on failure keeps the old snapshot and returns the error. `ErrNilSnapshot`
  for a loader returning neither.
- **`cmd/featurelayer-validate`** — a standard-library CLI that loads a JSON
  config from a path or stdin, prints a one-line summary or every validation
  error, and exits 0 / 1 (invalid or unreadable) / 2 (usage). `-q` silences the
  success line. `testdata/config.json` and `testdata/invalid.json` exercise it.
- **`store/drops` submodule** (`github.com/bernardoforcillo/featurelayer/store/drops`,
  package `dropsstore`) — PostgreSQL `SubscriptionStore` (+ `Seeder`) and
  `UsageStore` over drops v0.6.0, as a separate Go module so the root stays
  standard-library only. Subscriptions are one table with scalar columns and
  `addons`/`trial`/`grants` as JSONB (the readme records why); `Increment` is
  a single `INSERT … SELECT … WHERE … ON CONFLICT (tenant, feature, period,
  subject) DO UPDATE SET total = total + EXCLUDED.total WHERE … RETURNING total`
  statement with a fallback `SELECT` for the unchanged total; `CreateSchema` is
  idempotent and emits the composite primary key itself; `WithNames`,
  `WithClock`, `Schema()`. `EvalContextFrom(ctx)` builds an `EvalContext` from
  drops' `pg.TenantFrom` / `pg.SubjectFrom` (the authlayer bridge without
  importing authlayer) and `Resolver(enrich)` adds attributes. Integration tests
  behind `//go:build integration` run both contract suites against
  `FEATURELAYER_TEST_DSN`.
- `changelog.md` (this file) and readme sections: Loading & validation,
  Per-subject limits, Context, Attribute conventions, Storage.

### Changed

- `MemSubscriptions.Set` and `Subscription` now **deep-copy**: mutating a
  `Subscription` after `Set`, or one returned by `Subscription`, no longer
  reaches the stored value (previously the returned value was a shallow copy
  sharing grant, limit and trial pointers). The contract suite requires this
  of every store.
- `entitlement.Resolution.Limit` for a plan/add-on sum carries the agreed
  scope (`Per`) when any source spells it out explicitly; a sum of limits that
  all leave `per` empty still has `Per == ""`.
- Doc comments added to every exported symbol in `entitlement` that lacked
  one (`PlanID`, `AddOnID`, `Kind`, `Limit`, `Entitlement`, `Plan`, `AddOn`,
  `Subscription`, `PlanTrial`, `Grant`, the `Mem*` constructors and methods).
- Unchecked write results in `flags/evaluator.go` (hash writes) and a test are
  now discarded explicitly so `golangci-lint run` is clean; no behaviour change.
