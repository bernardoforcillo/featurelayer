package featurelayer

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestLoadFileTestdata(t *testing.T) {
	snap, err := LoadFile("testdata/config.json")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if len(snap.Features()) != 4 || len(snap.Plans()) != 2 || len(snap.AddOns()) != 1 || len(snap.Segments()) != 1 {
		t.Errorf("loaded shape: %d features %d plans %d addons %d segments", len(snap.Features()), len(snap.Plans()), len(snap.AddOns()), len(snap.Segments()))
	}
	// The per-subject limit came through the JSON "per" tag.
	for _, e := range snap.PlanEntitlements("pro") {
		if e.Feature == "ai.tokens" && (e.Limit == nil || e.Limit.Scope() != "subject") {
			t.Errorf("ai.tokens limit = %+v, want per-subject", e.Limit)
		}
	}
}

func TestLoadFileErrors(t *testing.T) {
	if _, err := LoadFile("testdata/does-not-exist.json"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file: %v", err)
	}
	_, err := LoadFile("testdata/invalid.json")
	if err == nil {
		t.Fatal("invalid.json must fail")
	}
	// Every problem is reported at once, as *ValidationError.
	paths := map[string]bool{}
	for _, e := range flattenErrs(err) {
		var ve *ValidationError
		if errors.As(e, &ve) {
			paths[ve.Path] = true
		}
	}
	for _, want := range []string{
		"features[1].key",
		"flags[0].feature",
		"plans[0].extends",
		"plans[0].entitlements[0].limit.period",
		"plans[0].entitlements[0].limit.per",
	} {
		if !paths[want] {
			t.Errorf("missing validation error at %q; got %v", want, err)
		}
	}
}

func TestLoadJSONStrict(t *testing.T) {
	// A typo'd key is an error, not a silently ignored field.
	_, err := LoadJSON(strings.NewReader(`{"features":[{"key":"a","lifecycle":"ga"}],"entitlments":[]}`))
	if err == nil || !strings.Contains(err.Error(), "entitlments") {
		t.Errorf("unknown field must be reported: %v", err)
	}
	// Malformed JSON.
	if _, err := LoadJSON(strings.NewReader(`{"features": [`)); err == nil {
		t.Error("truncated JSON must fail")
	}
	// Trailing content after the document.
	if _, err := LoadJSON(strings.NewReader(`{"features":[{"key":"a","lifecycle":"ga"}]} {"features":[]}`)); err == nil {
		t.Error("a second document must fail")
	}
	// Minimal valid document.
	snap, err := LoadJSON(strings.NewReader(`{"features":[{"key":"a","lifecycle":"ga"}]}`))
	if err != nil || snap == nil {
		t.Errorf("minimal config: %v", err)
	}
}

func TestReload(t *testing.T) {
	var applied []ApplyEvent
	e, _, _ := testEngine(t, WithApplyHook(func(ev ApplyEvent) { applied = append(applied, ev) }))
	before := e.Snapshot()

	// Failure: the old snapshot stays, no hook fires, the error surfaces.
	boom := errors.New("boom")
	if err := e.Reload(func() (*Snapshot, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Errorf("Reload error = %v, want boom", err)
	}
	if err := e.Reload(func() (*Snapshot, error) { return nil, nil }); !errors.Is(err, ErrNilSnapshot) {
		t.Errorf("Reload nil = %v, want ErrNilSnapshot", err)
	}
	if e.Snapshot() != before || len(applied) != 0 {
		t.Errorf("failed Reload must not touch the engine: same=%v hooks=%d", e.Snapshot() == before, len(applied))
	}

	// Success: applied, hook fired once with the pair.
	if err := e.Reload(func() (*Snapshot, error) { return LoadFile("testdata/config.json") }); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if e.Snapshot() == before || len(applied) != 1 || applied[0].Prev != before || applied[0].Next != e.Snapshot() {
		t.Errorf("successful Reload: swapped=%v events=%+v", e.Snapshot() != before, applied)
	}
	if applied[0].At != tNow {
		t.Errorf("apply event uses the engine clock: %v", applied[0].At)
	}
	// The new snapshot is live: "ai.tokens" exists only in testdata.
	if d := e.Evaluate(t.Context(), "old.widget", EvalContext{TenantID: "acme"}); d.Reason != ReasonUnknownFeature {
		t.Errorf("old snapshot still served: %+v", d)
	}
}
