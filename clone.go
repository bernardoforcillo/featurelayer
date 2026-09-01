package featurelayer

import (
	"slices"

	"github.com/bernardoforcillo/featurelayer/catalog"
	"github.com/bernardoforcillo/featurelayer/entitlement"
	"github.com/bernardoforcillo/featurelayer/flags"
)

// Clone helpers give Snapshot value semantics at both boundaries:
// NewSnapshot clones cfg on the way in, so mutating cfg afterwards
// never reaches the snapshot; every Snapshot accessor clones on the
// way out, so mutating a returned value never reaches the snapshot
// either. Variant.Value and Condition.Value are the one documented
// exception (spec §6.1): they are held by reference and callers must
// treat them as immutable.

func cloneFeature(f catalog.Feature) catalog.Feature {
	f.Tags = slices.Clone(f.Tags)
	f.DependsOn = slices.Clone(f.DependsOn)
	return f
}

func cloneCondition(c flags.Condition) flags.Condition {
	c.Values = slices.Clone(c.Values)
	return c
}

func cloneConditions(cs []flags.Condition) []flags.Condition {
	if cs == nil {
		return nil
	}
	out := make([]flags.Condition, len(cs))
	for i, c := range cs {
		out[i] = cloneCondition(c)
	}
	return out
}

func cloneWindow(w *flags.Window) *flags.Window {
	if w == nil {
		return nil
	}
	cp := *w
	return &cp
}

func cloneRollout(ro *flags.Rollout) *flags.Rollout {
	if ro == nil {
		return nil
	}
	cp := *ro
	cp.Split = slices.Clone(ro.Split)
	return &cp
}

func cloneServe(s flags.Serve) flags.Serve {
	s.Rollout = cloneRollout(s.Rollout)
	return s
}

func cloneRule(r flags.Rule) flags.Rule {
	r.Conditions = cloneConditions(r.Conditions)
	r.Serve = cloneServe(r.Serve)
	return r
}

func cloneRules(rs []flags.Rule) []flags.Rule {
	if rs == nil {
		return nil
	}
	out := make([]flags.Rule, len(rs))
	for i, r := range rs {
		out[i] = cloneRule(r)
	}
	return out
}

func cloneSegmentRule(r flags.SegmentRule) flags.SegmentRule {
	r.Conditions = cloneConditions(r.Conditions)
	return r
}

func cloneSegmentRules(rs []flags.SegmentRule) []flags.SegmentRule {
	if rs == nil {
		return nil
	}
	out := make([]flags.SegmentRule, len(rs))
	for i, r := range rs {
		out[i] = cloneSegmentRule(r)
	}
	return out
}

func cloneSegment(sg flags.Segment) flags.Segment {
	sg.Rules = cloneSegmentRules(sg.Rules)
	return sg
}

func cloneFlag(fl flags.Flag) flags.Flag {
	fl.Window = cloneWindow(fl.Window)
	fl.Variants = slices.Clone(fl.Variants) // Variant.Value stays by-reference
	fl.Rules = cloneRules(fl.Rules)
	fl.Default = cloneServe(fl.Default)
	return fl
}

func cloneLimit(l *entitlement.Limit) *entitlement.Limit {
	if l == nil {
		return nil
	}
	cp := *l
	return &cp
}

func cloneEntitlement(e entitlement.Entitlement) entitlement.Entitlement {
	e.Limit = cloneLimit(e.Limit)
	return e
}

func cloneEntitlements(es []entitlement.Entitlement) []entitlement.Entitlement {
	if es == nil {
		return nil
	}
	out := make([]entitlement.Entitlement, len(es))
	for i, e := range es {
		out[i] = cloneEntitlement(e)
	}
	return out
}

func clonePlan(p entitlement.Plan) entitlement.Plan {
	p.Entitlements = cloneEntitlements(p.Entitlements)
	return p
}

func cloneAddOn(a entitlement.AddOn) entitlement.AddOn {
	a.Requires = slices.Clone(a.Requires)
	a.Entitlements = cloneEntitlements(a.Entitlements)
	return a
}

// cloneConfig deep-copies cfg so NewSnapshot can index and build its
// evaluator/resolver from data the caller can no longer reach.
func cloneConfig(cfg Config) Config {
	out := Config{
		Features: make([]catalog.Feature, len(cfg.Features)),
		Segments: make([]flags.Segment, len(cfg.Segments)),
		Flags:    make([]flags.Flag, len(cfg.Flags)),
		Plans:    make([]entitlement.Plan, len(cfg.Plans)),
		AddOns:   make([]entitlement.AddOn, len(cfg.AddOns)),
	}
	for i, f := range cfg.Features {
		out.Features[i] = cloneFeature(f)
	}
	for i, sg := range cfg.Segments {
		out.Segments[i] = cloneSegment(sg)
	}
	for i, fl := range cfg.Flags {
		out.Flags[i] = cloneFlag(fl)
	}
	for i, p := range cfg.Plans {
		out.Plans[i] = clonePlan(p)
	}
	for i, a := range cfg.AddOns {
		out.AddOns[i] = cloneAddOn(a)
	}
	return out
}
