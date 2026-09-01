// Package flags models operational feature flags: kill switch, time
// windows, attribute targeting, segments, percentage rollout, variants.
package flags

import (
	"time"

	"github.com/bernardoforcillo/featurelayer/catalog"
)

type Flag struct {
	Feature  catalog.Key `json:"feature"`
	Enabled  bool        `json:"enabled"`
	Window   *Window     `json:"window,omitempty"`
	Variants []Variant   `json:"variants,omitempty"`
	Rules    []Rule      `json:"rules,omitempty"`
	Default  Serve       `json:"default"`
}

type Window struct {
	From  time.Time `json:"from,omitempty"`
	Until time.Time `json:"until,omitempty"`
}

type Variant struct {
	Key   string `json:"key"`
	Value any    `json:"value,omitempty"`
}

type Rule struct {
	Name       string      `json:"name,omitempty"`
	Conditions []Condition `json:"conditions,omitempty"`
	Serve      Serve       `json:"serve"`
}

type Serve struct {
	On      bool     `json:"on"`
	Variant string   `json:"variant,omitempty"`
	Rollout *Rollout `json:"rollout,omitempty"`
}

type Rollout struct {
	BucketBy string    `json:"bucketBy,omitempty"`
	Seed     string    `json:"seed,omitempty"`
	Split    []Portion `json:"split"`
}

type Portion struct {
	Variant string  `json:"variant,omitempty"`
	Percent float64 `json:"percent"`
}

// Percent is shorthand for a boolean p% rollout.
func Percent(p float64) *Rollout {
	return &Rollout{Split: []Portion{{Percent: p}}}
}

type Segment struct {
	Key   string        `json:"key"`
	Name  string        `json:"name,omitempty"`
	Rules []SegmentRule `json:"rules"`
}

type SegmentRule struct {
	Name       string      `json:"name,omitempty"`
	Conditions []Condition `json:"conditions"`
}

// Reason explains a flag outcome.
type Reason string

const (
	ReasonOff     Reason = "off"
	ReasonWindow  Reason = "window"
	ReasonRule    Reason = "rule"
	ReasonRollout Reason = "rollout"
	ReasonDefault Reason = "default"
)

// Outcome is the result of evaluating a flag.
type Outcome struct {
	On      bool
	Variant *Variant
	Reason  Reason
	Detail  string
}
