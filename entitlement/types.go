// Package entitlement models commercial access: plans (with
// inheritance), add-ons, per-tenant grants and trials, limits and
// billing periods, and the two stores that hold per-tenant state.
package entitlement

import "github.com/bernardoforcillo/featurelayer/catalog"

// PlanID identifies a Plan.
type PlanID string

// AddOnID identifies an AddOn.
type AddOnID string

// Period is the reset window of a metered limit.
type Period string

const (
	None  Period = ""
	Day   Period = "day"
	Week  Period = "week"
	Month Period = "month"
	Year  Period = "year"
)

// Valid reports whether p is a defined period.
func (p Period) Valid() bool {
	switch p {
	case None, Day, Week, Month, Year:
		return true
	}
	return false
}

// LimitScope says which counter a metered limit meters: one per
// tenant (the default) or one per subject (user) within the tenant.
type LimitScope string

const (
	// PerTenant meters one counter per tenant. It is the default: the
	// zero value "" means PerTenant, so existing configurations are
	// unchanged.
	PerTenant LimitScope = "tenant"
	// PerSubject meters one counter per (tenant, subject). The subject
	// is EvalContext.UserID; metering a PerSubject limit without one is
	// a fail-closed decision, never a shared counter.
	PerSubject LimitScope = "subject"
)

// Valid reports whether s is a defined scope. "" is valid and means
// PerTenant.
func (s LimitScope) Valid() bool {
	switch s {
	case "", PerTenant, PerSubject:
		return true
	}
	return false
}

// Limit is a metered cap: at most Max units per Period, counted per
// tenant or per subject.
type Limit struct {
	Max    int64      `json:"max"`
	Period Period     `json:"period,omitempty"`
	Per    LimitScope `json:"per,omitempty"` // "" = PerTenant
}

// Scope returns the effective scope: PerTenant when Per is empty,
// else Per unchanged (an invalid value is returned as is, so callers
// can still detect it with Valid).
func (l Limit) Scope() LimitScope {
	if l.Per == "" {
		return PerTenant
	}
	return l.Per
}

// Entitlement grants one feature, optionally under a metered limit.
type Entitlement struct {
	Feature catalog.Key `json:"feature"`
	Limit   *Limit      `json:"limit,omitempty"` // nil = unlimited
}

// Plan is a sellable bundle of entitlements. Extends flattens the
// parent's entitlements under this plan's, per feature.
type Plan struct {
	ID           PlanID        `json:"id"`
	Name         string        `json:"name,omitempty"`
	Extends      PlanID        `json:"extends,omitempty"`
	Entitlements []Entitlement `json:"entitlements,omitempty"`
}

// AddOn is a bundle of entitlements a tenant can hold on top of a
// plan; Requires restricts it to (descendants of) the listed plans.
type AddOn struct {
	ID           AddOnID       `json:"id"`
	Name         string        `json:"name,omitempty"`
	Requires     []PlanID      `json:"requires,omitempty"`
	Entitlements []Entitlement `json:"entitlements,omitempty"`
}

// Limited is shorthand for an entitlement with a metered limit.
func Limited(feature catalog.Key, max int64, period Period) Entitlement {
	return Entitlement{Feature: feature, Limit: &Limit{Max: max, Period: period}}
}

// LimitedPer is Limited with an explicit scope: PerTenant meters the
// whole tenant, PerSubject meters each user of the tenant separately.
func LimitedPer(feature catalog.Key, max int64, period Period, scope LimitScope) Entitlement {
	return Entitlement{Feature: feature, Limit: &Limit{Max: max, Period: period, Per: scope}}
}
