package flags

import (
	"fmt"
	"strings"
)

// Op is a condition operator.
type Op string

const (
	Eq           Op = "eq"
	NotEq        Op = "neq"
	In           Op = "in"
	NotIn        Op = "nin"
	Contains     Op = "contains"
	StartsWith   Op = "startsWith"
	EndsWith     Op = "endsWith"
	Gt           Op = "gt"
	Gte          Op = "gte"
	Lt           Op = "lt"
	Lte          Op = "lte"
	Exists       Op = "exists"
	NotExists    Op = "notExists"
	Matches      Op = "matches"
	SemverGt     Op = "semverGt"
	SemverGte    Op = "semverGte"
	SemverLt     Op = "semverLt"
	SemverLte    Op = "semverLte"
	InSegment    Op = "inSegment"
	NotInSegment Op = "notInSegment"
)

// Valid reports whether o is a defined operator.
func (o Op) Valid() bool {
	switch o {
	case Eq, NotEq, In, NotIn, Contains, StartsWith, EndsWith,
		Gt, Gte, Lt, Lte, Exists, NotExists, Matches,
		SemverGt, SemverGte, SemverLt, SemverLte, InSegment, NotInSegment:
		return true
	}
	return false
}

// Condition is one targeting predicate over the attribute map.
type Condition struct {
	Attribute string `json:"attribute,omitempty"`
	Op        Op     `json:"op"`
	Value     any    `json:"value,omitempty"`
	Values    []any  `json:"values,omitempty"`
}

// match evaluates one condition. inSegment forbids segment ops (so a
// crafted segment cannot recurse into segments).
func (ev *Evaluator) match(c Condition, attrs map[string]any, inSegment bool) bool {
	switch c.Op {
	case InSegment, NotInSegment:
		if inSegment {
			return false
		}
		member := ev.InSegment(str(c.Value), attrs)
		if c.Op == NotInSegment {
			return !member
		}
		return member
	}
	v, ok := attrs[c.Attribute]
	switch c.Op {
	case Exists:
		return ok
	case NotExists:
		return !ok
	case Eq:
		return ok && eqValues(v, c.Value)
	case NotEq:
		return !ok || !eqValues(v, c.Value)
	case In:
		if !ok {
			return false
		}
		for _, w := range c.Values {
			if eqValues(v, w) {
				return true
			}
		}
		return false
	case NotIn:
		if !ok {
			return true
		}
		for _, w := range c.Values {
			if eqValues(v, w) {
				return false
			}
		}
		return true
	case Contains:
		if !ok {
			return false
		}
		switch s := v.(type) {
		case []string:
			for _, e := range s {
				if eqValues(e, c.Value) {
					return true
				}
			}
			return false
		case []any:
			for _, e := range s {
				if eqValues(e, c.Value) {
					return true
				}
			}
			return false
		}
		return strings.Contains(str(v), str(c.Value))
	case StartsWith:
		return ok && strings.HasPrefix(str(v), str(c.Value))
	case EndsWith:
		return ok && strings.HasSuffix(str(v), str(c.Value))
	case Gt, Gte, Lt, Lte:
		if !ok {
			return false
		}
		a, aok := toFloat(v)
		b, bok := toFloat(c.Value)
		if !aok || !bok {
			return false
		}
		switch c.Op {
		case Gt:
			return a > b
		case Gte:
			return a >= b
		case Lt:
			return a < b
		default:
			return a <= b
		}
	case Matches:
		return ok && ev.matchRegexp(str(c.Value), str(v))
	case SemverGt, SemverGte, SemverLt, SemverLte:
		if !ok {
			return false
		}
		return matchSemver(c.Op, str(v), str(c.Value))
	}
	return false
}

func eqValues(a, b any) bool {
	if fa, ok := toFloat(a); ok {
		if fb, ok := toFloat(b); ok {
			return fa == fb
		}
	}
	return str(a) == str(b)
}

func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}
