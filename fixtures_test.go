package featurelayer

import (
	"time"

	"github.com/bernardoforcillo/featurelayer/catalog"
	"github.com/bernardoforcillo/featurelayer/entitlement"
	"github.com/bernardoforcillo/featurelayer/flags"
)

var tNow = time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

func d(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// fullTestConfig exercises every subsystem. Golden buckets (seed
// "export.csv"): tenant-1=18.31, tenant-2=0.42, tenant-3=82.53, acme=54.83;
// (seed "new-editor"): tenant-1=86.51.
func fullTestConfig() Config {
	return Config{
		Features: []catalog.Feature{
			{Key: "export.csv", Lifecycle: catalog.GA},
			{Key: "api.calls", Lifecycle: catalog.GA},
			{Key: "sso.saml", Lifecycle: catalog.GA},
			{Key: "new-editor", Lifecycle: catalog.Beta, Free: true},
			{Key: "editor.pro", Lifecycle: catalog.GA, Free: true, DependsOn: []catalog.Key{"new-editor"}},
			{Key: "old.widget", Lifecycle: catalog.Deprecated, Free: true},
			{Key: "secret", Lifecycle: catalog.Draft, Free: true},
			{Key: "gone", Lifecycle: catalog.Retired, Free: true},
			{Key: "killed.feature", Lifecycle: catalog.GA, Free: true},
			{Key: "windowed.feature", Lifecycle: catalog.GA, Free: true},
			{Key: "plain.feature", Lifecycle: catalog.GA, Free: true},
			{Key: "ai.tokens", Lifecycle: catalog.GA},
		},
		Segments: []flags.Segment{
			{Key: "beta-testers", Rules: []flags.SegmentRule{
				{Conditions: []flags.Condition{{Attribute: "tenant", Op: flags.In, Values: []any{"acme"}}}},
			}},
		},
		Flags: []flags.Flag{
			{Feature: "export.csv", Enabled: true,
				Rules: []flags.Rule{
					{Name: "beta", Conditions: []flags.Condition{{Op: flags.InSegment, Value: "beta-testers"}}, Serve: flags.Serve{On: true}},
					{Name: "pro-only-check", Conditions: []flags.Condition{{Attribute: "plan", Op: flags.Eq, Value: "legacy"}}, Serve: flags.Serve{On: false}},
				},
				Default: flags.Serve{Rollout: flags.Percent(20)},
			},
			{Feature: "new-editor", Enabled: true, Default: flags.Serve{Rollout: flags.Percent(20)}},
			{Feature: "killed.feature", Enabled: false},
			{Feature: "windowed.feature", Enabled: true, Window: &flags.Window{From: tNow.Add(24 * time.Hour)}, Default: flags.Serve{On: true}},
			{Feature: "plain.feature", Enabled: true, Default: flags.Serve{On: true}},
		},
		Plans: []entitlement.Plan{
			{ID: "free", Entitlements: []entitlement.Entitlement{entitlement.Limited("api.calls", 100, entitlement.Month)}},
			{ID: "pro", Extends: "free", Entitlements: []entitlement.Entitlement{
				{Feature: "export.csv"},
				entitlement.Limited("api.calls", 1000, entitlement.Month),
				entitlement.LimitedPer("ai.tokens", 10, entitlement.Day, entitlement.PerSubject),
			}},
		},
		AddOns: []entitlement.AddOn{
			{ID: "extra-calls", Requires: []entitlement.PlanID{"pro"}, Entitlements: []entitlement.Entitlement{entitlement.Limited("api.calls", 500, entitlement.Month)}},
		},
	}
}

// usageKeyFor is the tenant-scoped monthly counter key at tNow.
func usageKeyFor(tenant string, feature catalog.Key) entitlement.UsageKey {
	return entitlement.UsageKey{Tenant: tenant, Feature: feature, Period: entitlement.PeriodKey(entitlement.Month, time.Time{}, tNow)}
}
