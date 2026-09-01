package flags

import "testing"

func testSegments() []Segment {
	return []Segment{
		{Key: "beta-testers", Rules: []SegmentRule{
			{Name: "named tenants", Conditions: []Condition{{Attribute: "tenant", Op: In, Values: []any{"acme", "globex"}}}},
			{Name: "acme domain", Conditions: []Condition{{Attribute: "email", Op: EndsWith, Value: "@acme.io"}}},
		}},
		{Key: "empty-rule-guard", Rules: []SegmentRule{{Conditions: nil}}},
	}
}

func TestInSegment(t *testing.T) {
	ev := NewEvaluator(testSegments()...)
	if !ev.InSegment("beta-testers", map[string]any{"tenant": "acme"}) {
		t.Error("first rule should match")
	}
	if !ev.InSegment("beta-testers", map[string]any{"tenant": "other", "email": "x@acme.io"}) {
		t.Error("second rule should match (OR between rules)")
	}
	if ev.InSegment("beta-testers", map[string]any{"tenant": "other"}) {
		t.Error("no rule should match")
	}
	if ev.InSegment("empty-rule-guard", map[string]any{"tenant": "x"}) {
		t.Error("a rule without conditions must not match anyone")
	}
	if ev.InSegment("unknown", map[string]any{"tenant": "acme"}) {
		t.Error("unknown segment has no members")
	}
}

func TestSegmentOps(t *testing.T) {
	ev := NewEvaluator(testSegments()...)
	attrs := map[string]any{"tenant": "acme"}
	if !ev.match(Condition{Op: InSegment, Value: "beta-testers"}, attrs, false) {
		t.Error("inSegment should match")
	}
	if ev.match(Condition{Op: NotInSegment, Value: "beta-testers"}, attrs, false) {
		t.Error("notInSegment should not match a member")
	}
	if !ev.match(Condition{Op: NotInSegment, Value: "unknown"}, attrs, false) {
		t.Error("notInSegment on unknown segment is true")
	}
	if ev.match(Condition{Op: InSegment, Value: "beta-testers"}, attrs, true) {
		t.Error("segment ops inside a segment must never match")
	}
	var zero Evaluator
	if zero.InSegment("beta-testers", attrs) {
		t.Error("zero-value evaluator has no segments")
	}
}
