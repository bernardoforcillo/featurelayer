package flags

import "sync"

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

// Stubs replaced in Tasks 3 and 4.
func (ev *Evaluator) matchRegexp(pattern, s string) bool { return false }
func matchSemver(op Op, v, ref string) bool              { return false }

// InSegment reports whether attrs belong to the named segment.
func (ev *Evaluator) InSegment(key string, attrs map[string]any) bool { return false }
