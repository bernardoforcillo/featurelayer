package dropsstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bernardoforcillo/drops"
	"github.com/bernardoforcillo/drops/pg"

	"github.com/bernardoforcillo/featurelayer/entitlement"
)

// SubscriptionStore is a drops-backed entitlement.SubscriptionStore
// that also implements entitlement.Seeder, so the application writes
// billing state through the same object the engine reads it from.
type SubscriptionStore struct {
	db  *pg.DB
	s   *Schema
	now func() time.Time
}

// Compile-time proof the store satisfies both ports.
var (
	_ entitlement.SubscriptionStore = (*SubscriptionStore)(nil)
	_ entitlement.Seeder            = (*SubscriptionStore)(nil)
)

// NewSubscriptionStore returns a SubscriptionStore over db.
func NewSubscriptionStore(db *pg.DB, opts ...Option) *SubscriptionStore {
	cfg := newSettings(opts)
	return &SubscriptionStore{db: db, s: NewSchema(opts...), now: cfg.now}
}

// Schema exposes the table definitions, to emit DDL or join against.
func (st *SubscriptionStore) Schema() *Schema { return st.s }

// CreateSchema issues CREATE TABLE IF NOT EXISTS for the subscriptions
// table. It is idempotent and safe to re-run; it adds what is missing
// and never alters what exists, so it will not migrate a table whose
// columns differ. Production deployments that own their migrations can
// skip it and take the DDL from Schema instead.
func (st *SubscriptionStore) CreateSchema(ctx context.Context) error {
	return createTable(ctx, st.db, st.s.Subscriptions)
}

// subscriptionRow is the table's shape for scanning. The three jsonb
// columns arrive as JSON text.
type subscriptionRow struct {
	TenantID      string     `drop:"tenant_id"`
	Plan          string     `drop:"plan"`
	AddOns        string     `drop:"addons"`
	Trial         string     `drop:"trial"`
	Grants        string     `drop:"grants"`
	BillingAnchor *time.Time `drop:"billing_anchor"`
}

// Subscription loads the tenant's row, mapping drops' ErrNoRows to
// entitlement.ErrNoSubscription. The returned value is freshly decoded
// from the row, so it shares nothing with any other caller.
func (st *SubscriptionStore) Subscription(ctx context.Context, tenantID string) (*entitlement.Subscription, error) {
	var row subscriptionRow
	err := st.db.Select().From(st.s.Subscriptions).
		Where(st.s.sub.tenantID.Eq(tenantID)).
		One(ctx, &row)
	if err != nil {
		if errors.Is(err, pg.ErrNoRows) {
			return nil, entitlement.ErrNoSubscription
		}
		return nil, err
	}
	sub := entitlement.Subscription{TenantID: row.TenantID, Plan: entitlement.PlanID(row.Plan)}
	if err := json.Unmarshal([]byte(row.AddOns), &sub.AddOns); err != nil {
		return nil, fmt.Errorf("dropsstore: tenant %q addons: %w", tenantID, err)
	}
	if err := json.Unmarshal([]byte(row.Trial), &sub.Trial); err != nil {
		return nil, fmt.Errorf("dropsstore: tenant %q trial: %w", tenantID, err)
	}
	if err := json.Unmarshal([]byte(row.Grants), &sub.Grants); err != nil {
		return nil, fmt.Errorf("dropsstore: tenant %q grants: %w", tenantID, err)
	}
	if row.BillingAnchor != nil {
		sub.BillingAnchor = row.BillingAnchor.UTC()
	}
	return &sub, nil
}

// Set writes sub as the tenant's whole subscription in one statement:
//
//	INSERT INTO <feature_subscriptions> (tenant_id, plan, addons, trial, grants, billing_anchor, updated_at)
//	VALUES ($1, ...)
//	ON CONFLICT (tenant_id) DO UPDATE SET plan = ..., addons = ..., trial = ..., grants = ..., billing_anchor = ..., updated_at = ...
//
// Every non-key column is assigned in the DO UPDATE, including the
// empty lists and the null trial, which is what makes this the
// wholesale replacement the Seeder port requires rather than a merge:
// nothing from a previous write survives. Being one statement, two
// concurrent Sets for the same new tenant resolve into one row without
// either seeing a unique violation. An empty TenantID is refused with
// entitlement.ErrEmptyTenantID before anything is sent.
func (st *SubscriptionStore) Set(ctx context.Context, sub entitlement.Subscription) error {
	if sub.TenantID == "" {
		return entitlement.ErrEmptyTenantID
	}
	addons, trial, grants, err := encodeLists(sub)
	if err != nil {
		return err
	}
	c := st.s.sub
	anchor := c.billingAnchor.Expr(drops.Raw("NULL"))
	if !sub.BillingAnchor.IsZero() {
		anchor = c.billingAnchor.Val(sub.BillingAnchor.UTC())
	}
	now := st.now().UTC()
	_, err = st.db.Insert(st.s.Subscriptions).
		Row(
			c.tenantID.Val(sub.TenantID),
			c.plan.Val(string(sub.Plan)),
			c.addons.Val(addons),
			c.trial.Val(trial),
			c.grants.Val(grants),
			anchor,
			c.updatedAt.Val(now),
		).
		OnConflictUpdate(c.tenantID).
		Set(
			c.plan.Val(string(sub.Plan)),
			c.addons.Val(addons),
			c.trial.Val(trial),
			c.grants.Val(grants),
			anchor,
			c.updatedAt.Val(now),
		).
		Done().
		Exec(ctx)
	return err
}

// Delete removes the tenant's row. An unknown tenant is a no-op, not
// an error: the port's Delete is idempotent.
func (st *SubscriptionStore) Delete(ctx context.Context, tenantID string) error {
	_, err := st.db.Delete(st.s.Subscriptions).
		Where(st.s.sub.tenantID.Eq(tenantID)).
		Exec(ctx)
	return err
}

// encodeLists renders the three jsonb columns. Nil lists become [] so
// the columns stay NOT NULL; a nil trial becomes the JSON null.
func encodeLists(sub entitlement.Subscription) (addons, trial, grants string, err error) {
	if sub.AddOns == nil {
		sub.AddOns = []entitlement.AddOnID{}
	}
	if sub.Grants == nil {
		sub.Grants = []entitlement.Grant{}
	}
	a, err := json.Marshal(sub.AddOns)
	if err != nil {
		return "", "", "", fmt.Errorf("dropsstore: encode addons: %w", err)
	}
	t, err := json.Marshal(sub.Trial)
	if err != nil {
		return "", "", "", fmt.Errorf("dropsstore: encode trial: %w", err)
	}
	g, err := json.Marshal(sub.Grants)
	if err != nil {
		return "", "", "", fmt.Errorf("dropsstore: encode grants: %w", err)
	}
	return string(a), string(t), string(g), nil
}
