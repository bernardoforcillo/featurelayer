package featurelayer

import (
	"errors"
	"strings"
	"testing"

	"github.com/bernardoforcillo/featurelayer/catalog"
	"github.com/bernardoforcillo/featurelayer/entitlement"
	"github.com/bernardoforcillo/featurelayer/flags"
)

func feat(key catalog.Key) catalog.Feature {
	return catalog.Feature{Key: key, Lifecycle: catalog.GA}
}

// wantErr asserts that building cfg fails with a ValidationError whose
// Path equals path.
func wantErr(t *testing.T, cfg Config, path string) {
	t.Helper()
	_, err := NewSnapshot(cfg)
	if err == nil {
		t.Fatalf("want error at %q, got nil", path)
	}
	for _, e := range flattenErrs(err) {
		var ve *ValidationError
		if errors.As(e, &ve) && ve.Path == path {
			return
		}
	}
	t.Fatalf("no ValidationError at %q in: %v", path, err)
}

func flattenErrs(err error) []error {
	if u, ok := err.(interface{ Unwrap() []error }); ok {
		return u.Unwrap()
	}
	return []error{err}
}

func TestValidation(t *testing.T) {
	ok := Config{Features: []catalog.Feature{feat("a")}}
	if _, err := NewSnapshot(ok); err != nil {
		t.Fatalf("minimal config must validate: %v", err)
	}

	wantErr(t, Config{Features: []catalog.Feature{{Key: "Bad!", Lifecycle: catalog.GA}}}, "features[0].key")
	wantErr(t, Config{Features: []catalog.Feature{{Key: "a"}}}, "features[0].lifecycle")
	wantErr(t, Config{Features: []catalog.Feature{feat("a"), feat("a")}}, "features[1].key")
	wantErr(t, Config{Features: []catalog.Feature{{Key: "a", Lifecycle: catalog.GA, DependsOn: []catalog.Key{"ghost"}}}}, "features[0].dependsOn[0]")
	wantErr(t, Config{Features: []catalog.Feature{{Key: "a", Lifecycle: catalog.GA, DependsOn: []catalog.Key{"a"}}}}, "features[0].dependsOn[0]")
	cyc := Config{Features: []catalog.Feature{
		{Key: "a", Lifecycle: catalog.GA, DependsOn: []catalog.Key{"b"}},
		{Key: "b", Lifecycle: catalog.GA, DependsOn: []catalog.Key{"a"}},
	}}
	wantErr(t, cyc, "features[0].dependsOn")

	// two disjoint cycles must both be reported, not just the first found
	disjointCycles := Config{Features: []catalog.Feature{
		{Key: "a", Lifecycle: catalog.GA, DependsOn: []catalog.Key{"b"}},
		{Key: "b", Lifecycle: catalog.GA, DependsOn: []catalog.Key{"a"}},
		{Key: "c", Lifecycle: catalog.GA, DependsOn: []catalog.Key{"d"}},
		{Key: "d", Lifecycle: catalog.GA, DependsOn: []catalog.Key{"c"}},
	}}
	wantErr(t, disjointCycles, "features[0].dependsOn")
	wantErr(t, disjointCycles, "features[2].dependsOn")

	seg := func(s flags.Segment) Config {
		return Config{Features: []catalog.Feature{feat("a")}, Segments: []flags.Segment{s}}
	}
	wantErr(t, seg(flags.Segment{Key: "Bad!"}), "segments[0].key")
	wantErr(t, seg(flags.Segment{Key: "s"}), "segments[0].rules")
	wantErr(t, seg(flags.Segment{Key: "s", Rules: []flags.SegmentRule{{}}}), "segments[0].rules[0].conditions")
	wantErr(t, seg(flags.Segment{Key: "s", Rules: []flags.SegmentRule{
		{Conditions: []flags.Condition{{Op: flags.InSegment, Value: "s"}}},
	}}), "segments[0].rules[0].conditions[0].op")

	fl := func(f flags.Flag) Config {
		return Config{Features: []catalog.Feature{feat("a")}, Flags: []flags.Flag{f}}
	}
	wantErr(t, fl(flags.Flag{Feature: "ghost", Enabled: true}), "flags[0].feature")
	two := Config{Features: []catalog.Feature{feat("a")}, Flags: []flags.Flag{{Feature: "a", Enabled: true}, {Feature: "a", Enabled: true}}}
	wantErr(t, two, "flags[1].feature")
	wantErr(t, fl(flags.Flag{Feature: "a", Enabled: true, Variants: []flags.Variant{{Key: ""}}}), "flags[0].variants[0].key")
	wantErr(t, fl(flags.Flag{Feature: "a", Enabled: true, Variants: []flags.Variant{{Key: "x"}, {Key: "x"}}}), "flags[0].variants[1].key")
	wantErr(t, fl(flags.Flag{Feature: "a", Enabled: true, Default: flags.Serve{On: true, Variant: "ghost"}}), "flags[0].default.variant")
	wantErr(t, fl(flags.Flag{Feature: "a", Enabled: true,
		Rules: []flags.Rule{{Serve: flags.Serve{Rollout: &flags.Rollout{Split: []flags.Portion{{Percent: 60}, {Percent: 50}}}}}},
	}), "flags[0].rules[0].serve.rollout.split")
	wantErr(t, fl(flags.Flag{Feature: "a", Enabled: true,
		Default: flags.Serve{Rollout: &flags.Rollout{Split: []flags.Portion{{Percent: -1}}}},
	}), "flags[0].default.rollout.split[0].percent")
	wantErr(t, fl(flags.Flag{Feature: "a", Enabled: true,
		Rules: []flags.Rule{{Conditions: []flags.Condition{{Attribute: "x", Op: "bogus", Value: 1}}, Serve: flags.Serve{On: true}}},
	}), "flags[0].rules[0].conditions[0].op")
	wantErr(t, fl(flags.Flag{Feature: "a", Enabled: true,
		Rules: []flags.Rule{{Conditions: []flags.Condition{{Op: flags.Eq, Value: 1}}, Serve: flags.Serve{On: true}}},
	}), "flags[0].rules[0].conditions[0].attribute")
	wantErr(t, fl(flags.Flag{Feature: "a", Enabled: true,
		Rules: []flags.Rule{{Conditions: []flags.Condition{{Attribute: "x", Op: flags.InSegment, Value: "ghost"}}, Serve: flags.Serve{On: true}}},
	}), "flags[0].rules[0].conditions[0].attribute") // segment ops require empty attribute
	wantErr(t, fl(flags.Flag{Feature: "a", Enabled: true,
		Rules: []flags.Rule{{Conditions: []flags.Condition{{Op: flags.InSegment, Value: "ghost"}}, Serve: flags.Serve{On: true}}},
	}), "flags[0].rules[0].conditions[0].value") // unknown segment
	wantErr(t, fl(flags.Flag{Feature: "a", Enabled: true,
		Rules: []flags.Rule{{Conditions: []flags.Condition{{Attribute: "x", Op: flags.Matches, Value: "("}}, Serve: flags.Serve{On: true}}},
	}), "flags[0].rules[0].conditions[0].value") // invalid regexp
	wantErr(t, fl(flags.Flag{Feature: "a", Enabled: true,
		Rules: []flags.Rule{{Conditions: []flags.Condition{{Attribute: "x", Op: flags.SemverGt, Value: "nope"}}, Serve: flags.Serve{On: true}}},
	}), "flags[0].rules[0].conditions[0].value") // invalid semver
	wantErr(t, fl(flags.Flag{Feature: "a", Enabled: true,
		Rules: []flags.Rule{{Conditions: []flags.Condition{{Attribute: "x", Op: flags.In}}, Serve: flags.Serve{On: true}}},
	}), "flags[0].rules[0].conditions[0].values")
	wantErr(t, fl(flags.Flag{Feature: "a", Enabled: true,
		Rules: []flags.Rule{{Conditions: []flags.Condition{{Attribute: "x", Op: flags.Exists, Value: 1}}, Serve: flags.Serve{On: true}}},
	}), "flags[0].rules[0].conditions[0].value")
	badWin := flags.Flag{Feature: "a", Enabled: true, Window: &flags.Window{From: d("2026-02-01T00:00:00Z"), Until: d("2026-01-01T00:00:00Z")}}
	wantErr(t, fl(badWin), "flags[0].window")

	pl := func(plans []entitlement.Plan, addons []entitlement.AddOn) Config {
		return Config{Features: []catalog.Feature{feat("a")}, Plans: plans, AddOns: addons}
	}
	wantErr(t, pl([]entitlement.Plan{{ID: ""}}, nil), "plans[0].id")
	wantErr(t, pl([]entitlement.Plan{{ID: "p"}, {ID: "p"}}, nil), "plans[1].id")
	wantErr(t, pl([]entitlement.Plan{{ID: "p", Extends: "ghost"}}, nil), "plans[0].extends")
	wantErr(t, pl([]entitlement.Plan{{ID: "p", Extends: "q"}, {ID: "q", Extends: "p"}}, nil), "plans[0].extends")
	wantErr(t, pl([]entitlement.Plan{{ID: "p", Entitlements: []entitlement.Entitlement{{Feature: "ghost"}}}}, nil), "plans[0].entitlements[0].feature")
	wantErr(t, pl([]entitlement.Plan{{ID: "p", Entitlements: []entitlement.Entitlement{{Feature: "a"}, {Feature: "a"}}}}, nil), "plans[0].entitlements[1].feature")
	wantErr(t, pl([]entitlement.Plan{{ID: "p", Entitlements: []entitlement.Entitlement{{Feature: "a", Limit: &entitlement.Limit{Max: -1}}}}}, nil), "plans[0].entitlements[0].limit.max")
	wantErr(t, pl([]entitlement.Plan{{ID: "p", Entitlements: []entitlement.Entitlement{{Feature: "a", Limit: &entitlement.Limit{Max: 1, Period: "decade"}}}}}, nil), "plans[0].entitlements[0].limit.period")
	wantErr(t, pl(nil, []entitlement.AddOn{{ID: "x", Requires: []entitlement.PlanID{"ghost"}}}), "addons[0].requires[0]")
	// period disagreement between plan and addon on the same feature
	disagree := pl(
		[]entitlement.Plan{{ID: "p", Entitlements: []entitlement.Entitlement{entitlement.Limited("a", 10, entitlement.Month)}}},
		[]entitlement.AddOn{{ID: "x", Entitlements: []entitlement.Entitlement{entitlement.Limited("a", 5, entitlement.Day)}}},
	)
	wantErr(t, disagree, "addons[0].entitlements[0].limit.period")

	// period disagreement between two plans must deterministically blame
	// the second (declaration order), not whichever the map happened to
	// range over first
	planDisagree := pl([]entitlement.Plan{
		{ID: "p", Entitlements: []entitlement.Entitlement{entitlement.Limited("a", 10, entitlement.Month)}},
		{ID: "q", Entitlements: []entitlement.Entitlement{entitlement.Limited("a", 10, entitlement.Day)}},
	}, nil)
	wantErr(t, planDisagree, "plans[1].entitlements[0].limit.period")

	// multiple problems reported at once
	_, err := NewSnapshot(Config{Features: []catalog.Feature{{Key: "Bad!"}, {Key: "also bad"}}})
	if err == nil || len(flattenErrs(err)) < 3 {
		t.Errorf("want ≥3 joined errors, got: %v", err)
	}
	if !strings.Contains(err.Error(), "features[0].key") {
		t.Errorf("Error() must carry paths: %v", err)
	}
}
