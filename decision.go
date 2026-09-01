package featurelayer

import (
	"time"

	"github.com/bernardoforcillo/featurelayer/catalog"
	"github.com/bernardoforcillo/featurelayer/entitlement"
	"github.com/bernardoforcillo/featurelayer/flags"
)

// EvalContext identifies who is asking.
type EvalContext struct {
	TenantID   string
	UserID     string
	Attributes map[string]any
	Now        time.Time // zero = engine clock
}

// Reason names the pipeline step that decided the outcome.
type Reason string

const (
	ReasonUnknownFeature Reason = "unknown_feature"
	ReasonLifecycle      Reason = "lifecycle"
	ReasonFlagOff        Reason = "flag_off"
	ReasonFlagWindow     Reason = "flag_window"
	ReasonFlagRule       Reason = "flag_rule"
	ReasonFlagRollout    Reason = "flag_rollout"
	ReasonFlagDefault    Reason = "flag_default"
	ReasonNoFlag         Reason = "no_flag"
	ReasonNotEntitled    Reason = "not_entitled"
	ReasonDenied         Reason = "denied"
	ReasonPrerequisite   Reason = "prerequisite"
	ReasonLimitReached   Reason = "limit_reached"
	ReasonStoreError     Reason = "store_error"
)

// Decision is the full answer for one feature and one context.
type Decision struct {
	Feature     catalog.Key
	Enabled     bool
	Variant     *flags.Variant
	Reason      Reason
	Detail      string
	Lifecycle   catalog.Lifecycle
	Entitlement *entitlement.Resolution // nil when the entitlement step was skipped
	Usage       *UsageInfo              // set only by Consume and Usage
	Err         error                   // the error behind a fail-closed decision
}

// UsageInfo reports metering state for limited features.
type UsageInfo struct {
	Used        int64
	Max         int64 // -1 when unlimited
	Remaining   int64 // -1 when unlimited
	Period      string
	PeriodStart time.Time
	ResetsAt    time.Time
}
