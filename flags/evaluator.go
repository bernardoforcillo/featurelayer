package flags

import (
	"hash/fnv"
	"io"
	"regexp"
	"sync"
	"time"
)

// Evaluator evaluates flags against attribute maps. It holds the
// segment definitions and a compiled-regexp cache. The zero value is
// usable for flags that reference no segments.
type Evaluator struct {
	segments map[string]Segment
	res      sync.Map // pattern string -> *regexp.Regexp (nil for invalid)
}

// NewEvaluator returns an Evaluator that knows the given segments.
func NewEvaluator(segments ...Segment) *Evaluator {
	ev := &Evaluator{segments: make(map[string]Segment, len(segments))}
	for _, s := range segments {
		ev.segments[s.Key] = s
	}
	return ev
}

func (ev *Evaluator) matchAll(cs []Condition, attrs map[string]any, inSegment bool) bool {
	for _, c := range cs {
		if !ev.match(c, attrs, inSegment) {
			return false
		}
	}
	return true
}

// matchRegexp compiles pattern once (caching it, nil for invalid) and
// matches it unanchored against s.
func (ev *Evaluator) matchRegexp(pattern, s string) bool {
	if v, ok := ev.res.Load(pattern); ok {
		re, _ := v.(*regexp.Regexp)
		return re != nil && re.MatchString(s)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		re = nil
	}
	ev.res.Store(pattern, re)
	return re != nil && re.MatchString(s)
}

// InSegment reports whether attrs belong to the named segment: member
// when any of the segment's rules has all its conditions matching.
// A rule without conditions matches no one; an unknown segment has no
// members.
func (ev *Evaluator) InSegment(key string, attrs map[string]any) bool {
	seg, ok := ev.segments[key]
	if !ok {
		return false
	}
	for _, r := range seg.Rules {
		if len(r.Conditions) == 0 {
			continue
		}
		if ev.matchAll(r.Conditions, attrs, true) {
			return true
		}
	}
	return false
}

// bucketOf maps (seed, attr) to a stable bucket in [0, 100) with two
// decimals: fnv64a(seed + ":" + attr) % 10000 / 100. This algorithm is
// a public contract; changing it would reshuffle every live rollout.
func bucketOf(seed, attr string) float64 {
	h := fnv.New64a()
	io.WriteString(h, seed)
	io.WriteString(h, ":")
	io.WriteString(h, attr)
	return float64(h.Sum64()%10000) / 100
}

// Serve applies rules in order (first full match wins), then Default.
func (ev *Evaluator) Serve(f *Flag, attrs map[string]any) Outcome {
	for i := range f.Rules {
		r := &f.Rules[i]
		if ev.matchAll(r.Conditions, attrs, false) {
			return ev.serve(f, r.Serve, r.Name, ReasonRule, attrs)
		}
	}
	return ev.serve(f, f.Default, "", ReasonDefault, attrs)
}

// Evaluate is Active followed by Serve.
func (ev *Evaluator) Evaluate(f *Flag, attrs map[string]any, now time.Time) Outcome {
	if ok, out := f.Active(now); !ok {
		return out
	}
	return ev.Serve(f, attrs)
}

func (ev *Evaluator) serve(f *Flag, s Serve, detail string, reason Reason, attrs map[string]any) Outcome {
	if s.Rollout == nil {
		return Outcome{On: s.On, Variant: f.variant(s.Variant), Reason: reason, Detail: detail}
	}
	ro := s.Rollout
	by := ro.BucketBy
	if by == "" {
		by = "tenant"
	}
	val := str(attrs[by])
	if val == "" {
		return Outcome{Reason: ReasonRollout, Detail: "no bucket attribute"}
	}
	seed := ro.Seed
	if seed == "" {
		seed = string(f.Feature)
	}
	b := bucketOf(seed, val)
	cum := 0.0
	for _, p := range ro.Split {
		if b < cum+p.Percent {
			return Outcome{On: true, Variant: f.variant(p.Variant), Reason: ReasonRollout, Detail: detail}
		}
		cum += p.Percent
	}
	return Outcome{Reason: ReasonRollout, Detail: detail}
}
