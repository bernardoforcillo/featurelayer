// Package entitlement models commercial access: plans (with
// inheritance), add-ons, per-tenant grants and trials, limits and
// billing periods, and the two stores that hold per-tenant state.
package entitlement

import "github.com/bernardoforcillo/featurelayer/catalog"

type PlanID string
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

type Limit struct {
	Max    int64  `json:"max"`
	Period Period `json:"period,omitempty"`
}

type Entitlement struct {
	Feature catalog.Key `json:"feature"`
	Limit   *Limit      `json:"limit,omitempty"` // nil = unlimited
}

type Plan struct {
	ID           PlanID        `json:"id"`
	Name         string        `json:"name,omitempty"`
	Extends      PlanID        `json:"extends,omitempty"`
	Entitlements []Entitlement `json:"entitlements,omitempty"`
}

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
