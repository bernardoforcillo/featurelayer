package featurelayer

import (
	"cmp"
	"slices"

	"github.com/bernardoforcillo/featurelayer/catalog"
	"github.com/bernardoforcillo/featurelayer/entitlement"
	"github.com/bernardoforcillo/featurelayer/flags"
)

// Snapshot is an immutable, validated set of definitions. Safe to
// share; replace it atomically with Engine.Apply.
type Snapshot struct {
	features   map[catalog.Key]catalog.Feature
	segments   map[string]flags.Segment
	flagsByKey map[catalog.Key]flags.Flag
	plans      map[entitlement.PlanID]entitlement.Plan
	addons     map[entitlement.AddOnID]entitlement.AddOn
	evaluator  *flags.Evaluator
	resolver   *entitlement.Resolver
}

// NewSnapshot validates cfg (returning every problem at once as an
// errors.Join of *ValidationError) and builds the indexed snapshot.
// cfg is deep-copied before indexing: mutating cfg afterwards, at any
// nesting depth, never affects the snapshot. The one exception is
// Variant.Value and Condition.Value, held by reference per spec §6.1.
func NewSnapshot(cfg Config) (*Snapshot, error) {
	if err := validate(cfg); err != nil {
		return nil, err
	}
	cfg = cloneConfig(cfg)
	s := &Snapshot{
		features:   make(map[catalog.Key]catalog.Feature, len(cfg.Features)),
		segments:   make(map[string]flags.Segment, len(cfg.Segments)),
		flagsByKey: make(map[catalog.Key]flags.Flag, len(cfg.Flags)),
		plans:      make(map[entitlement.PlanID]entitlement.Plan, len(cfg.Plans)),
		addons:     make(map[entitlement.AddOnID]entitlement.AddOn, len(cfg.AddOns)),
	}
	for _, f := range cfg.Features {
		s.features[f.Key] = f
	}
	for _, sg := range cfg.Segments {
		s.segments[sg.Key] = sg
	}
	for _, fl := range cfg.Flags {
		s.flagsByKey[fl.Feature] = fl
	}
	for _, p := range cfg.Plans {
		s.plans[p.ID] = p
	}
	for _, a := range cfg.AddOns {
		s.addons[a.ID] = a
	}
	// Built from the cloned data so the evaluator's and resolver's own
	// internals are isolated from cfg too.
	s.evaluator = flags.NewEvaluator(cfg.Segments...)
	r, err := entitlement.NewResolver(cfg.Plans, cfg.AddOns)
	if err != nil {
		// unreachable: validate() already rejected these configs
		return nil, err
	}
	s.resolver = r
	return s, nil
}

// Feature returns a deep copy: mutating it can never reach the
// snapshot or any other caller's copy.
func (s *Snapshot) Feature(key catalog.Key) (catalog.Feature, bool) {
	f, ok := s.features[key]
	return cloneFeature(f), ok
}

// Segment returns a deep copy; see Feature.
func (s *Snapshot) Segment(key string) (flags.Segment, bool) {
	sg, ok := s.segments[key]
	return cloneSegment(sg), ok
}

// Flag returns a deep copy; see Feature.
func (s *Snapshot) Flag(key catalog.Key) (flags.Flag, bool) {
	f, ok := s.flagsByKey[key]
	return cloneFlag(f), ok
}

// Plan returns a deep copy (as configured, not flattened); see Feature.
func (s *Snapshot) Plan(id entitlement.PlanID) (entitlement.Plan, bool) {
	p, ok := s.plans[id]
	return clonePlan(p), ok
}

// AddOn returns a deep copy; see Feature.
func (s *Snapshot) AddOn(id entitlement.AddOnID) (entitlement.AddOn, bool) {
	a, ok := s.addons[id]
	return cloneAddOn(a), ok
}

// PlanEntitlements returns a plan's entitlements flattened through
// Extends (descendants override ancestors per feature), deep-copied;
// see Feature.
func (s *Snapshot) PlanEntitlements(id entitlement.PlanID) []entitlement.Entitlement {
	return s.resolver.Entitlements(id)
}

// Features returns deep copies, sorted by key; see Feature.
func (s *Snapshot) Features() []catalog.Feature {
	out := make([]catalog.Feature, 0, len(s.features))
	for _, f := range s.features {
		out = append(out, cloneFeature(f))
	}
	slices.SortFunc(out, func(a, b catalog.Feature) int { return cmp.Compare(a.Key, b.Key) })
	return out
}

// Segments returns deep copies, sorted by key; see Feature.
func (s *Snapshot) Segments() []flags.Segment {
	out := make([]flags.Segment, 0, len(s.segments))
	for _, sg := range s.segments {
		out = append(out, cloneSegment(sg))
	}
	slices.SortFunc(out, func(a, b flags.Segment) int { return cmp.Compare(a.Key, b.Key) })
	return out
}

// Plans returns deep copies (as configured, not flattened), sorted by
// id; see Feature.
func (s *Snapshot) Plans() []entitlement.Plan {
	out := make([]entitlement.Plan, 0, len(s.plans))
	for _, p := range s.plans {
		out = append(out, clonePlan(p))
	}
	slices.SortFunc(out, func(a, b entitlement.Plan) int { return cmp.Compare(a.ID, b.ID) })
	return out
}

// AddOns returns deep copies, sorted by id; see Feature.
func (s *Snapshot) AddOns() []entitlement.AddOn {
	out := make([]entitlement.AddOn, 0, len(s.addons))
	for _, a := range s.addons {
		out = append(out, cloneAddOn(a))
	}
	slices.SortFunc(out, func(a, b entitlement.AddOn) int { return cmp.Compare(a.ID, b.ID) })
	return out
}
