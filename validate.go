package featurelayer

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/bernardoforcillo/featurelayer/catalog"
	"github.com/bernardoforcillo/featurelayer/entitlement"
	"github.com/bernardoforcillo/featurelayer/flags"
)

// ValidationError is one problem found by NewSnapshot.
type ValidationError struct {
	Path string
	Msg  string
}

func (e *ValidationError) Error() string { return e.Path + ": " + e.Msg }

type validator struct{ errs []error }

func (v *validator) add(path, format string, args ...any) {
	v.errs = append(v.errs, &ValidationError{Path: path, Msg: fmt.Sprintf(format, args...)})
}

func validate(cfg Config) error {
	v := &validator{}
	featSet := map[catalog.Key]bool{}
	for i, f := range cfg.Features {
		p := fmt.Sprintf("features[%d]", i)
		if err := catalog.ValidateKey(f.Key); err != nil {
			v.add(p+".key", "invalid key %q", f.Key)
		} else if featSet[f.Key] {
			v.add(p+".key", "duplicate feature %q", f.Key)
		} else {
			featSet[f.Key] = true
		}
		if !f.Lifecycle.Valid() {
			v.add(p+".lifecycle", "invalid lifecycle %q", f.Lifecycle)
		}
	}
	// DependsOn references and self-references (existence first, cycles after)
	for i, f := range cfg.Features {
		for j, dep := range f.DependsOn {
			p := fmt.Sprintf("features[%d].dependsOn[%d]", i, j)
			if dep == f.Key {
				v.add(p, "feature depends on itself")
			} else if !featSet[dep] {
				v.add(p, "unknown feature %q", dep)
			}
		}
	}
	v.checkDependencyCycles(cfg.Features)

	segSet := map[string]bool{}
	for i, s := range cfg.Segments {
		p := fmt.Sprintf("segments[%d]", i)
		if err := catalog.ValidateKey(catalog.Key(s.Key)); err != nil {
			v.add(p+".key", "invalid key %q", s.Key)
		} else if segSet[s.Key] {
			v.add(p+".key", "duplicate segment %q", s.Key)
		} else {
			segSet[s.Key] = true
		}
		if len(s.Rules) == 0 {
			v.add(p+".rules", "segment needs at least one rule")
		}
		for j, r := range s.Rules {
			rp := fmt.Sprintf("%s.rules[%d]", p, j)
			if len(r.Conditions) == 0 {
				v.add(rp+".conditions", "rule needs at least one condition")
			}
			for k, c := range r.Conditions {
				v.checkCondition(fmt.Sprintf("%s.conditions[%d]", rp, k), c, nil, true)
			}
		}
	}
	// second pass for segment refs needs the full segment set
	segRef := func(key string) bool { return segSet[key] }

	flagSet := map[catalog.Key]bool{}
	for i, fl := range cfg.Flags {
		p := fmt.Sprintf("flags[%d]", i)
		if !featSet[fl.Feature] {
			v.add(p+".feature", "unknown feature %q", fl.Feature)
		} else if flagSet[fl.Feature] {
			v.add(p+".feature", "duplicate flag for feature %q", fl.Feature)
		} else {
			flagSet[fl.Feature] = true
		}
		varSet := map[string]bool{}
		for j, vr := range fl.Variants {
			vp := fmt.Sprintf("%s.variants[%d]", p, j)
			if vr.Key == "" {
				v.add(vp+".key", "empty variant key")
			} else if varSet[vr.Key] {
				v.add(vp+".key", "duplicate variant %q", vr.Key)
			} else {
				varSet[vr.Key] = true
			}
		}
		if w := fl.Window; w != nil && !w.From.IsZero() && !w.Until.IsZero() && !w.Until.After(w.From) {
			v.add(p+".window", "Until must be after From")
		}
		for j, r := range fl.Rules {
			rp := fmt.Sprintf("%s.rules[%d]", p, j)
			for k, c := range r.Conditions {
				v.checkCondition(fmt.Sprintf("%s.conditions[%d]", rp, k), c, segRef, false)
			}
			v.checkServe(rp+".serve", r.Serve, varSet)
		}
		v.checkServe(p+".default", fl.Default, varSet)
	}

	planSet := map[entitlement.PlanID]int{}
	for i, pl := range cfg.Plans {
		p := fmt.Sprintf("plans[%d]", i)
		if pl.ID == "" {
			v.add(p+".id", "empty plan id")
		} else if _, dup := planSet[pl.ID]; dup {
			v.add(p+".id", "duplicate plan %q", pl.ID)
		} else {
			planSet[pl.ID] = i
		}
		v.checkEntitlements(p, pl.Entitlements, featSet)
	}
	for i, pl := range cfg.Plans {
		if pl.Extends == "" {
			continue
		}
		p := fmt.Sprintf("plans[%d].extends", i)
		if _, ok := planSet[pl.Extends]; !ok {
			v.add(p, "unknown plan %q", pl.Extends)
			continue
		}
		// cycle walk
		seen := map[entitlement.PlanID]bool{pl.ID: true}
		for cur := pl.Extends; cur != ""; {
			if seen[cur] {
				v.add(p, "cyclic Extends through %q", cur)
				break
			}
			seen[cur] = true
			idx, ok := planSet[cur]
			if !ok {
				break
			}
			cur = cfg.Plans[idx].Extends
		}
	}
	addonSet := map[entitlement.AddOnID]bool{}
	for i, a := range cfg.AddOns {
		p := fmt.Sprintf("addons[%d]", i)
		if a.ID == "" {
			v.add(p+".id", "empty addon id")
		} else if addonSet[a.ID] {
			v.add(p+".id", "duplicate addon %q", a.ID)
		} else {
			addonSet[a.ID] = true
		}
		for j, req := range a.Requires {
			if _, ok := planSet[req]; !ok {
				v.add(fmt.Sprintf("%s.requires[%d]", p, j), "unknown plan %q", req)
			}
		}
		v.checkEntitlements(p, a.Entitlements, featSet)
	}
	v.checkPeriodAgreement(cfg, planSet)

	if len(v.errs) == 0 {
		return nil
	}
	return errors.Join(v.errs...)
}

