package featurelayer

import (
	"sync/atomic"
	"time"

	"github.com/bernardoforcillo/featurelayer/entitlement"
)

// Engine evaluates features against the current snapshot and the
// configured stores. Safe for concurrent use.
type Engine struct {
	snap          atomic.Pointer[Snapshot]
	subs          entitlement.SubscriptionStore
	usage         entitlement.UsageStore
	clock         func() time.Time
	decisionHooks []func(DecisionEvent)
	applyHooks    []func(ApplyEvent)
}

// Option configures an Engine.
type Option func(*Engine)

// WithSubscriptions enables the entitlement step. Without it the
// engine runs in flags-only mode: gated features skip the commercial
// check entirely.
func WithSubscriptions(s entitlement.SubscriptionStore) Option {
	return func(e *Engine) { e.subs = s }
}

// WithUsage enables Consume and Usage.
func WithUsage(u entitlement.UsageStore) Option {
	return func(e *Engine) { e.usage = u }
}

// WithClock overrides the time source (tests, replay).
func WithClock(now func() time.Time) Option {
	return func(e *Engine) { e.clock = now }
}

// WithDecisionHook registers fn to run synchronously after every
// top-level Evaluate/IsEnabled/Variant/Consume/Usage call, in
// registration order. Hooks must be fast, must not block, and must be
// safe for concurrent use. Panics are not recovered.
func WithDecisionHook(fn func(DecisionEvent)) Option {
	return func(e *Engine) { e.decisionHooks = append(e.decisionHooks, fn) }
}

// WithApplyHook registers fn to run synchronously after every Apply.
func WithApplyHook(fn func(ApplyEvent)) Option {
	return func(e *Engine) { e.applyHooks = append(e.applyHooks, fn) }
}

// New builds an Engine on the given snapshot. Panics on nil snapshot.
func New(snap *Snapshot, opts ...Option) *Engine {
	if snap == nil {
		panic("featurelayer: nil snapshot")
	}
	e := &Engine{clock: time.Now}
	e.snap.Store(snap)
	for _, o := range opts {
		o(e)
	}
	return e
}

// Apply atomically replaces the snapshot. In-flight evaluations
// finish on the snapshot they started with.
func (e *Engine) Apply(snap *Snapshot) {
	if snap == nil {
		panic("featurelayer: nil snapshot")
	}
	prev := e.snap.Swap(snap)
	ev := ApplyEvent{Prev: prev, Next: snap, At: e.clock()}
	for _, h := range e.applyHooks {
		h(ev)
	}
}

// Snapshot returns the current snapshot.
func (e *Engine) Snapshot() *Snapshot { return e.snap.Load() }

func (e *Engine) fireDecision(op string, ec EvalContext, d Decision, elapsed time.Duration, err error) {
	if len(e.decisionHooks) == 0 {
		return
	}
	ev := DecisionEvent{Op: op, Context: ec, Decision: d, Elapsed: elapsed, Err: err}
	for _, h := range e.decisionHooks {
		h(ev)
	}
}

// Hook op names.
const (
	OpEvaluate = "evaluate"
	OpConsume  = "consume"
	OpUsage    = "usage"
)

// DecisionEvent is passed to decision hooks after each top-level call.
type DecisionEvent struct {
	Op       string
	Context  EvalContext
	Decision Decision
	Elapsed  time.Duration
	Err      error
}

// ApplyEvent is passed to apply hooks after each snapshot swap.
type ApplyEvent struct {
	Prev, Next *Snapshot
	At         time.Time
}
