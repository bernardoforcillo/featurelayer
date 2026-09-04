package dropsstore

import (
	"context"
	"errors"

	"github.com/bernardoforcillo/drops"
	"github.com/bernardoforcillo/drops/pg"

	"github.com/bernardoforcillo/featurelayer/entitlement"
)

// UsageStore is a drops-backed entitlement.UsageStore: one row per
// entitlement.UsageKey, one atomic statement per Increment.
type UsageStore struct {
	db *pg.DB
	s  *Schema
}

// Compile-time proof the store satisfies the port.
var _ entitlement.UsageStore = (*UsageStore)(nil)

// NewUsageStore returns a UsageStore over db.
func NewUsageStore(db *pg.DB, opts ...Option) *UsageStore {
	return &UsageStore{db: db, s: NewSchema(opts...)}
}

// Schema exposes the table definitions, to emit DDL or join against.
func (st *UsageStore) Schema() *Schema { return st.s }

// CreateSchema issues CREATE TABLE IF NOT EXISTS for the usage table
// followed by its composite PRIMARY KEY (tenant, feature, period,
// subject) as a guarded ALTER TABLE — drops' CREATE TABLE carries
// single-column keys only, and this key is load-bearing: it is the ON
// CONFLICT arbiter Increment relies on, so without it every Increment
// would insert a fresh row and no limit would ever be reached. Every
// statement is idempotent and the call is safe to re-run; like
// SubscriptionStore.CreateSchema it adds and never migrates.
func (st *UsageStore) CreateSchema(ctx context.Context) error {
	return createTable(ctx, st.db, st.s.Usage)
}

// totalRow scans the single column the usage statements return.
type totalRow struct {
	Total int64 `drop:"total"`
}

// Get returns the counter for key, 0 when there is no row.
func (st *UsageStore) Get(ctx context.Context, key entitlement.UsageKey) (int64, error) {
	var row totalRow
	err := st.db.Select(st.s.usage.total).From(st.s.Usage).
		Where(st.keyPredicates(key)...).
		One(ctx, &row)
	if err != nil {
		if errors.Is(err, pg.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return row.Total, nil
}

// Increment is the port's check-and-add as ONE statement:
//
//	INSERT INTO <feature_usage> (tenant, feature, period, subject, total)
//	SELECT $1, $2, $3, $4, $5
//	 WHERE $6 < 0 OR $5 <= $6
//	ON CONFLICT (tenant, feature, period, subject) DO UPDATE
//	   SET total = <feature_usage>.total + EXCLUDED.total
//	 WHERE $6 < 0 OR <feature_usage>.total + EXCLUDED.total <= $6
//	RETURNING total
//
// with $5 = delta and $6 = max. The two WHERE clauses are the
// compare-and-set, one per path. The ON CONFLICT ... WHERE gates the
// update of an existing row: PostgreSQL evaluates it against the row's
// latest committed version under the row lock ON CONFLICT takes, so of
// any number of concurrent callers each one sees every earlier winner's
// total and exactly max units are ever admitted — the property the
// engine's limit_reached decision rests on, and the one a SELECT then
// UPDATE gets wrong. The INSERT ... SELECT ... WHERE gates the no-row
// path: a first delta above max must not create the counter, and a
// plain VALUES insert would. Two concurrent first Increments on a fresh
// key are the same race as two concurrent inserts anywhere; the
// primary key turns the loser into the DO UPDATE branch, which then
// re-checks against the winner's total.
//
// The statement returns no row when it admitted nothing; the total is
// then read back with a plain SELECT so the caller gets the unchanged
// value the port promises (0 when the counter does not exist). That
// read never writes, so it cannot reopen the atomicity above.
//
// It is raw SQL rendered through drops' builder — identifiers quoted,
// values bound — because drops' InsertBuilder writes VALUES, not
// SELECT ... WHERE, and the no-row guard needs the latter.
func (st *UsageStore) Increment(ctx context.Context, key entitlement.UsageKey, delta, max int64) (int64, bool, error) {
	sql, args := drops.String(st.incrementExpr(key, delta, max))
	rows, err := st.db.Query(ctx, sql, args...)
	if err != nil {
		return 0, false, err
	}
	var row totalRow
	err = pg.ScanOne(rows, &row)
	if err == nil {
		return row.Total, true, nil
	}
	if !errors.Is(err, pg.ErrNoRows) {
		return 0, false, err
	}
	total, err := st.Get(ctx, key)
	if err != nil {
		return 0, false, err
	}
	return total, false, nil
}

// incrementExpr renders the Increment statement. Column and table names
// go through the builder's identifier quoting; delta and max are bound
// (each occurrence binds again — PostgreSQL does not care and the
// builder has no parameter reuse).
func (st *UsageStore) incrementExpr(key entitlement.UsageKey, delta, max int64) drops.Expression {
	t, c := st.s.Usage, st.s.usage
	total := drops.ExprFunc(func(b *drops.Builder) {
		b.WriteQualified(t.Schema(), t.Name())
		b.WriteString(".")
		b.WriteIdent(c.total.Name())
	})
	return drops.ExprFunc(func(b *drops.Builder) {
		b.WriteString("INSERT INTO ")
		b.WriteQualified(t.Schema(), t.Name())
		b.WriteString(" (")
		for i, col := range []*pg.Column{c.tenant.Column, c.feature.Column, c.period.Column, c.subject.Column, c.total.Column} {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteIdent(col.Name())
		}
		b.WriteString(") SELECT ")
		b.AddArg(key.Tenant)
		b.WriteString("::text, ")
		b.AddArg(string(key.Feature))
		b.WriteString("::text, ")
		b.AddArg(key.Period)
		b.WriteString("::text, ")
		b.AddArg(key.Subject)
		b.WriteString("::text, ")
		b.AddArg(delta)
		b.WriteString("::bigint WHERE ")
		b.AddArg(max)
		b.WriteString("::bigint < 0 OR ")
		b.AddArg(delta)
		b.WriteString("::bigint <= ")
		b.AddArg(max)
		b.WriteString("::bigint ON CONFLICT (")
		for i, col := range []*pg.Column{c.tenant.Column, c.feature.Column, c.period.Column, c.subject.Column} {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteIdent(col.Name())
		}
		b.WriteString(") DO UPDATE SET ")
		b.WriteIdent(c.total.Name())
		b.WriteString(" = ")
		b.Append(total)
		b.WriteString(" + EXCLUDED.")
		b.WriteIdent(c.total.Name())
		b.WriteString(" WHERE ")
		b.AddArg(max)
		b.WriteString("::bigint < 0 OR ")
		b.Append(total)
		b.WriteString(" + EXCLUDED.")
		b.WriteIdent(c.total.Name())
		b.WriteString(" <= ")
		b.AddArg(max)
		b.WriteString("::bigint RETURNING ")
		b.WriteIdent(c.total.Name())
	})
}

func (st *UsageStore) keyPredicates(key entitlement.UsageKey) []drops.Expression {
	c := st.s.usage
	return []drops.Expression{
		c.tenant.Eq(key.Tenant),
		c.feature.Eq(string(key.Feature)),
		c.period.Eq(key.Period),
		c.subject.Eq(key.Subject),
	}
}
