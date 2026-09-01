package flags

import "testing"

func TestMatchBasicOps(t *testing.T) {
	attrs := map[string]any{
		"tenant": "acme", "plan": "pro", "seats": 5, "ratio": 2.5,
		"addons": []string{"extra", "sso"}, "empty": "",
		"ids": []int{12, 3},
	}
	tests := []struct {
		name string
		c    Condition
		want bool
	}{
		{"eq string", Condition{Attribute: "tenant", Op: Eq, Value: "acme"}, true},
		{"eq string miss", Condition{Attribute: "tenant", Op: Eq, Value: "other"}, false},
		{"eq numeric int vs float", Condition{Attribute: "seats", Op: Eq, Value: 5.0}, true},
		{"eq mixed number/string", Condition{Attribute: "seats", Op: Eq, Value: "5"}, true},
		{"eq missing attr", Condition{Attribute: "nope", Op: Eq, Value: "x"}, false},
		{"neq", Condition{Attribute: "tenant", Op: NotEq, Value: "other"}, true},
		{"neq missing attr matches", Condition{Attribute: "nope", Op: NotEq, Value: "x"}, true},
		{"in", Condition{Attribute: "plan", Op: In, Values: []any{"free", "pro"}}, true},
		{"in miss", Condition{Attribute: "plan", Op: In, Values: []any{"free"}}, false},
		{"nin", Condition{Attribute: "plan", Op: NotIn, Values: []any{"free"}}, true},
		{"contains substring", Condition{Attribute: "tenant", Op: Contains, Value: "cm"}, true},
		{"contains slice membership", Condition{Attribute: "addons", Op: Contains, Value: "sso"}, true},
		{"contains slice miss", Condition{Attribute: "addons", Op: Contains, Value: "billing"}, false},
		{"contains int slice no substring match", Condition{Attribute: "ids", Op: Contains, Value: 2}, false},
		{"contains int slice membership", Condition{Attribute: "ids", Op: Contains, Value: 3}, true},
		{"contains int slice membership first", Condition{Attribute: "ids", Op: Contains, Value: 12}, true},
		{"startsWith", Condition{Attribute: "plan", Op: StartsWith, Value: "pr"}, true},
		{"endsWith", Condition{Attribute: "plan", Op: EndsWith, Value: "ro"}, true},
		{"gt", Condition{Attribute: "seats", Op: Gt, Value: 4}, true},
		{"gt equal is false", Condition{Attribute: "seats", Op: Gt, Value: 5}, false},
		{"gte equal", Condition{Attribute: "seats", Op: Gte, Value: 5}, true},
		{"lt float", Condition{Attribute: "ratio", Op: Lt, Value: 3}, true},
		{"lte", Condition{Attribute: "ratio", Op: Lte, Value: 2.5}, true},
		{"gt non-numeric no match", Condition{Attribute: "tenant", Op: Gt, Value: 1}, false},
		{"gt missing attr", Condition{Attribute: "nope", Op: Gt, Value: 1}, false},
		{"exists", Condition{Attribute: "empty", Op: Exists}, true},
		{"exists miss", Condition{Attribute: "nope", Op: Exists}, false},
		{"notExists", Condition{Attribute: "nope", Op: NotExists}, true},
		{"unknown op", Condition{Attribute: "tenant", Op: "bogus", Value: "acme"}, false},
	}
	ev := NewEvaluator()
	for _, tt := range tests {
		if got := ev.match(tt.c, attrs, false); got != tt.want {
			t.Errorf("%s: match = %v, want %v", tt.name, got, tt.want)
		}
	}
}
