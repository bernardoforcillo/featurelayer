package entitlement

import (
	"time"

	"github.com/bernardoforcillo/featurelayer/catalog"
)

// Subscription is a tenant's commercial state: base plan, add-ons, an
// optional plan trial, per-feature grants and the billing anchor that
// aligns metering periods.
type Subscription struct {
	TenantID      string     `json:"tenantId"`
	Plan          PlanID     `json:"plan,omitempty"`
	AddOns        []AddOnID  `json:"addons,omitempty"`
	Trial         *PlanTrial `json:"trial,omitempty"`
	Grants        []Grant    `json:"grants,omitempty"`
	BillingAnchor time.Time  `json:"billingAnchor,omitempty"`
}

// PlanTrial makes Plan the effective plan until Until.
type PlanTrial struct {
	Plan  PlanID    `json:"plan"`
	Until time.Time `json:"until"`
}

// Grant overrides plans for one feature: an allow (with an optional
// limit) or a Deny, until Until (zero = forever).
type Grant struct {
	Feature catalog.Key `json:"feature"`
	Deny    bool        `json:"deny,omitempty"`
	Limit   *Limit      `json:"limit,omitempty"`
	Until   time.Time   `json:"until,omitempty"`
	Reason  string      `json:"reason,omitempty"`
}

// Override grants a feature permanently, regardless of plan.
func Override(feature catalog.Key, limit *Limit, reason string) Grant {
	return Grant{Feature: feature, Limit: limit, Reason: reason}
}

// Trial grants a feature until the given instant.
func Trial(feature catalog.Key, until time.Time, reason string) Grant {
	return Grant{Feature: feature, Until: until, Reason: reason}
}

// clone deep-copies s: fresh slices, a fresh PlanTrial and fresh Limit
// allocations, so neither side of a store boundary can reach the other
// through a pointer.
func (s Subscription) clone() Subscription {
	if s.AddOns != nil {
		s.AddOns = append([]AddOnID(nil), s.AddOns...)
	}
	if s.Trial != nil {
		t := *s.Trial
		s.Trial = &t
	}
	if s.Grants != nil {
		grants := make([]Grant, len(s.Grants))
		for i, g := range s.Grants {
			if g.Limit != nil {
				l := *g.Limit
				g.Limit = &l
			}
			grants[i] = g
		}
		s.Grants = grants
	}
	return s
}

// active reports whether the grant applies at now.
func (g Grant) active(now time.Time) bool {
	return g.Until.IsZero() || now.Before(g.Until)
}