func (v *validator) checkCondition(path string, c flags.Condition, segExists func(string) bool, inSegment bool) {
	if !c.Op.Valid() {
		v.add(path+".op", "unknown op %q", c.Op)
		return
	}
	isSegOp := c.Op == flags.InSegment || c.Op == flags.NotInSegment
	if isSegOp {
		if inSegment {
			v.add(path+".op", "segment ops are not allowed inside segments")
			return
		}
		if c.Attribute != "" {
			v.add(path+".attribute", "must be empty for segment ops")
			return
		}
		key, _ := c.Value.(string)
		if key == "" || (segExists != nil && !segExists(key)) {
			v.add(path+".value", "unknown segment %v", c.Value)
		}
		return
	}
	if c.Attribute == "" {
		v.add(path+".attribute", "attribute required")
	}
	switch c.Op {
	case flags.Exists, flags.NotExists:
		if c.Value != nil || len(c.Values) > 0 {
			v.add(path+".value", "%s takes no value", c.Op)
		}
	case flags.In, flags.NotIn:
		if len(c.Values) == 0 {
			v.add(path+".values", "%s requires values", c.Op)
		}
	case flags.Matches:
		pat, _ := c.Value.(string)
		if _, err := regexp.Compile(pat); pat == "" || err != nil {
			v.add(path+".value", "invalid regexp: %v", c.Value)
		}
	case flags.SemverGt, flags.SemverGte, flags.SemverLt, flags.SemverLte:
		s, _ := c.Value.(string)
		if s == "" || !flags.ValidSemver(s) {
			v.add(path+".value", "invalid semver: %v", c.Value)
		}
	default:
		if c.Value == nil {
			v.add(path+".value", "%s requires a value", c.Op)
		}
	}
}

func (v *validator) checkServe(path string, s flags.Serve, variants map[string]bool) {
	if s.Variant != "" && !variants[s.Variant] {
		v.add(path+".variant", "unknown variant %q", s.Variant)
	}
	if ro := s.Rollout; ro != nil {
		sum := 0.0
		for i, p := range ro.Split {
			pp := fmt.Sprintf("%s.rollout.split[%d]", path, i)
			if p.Percent < 0 {
				v.add(pp+".percent", "negative percent")
			}
			if p.Variant != "" && !variants[p.Variant] {
				v.add(pp+".variant", "unknown variant %q", p.Variant)
			}
			sum += p.Percent
		}
		if sum > 100+1e-9 {
			v.add(path+".rollout.split", "percents sum to %.2f > 100", sum)
		}
	}
}

