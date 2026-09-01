package flags

import (
	"regexp"
	"sync"
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
