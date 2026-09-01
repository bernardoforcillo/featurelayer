package flags

import "testing"

func TestSemverOps(t *testing.T) {
	attrs := map[string]any{"version": "1.2.10", "short": "v2.1", "pre": "1.0.0-alpha.1", "bad": "abc"}
	tests := []struct {
		name string
		c    Condition
		want bool
	}{
		{"gt numeric not lexical", Condition{Attribute: "version", Op: SemverGt, Value: "1.2.3"}, true},
		{"lt", Condition{Attribute: "version", Op: SemverLt, Value: "1.3.0"}, true},
		{"gte equal with v-prefix and 2 components", Condition{Attribute: "short", Op: SemverGte, Value: "2.1.0"}, true},
		{"gt equal is false", Condition{Attribute: "short", Op: SemverGt, Value: "2.1.0"}, false},
		{"release greater than prerelease", Condition{Attribute: "pre", Op: SemverLt, Value: "1.0.0"}, true},
		{"prerelease ordering alpha < alpha.1", Condition{Attribute: "pre", Op: SemverGt, Value: "1.0.0-alpha"}, true},
		{"prerelease numeric < alphanumeric", Condition{Attribute: "pre", Op: SemverLt, Value: "1.0.0-alpha.beta"}, true},
		{"build metadata ignored", Condition{Attribute: "short", Op: SemverGte, Value: "2.1.0+build.5"}, true},
		{"unparsable attr no match", Condition{Attribute: "bad", Op: SemverGt, Value: "1.0.0"}, false},
		{"unparsable ref no match", Condition{Attribute: "version", Op: SemverGt, Value: "not-a-version"}, false},
		{"missing attr no match", Condition{Attribute: "nope", Op: SemverGt, Value: "1.0.0"}, false},
	}
	ev := NewEvaluator()
	for _, tt := range tests {
		if got := ev.match(tt.c, attrs, false); got != tt.want {
			t.Errorf("%s: match = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestMatchesOp(t *testing.T) {
	attrs := map[string]any{"email": "user@acme.io", "n": 42}
	ev := NewEvaluator()
	if !ev.match(Condition{Attribute: "email", Op: Matches, Value: `@acme\.io$`}, attrs, false) {
		t.Error("anchored suffix pattern should match")
	}
	if ev.match(Condition{Attribute: "email", Op: Matches, Value: `^acme`}, attrs, false) {
		t.Error("pattern should not match")
	}
	if !ev.match(Condition{Attribute: "n", Op: Matches, Value: `^42$`}, attrs, false) {
		t.Error("non-string attributes match on their string form")
	}
	if ev.match(Condition{Attribute: "email", Op: Matches, Value: `(`}, attrs, false) {
		t.Error("invalid pattern must never match")
	}
	// cache: same pattern twice must reuse the compiled regexp (smoke: no panic, same result)
	for i := 0; i < 2; i++ {
		if !ev.match(Condition{Attribute: "email", Op: Matches, Value: `acme`}, attrs, false) {
			t.Error("cached pattern should still match")
		}
	}
}
