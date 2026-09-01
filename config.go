// Package featurelayer answers, for a SaaS: can tenant T (user U) use
// feature F right now — and with which variant and limit? It unifies a
// feature catalog, operational flags and commercial entitlements
// behind one fail-closed evaluation engine. Definitions live in an
// immutable, hot-swappable Snapshot; per-tenant state lives behind the
// entitlement.SubscriptionStore and entitlement.UsageStore interfaces.
package featurelayer

import (
	"github.com/bernardoforcillo/featurelayer/catalog"
	"github.com/bernardoforcillo/featurelayer/entitlement"
	"github.com/bernardoforcillo/featurelayer/flags"
)

// Config is the declarative form of a Snapshot. It round-trips
// through encoding/json, so `json.Unmarshal(data, &cfg)` followed by
// NewSnapshot(cfg) is the configuration file format.
type Config struct {
	Features []catalog.Feature   `json:"features"`
	Segments []flags.Segment     `json:"segments,omitempty"`
	Flags    []flags.Flag        `json:"flags,omitempty"`
	Plans    []entitlement.Plan  `json:"plans,omitempty"`
	AddOns   []entitlement.AddOn `json:"addons,omitempty"`
}
