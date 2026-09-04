package featurelayer

import (
	"context"
	"errors"

	"github.com/bernardoforcillo/featurelayer/catalog"
)

// ErrNoEvalContext is returned by ConsumeCtx and UsageCtx when ctx
// carries no EvalContext (see NewContext).
var ErrNoEvalContext = errors.New("featurelayer: no EvalContext on ctx")

// ReasonNoContext is the fail-closed reason of every *Ctx call made on
// a context that carries no EvalContext.
const ReasonNoContext Reason = "no_context"

type evalContextKey struct{}

// NewContext returns ctx carrying ec, for the *Ctx variants of the
// Engine methods. Set it once where the request's identity is
// established (after authentication, in the same place the tenant and
// user are known) and every feature check downstream reads it back.
//
// The library reads nothing else from ctx: how the tenant and user get
// there is the application's business. store/drops offers
// EvalContextFrom, which builds an EvalContext from the drops
// tenant/subject the authlayer stack already puts on ctx.
func NewContext(ctx context.Context, ec EvalContext) context.Context {
	return context.WithValue(ctx, evalContextKey{}, ec)
}

// FromContext returns the EvalContext stored by NewContext, and false
// when there is none.
func FromContext(ctx context.Context) (EvalContext, bool) {
	ec, ok := ctx.Value(evalContextKey{}).(EvalContext)
	return ec, ok
}

// noContext is the decision every *Ctx call returns when ctx carries no
// EvalContext. It is a fail-closed decision, not a panic and not an
// enabled feature: a missing context is a wiring bug, and the safest
// thing to do with a wiring bug is to deny and make it visible through
// Reason and Err.
func noContext(key catalog.Key) Decision {
	return Decision{Feature: key, Reason: ReasonNoContext, Err: ErrNoEvalContext}
}

// EvaluateCtx is Evaluate with the EvalContext read from ctx. Without
// one it returns an off decision with Reason no_context and Err
// ErrNoEvalContext; decision hooks still fire, so the miss is
// observable.
func (e *Engine) EvaluateCtx(ctx context.Context, key catalog.Key) Decision {
	ec, ok := FromContext(ctx)
	if !ok {
		d := noContext(key)
		e.fireDecision(OpEvaluate, EvalContext{}, d, 0, nil)
		return d
	}
	return e.Evaluate(ctx, key, ec)
}

// IsEnabledCtx is IsEnabled with the EvalContext read from ctx; false
// when there is none.
func (e *Engine) IsEnabledCtx(ctx context.Context, key catalog.Key) bool {
	return e.EvaluateCtx(ctx, key).Enabled
}

// ConsumeCtx is Consume with the EvalContext read from ctx. Without one
// it consumes nothing and returns the no_context decision together
// with ErrNoEvalContext — an error, because unlike a business "no" this
// is a caller mistake.
func (e *Engine) ConsumeCtx(ctx context.Context, key catalog.Key, n int64) (Decision, error) {
	ec, ok := FromContext(ctx)
	if !ok {
		d := noContext(key)
		e.fireDecision(OpConsume, EvalContext{}, d, 0, ErrNoEvalContext)
		return d, ErrNoEvalContext
	}
	return e.Consume(ctx, key, ec, n)
}

// UsageCtx is Usage with the EvalContext read from ctx; see ConsumeCtx
// for the missing-context behaviour.
func (e *Engine) UsageCtx(ctx context.Context, key catalog.Key) (Decision, error) {
	ec, ok := FromContext(ctx)
	if !ok {
		d := noContext(key)
		e.fireDecision(OpUsage, EvalContext{}, d, 0, ErrNoEvalContext)
		return d, ErrNoEvalContext
	}
	return e.Usage(ctx, key, ec)
}