func (v *validator) checkEntitlements(path string, ents []entitlement.Entitlement, feats map[catalog.Key]bool) {
	seen := map[catalog.Key]bool{}
	for i, e := range ents {
		p := fmt.Sprintf("%s.entitlements[%d]", path, i)
		if !feats[e.Feature] {
			v.add(p+".feature", "unknown feature %q", e.Feature)
		} else if seen[e.Feature] {
			v.add(p+".feature", "duplicate entitlement for %q", e.Feature)
		} else {
			seen[e.Feature] = true
		}
		if l := e.Limit; l != nil {
			if l.Max < 0 {
				v.add(p+".limit.max", "negative max")
			}
			if !l.Period.Valid() {
				v.add(p+".limit.period", "invalid period %q", l.Period)
			}
			if !l.Per.Valid() {
				v.add(p+".limit.per", "invalid scope %q", l.Per)
			}
		}
	}
}

// checkPeriodAgreement: across every flattened plan and every addon
// that set a Limit for the same feature, the Period must be identical,
// and so must the scope (Per; "" and PerTenant are the same scope).
// The resolver sums such limits, and a sum across different periods
// or across a per-tenant and a per-subject counter has no meaning.
func (v *validator) checkPeriodAgreement(cfg Config, planSet map[entitlement.PlanID]int) {
	period := map[catalog.Key]entitlement.Period{}
	scope := map[catalog.Key]entitlement.LimitScope{}
	fix := map[catalog.Key]bool{}
	fixScope := map[catalog.Key]bool{}
	record := func(path string, e entitlement.Entitlement) {
		if e.Limit == nil || !e.Limit.Period.Valid() || !e.Limit.Per.Valid() {
			return
		}
		if sc, ok := scope[e.Feature]; ok && sc != e.Limit.Scope() && !fixScope[e.Feature] {
			v.add(path+".limit.per", "scope %q disagrees with %q used elsewhere for feature %q", e.Limit.Scope(), sc, e.Feature)
			fixScope[e.Feature] = true
		} else {
			scope[e.Feature] = e.Limit.Scope()
		}
		if p, ok := period[e.Feature]; ok && p != e.Limit.Period && !fix[e.Feature] {
			v.add(path+".limit.period", "period %q disagrees with %q used elsewhere for feature %q", e.Limit.Period, p, e.Feature)
			fix[e.Feature] = true
			return
		}
		period[e.Feature] = e.Limit.Period
	}
	// flattened plans: the leaf-most limit per feature, via the resolver.
	// Iterate cfg.Plans in declaration order (not the planSet map, whose
	// range order is randomized) so the blamed path is deterministic.
	if r, err := entitlement.NewResolver(cfg.Plans, cfg.AddOns); err == nil {
		for i, pl := range cfg.Plans {
			if idx, ok := planSet[pl.ID]; !ok || idx != i {
				continue // empty/duplicate id already reported elsewhere
			}
			for j, e := range r.Entitlements(pl.ID) {
				record(fmt.Sprintf("plans[%d].entitlements[%d]", i, j), e)
			}
		}
	}
	for i, a := range cfg.AddOns {
		for j, e := range a.Entitlements {
			record(fmt.Sprintf("addons[%d].entitlements[%d]", i, j), e)
		}
	}
}

// checkDependencyCycles runs a three-color DFS over DependsOn edges,
// reporting one error per disjoint cyclic component (not just the
// first cycle found overall).
func (v *validator) checkDependencyCycles(features []catalog.Feature) {
	idx := map[catalog.Key]int{}
	for i, f := range features {
		idx[f.Key] = i
	}
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make([]int, len(features))
	var stack []int
	var visit func(i int) bool
	visit = func(i int) bool {
		color[i] = gray
		stack = append(stack, i)
		for _, dep := range features[i].DependsOn {
			j, ok := idx[dep]
			if !ok || dep == features[i].Key {
				continue // reported by the reference pass
			}
			if color[j] == gray {
				return false
			}
			if color[j] == white && !visit(j) {
				return false
			}
		}
		color[i] = black
		stack = stack[:len(stack)-1]
		return true
	}
	for i := range features {
		if color[i] != white {
			continue
		}
		stack = stack[:0]
		if !visit(i) {
			v.add(fmt.Sprintf("features[%d].dependsOn", i), "dependency cycle involving %q", features[i].Key)
			// Mark every node left on the recursion stack (the gray
			// nodes of this cyclic component) black so the same cycle
			// is not re-reported through another entry point, then
			// keep scanning for further disjoint components.
			for _, j := range stack {
				color[j] = black
			}
		}
	}
}
