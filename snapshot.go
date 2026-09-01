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
func NewSnapshot(cfg Config) (*Snapshot, error) {
	if err := validate(cfg); err != nil {
		return nil, err
	}
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
	s.evaluator = flags.NewEvaluator(cfg.Segments...)
	r, err := entitlement.NewResolver(cfg.Plans, cfg.AddOns)
	if err != nil {
		// unreachable: validate() already rejected these configs
		return nil, err
	}
	s.resolver = r
	return s, nil
}

func (s *Snapshot) Feature(key catalog.Key) (catalog.Feature, bool) {
	f, ok := s.features[key]
	return f, ok
}
func (s *Snapshot) Segment(key string) (flags.Segment, bool) {
	sg, ok := s.segments[key]
	return sg, ok
}
func (s *Snapshot) Flag(key catalog.Key) (flags.Flag, bool) { f, ok := s.flagsByKey[key]; return f, ok }
func (s *Snapshot) Plan(id entitlement.PlanID) (entitlement.Plan, bool) {
	p, ok := s.plans[id]
	return p, ok
}
func (s *Snapshot) AddOn(id entitlement.AddOnID) (entitlement.AddOn, bool) {
	a, ok := s.addons[id]
	return a, ok
}

// PlanEntitlements returns a plan's entitlements flattened through
// Extends (descendants override ancestors per feature).
func (s *Snapshot) PlanEntitlements(id entitlement.PlanID) []entitlement.Entitlement {
	return s.resolver.Entitlements(id)
}

func (s *Snapshot) Features() []catalog.Feature {
	out := make([]catalog.Feature, 0, len(s.features))
	for _, f := range s.features {
		out = append(out, f)
	}
	slices.SortFunc(out, func(a, b catalog.Feature) int { return cmp.Compare(a.Key, b.Key) })
	return out
}

func (s *Snapshot) Segments() []flags.Segment {
	out := make([]flags.Segment, 0, len(s.segments))
	for _, sg := range s.segments {
		out = append(out, sg)
	}
	slices.SortFunc(out, func(a, b flags.Segment) int { return cmp.Compare(a.Key, b.Key) })
	return out
}

func (s *Snapshot) Plans() []entitlement.Plan {
	out := make([]entitlement.Plan, 0, len(s.plans))
	for _, p := range s.plans {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b entitlement.Plan) int { return cmp.Compare(a.ID, b.ID) })
	return out
}

func (s *Snapshot) AddOns() []entitlement.AddOn {
	out := make([]entitlement.AddOn, 0, len(s.addons))
	for _, a := range s.addons {
		out = append(out, a)
	}
	slices.SortFunc(out, func(a, b entitlement.AddOn) int { return cmp.Compare(a.ID, b.ID) })
	return out
}
