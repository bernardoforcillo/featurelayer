package dropsstore

import (
	"time"

	"github.com/bernardoforcillo/drops/pg"
)

// Names are the two table names the stores persist to. The zero value
// means the defaults.
type Names struct {
	Subscriptions string // default "feature_subscriptions"
	Usage         string // default "feature_usage"
}

func (n Names) withDefaults() Names {
	if n.Subscriptions == "" {
		n.Subscriptions = "feature_subscriptions"
	}
	if n.Usage == "" {
		n.Usage = "feature_usage"
	}
	return n
}

type settings struct {
	names Names
	now   func() time.Time
}

// Option customizes a Schema or a store at construction.
type Option func(*settings)

// WithNames overrides the table names, for a deployment that already
// owns a table by either default name or that runs two featurelayer
// engines against one database.
//
//	dropsstore.NewUsageStore(db, dropsstore.WithNames(dropsstore.Names{Usage: "billing_usage"}))
func WithNames(n Names) Option { return func(s *settings) { s.names = n } }

// WithClock overrides the time source used to stamp updated_at on
// subscription rows (tests, replay). Default time.Now.
func WithClock(now func() time.Time) Option { return func(s *settings) { s.now = now } }

// Schema holds the two tables and their columns:
//
//	<feature_subscriptions>  tenant_id text PK, plan text, addons jsonb, trial jsonb,
//	                         grants jsonb, billing_anchor timestamptz NULL,
//	                         updated_at timestamptz
//	<feature_usage>          tenant text, feature text, period text, subject text,
//	                         total bigint, PRIMARY KEY (tenant, feature, period, subject)
//
// # Why the subscription's lists are JSONB
//
// entitlement.Subscription is small and application-shaped: a plan id,
// a handful of add-on ids, at most one trial and a few grants. The
// engine reads it whole on every gated evaluation and the application
// writes it whole when billing state changes; nothing ever queries "all
// tenants holding add-on X" through this store, and the port offers no
// method that would. Normalising addons, grants and trial into child
// tables would buy three joins per read and a multi-statement write for
// no query the library makes. The scalar fields — tenant_id, plan,
// billing_anchor — are proper columns because they ARE what an operator
// filters and inspects by. If the application needs to report over
// grants, PostgreSQL's jsonb operators work on the column as it stands.
//
// The three JSONB columns hold exactly the encoding/json form of the
// corresponding Subscription fields (the same shape the config file
// uses), so a row is readable and hand-editable with psql. trial holds
// the JSON literal null when there is no trial, and the lists hold []
// rather than null, so every column is NOT NULL and there is no
// NULL-versus-empty distinction to get wrong.
//
// # The usage primary key is the whole UsageKey
//
// (tenant, feature, period, subject) is exactly entitlement.UsageKey,
// with the empty string standing for "no period" and "no subject": a
// primary key cannot hold NULL, and two NULLs would not conflict with
// each other, so the tenant-scoped counter is the row whose subject is
// "", not NULL. That key is what makes UsageStore.Increment a single
// INSERT ... ON CONFLICT statement, and therefore atomic; see
// UsageStore.Increment. It is a composite key, which drops' CREATE TABLE
// does not carry, so Store.CreateSchema emits it itself.
type Schema struct {
	Subscriptions *pg.Table
	Usage         *pg.Table

	sub   subscriptionColumns
	usage usageColumns
}

type subscriptionColumns struct {
	tenantID      *pg.Col[string]
	plan          *pg.Col[string]
	addons        *pg.Col[string]
	trial         *pg.Col[string]
	grants        *pg.Col[string]
	billingAnchor *pg.Col[time.Time]
	updatedAt     *pg.Col[time.Time]
}

type usageColumns struct {
	tenant  *pg.Col[string]
	feature *pg.Col[string]
	period  *pg.Col[string]
	subject *pg.Col[string]
	total   *pg.Col[int64]
}

// NewSchema builds the table definitions. The store constructors call
// it; use it directly only to generate DDL for your own migrations.
func NewSchema(opts ...Option) *Schema {
	cfg := newSettings(opts)
	names := cfg.names
	s := &Schema{
		Subscriptions: pg.NewTable(names.Subscriptions),
		Usage:         pg.NewTable(names.Usage),
	}
	// jsonb columns are typed as Go strings: the value is the JSON text
	// itself, which the pgx driver sends and receives as text for the
	// jsonb type, and which is unambiguous in both directions.
	s.sub = subscriptionColumns{
		tenantID:      pg.Add(s.Subscriptions, pg.Text("tenant_id").PrimaryKey()),
		plan:          pg.Add(s.Subscriptions, pg.Text("plan").NotNull()),
		addons:        pg.Add(s.Subscriptions, pg.Custom[string]("addons", "jsonb").NotNull()),
		trial:         pg.Add(s.Subscriptions, pg.Custom[string]("trial", "jsonb").NotNull()),
		grants:        pg.Add(s.Subscriptions, pg.Custom[string]("grants", "jsonb").NotNull()),
		billingAnchor: pg.Add(s.Subscriptions, pg.Timestamp("billing_anchor", true)),
		updatedAt:     pg.Add(s.Subscriptions, pg.Timestamp("updated_at", true).NotNull()),
	}
	s.usage = usageColumns{
		tenant:  pg.Add(s.Usage, pg.Text("tenant").NotNull()),
		feature: pg.Add(s.Usage, pg.Text("feature").NotNull()),
		period:  pg.Add(s.Usage, pg.Text("period").NotNull()),
		subject: pg.Add(s.Usage, pg.Text("subject").NotNull()),
		total:   pg.Add(s.Usage, pg.BigInt("total").NotNull()),
	}
	s.Usage.PrimaryKey(s.usage.tenant, s.usage.feature, s.usage.period, s.usage.subject)
	return s
}

func newSettings(opts []Option) settings {
	cfg := settings{now: time.Now}
	for _, o := range opts {
		o(&cfg)
	}
	cfg.names = cfg.names.withDefaults()
	return cfg
}
