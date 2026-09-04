package entitlement

import (
	"encoding/json"
	"testing"
)

func TestLimitScope(t *testing.T) {
	for _, s := range []LimitScope{"", PerTenant, PerSubject} {
		if !s.Valid() {
			t.Errorf("%q must be valid", s)
		}
	}
	for _, s := range []LimitScope{"user", "SUBJECT", "per-tenant"} {
		if s.Valid() {
			t.Errorf("%q must be invalid", s)
		}
	}
	if (Limit{}).Scope() != PerTenant {
		t.Error("empty Per must resolve to PerTenant")
	}
	if (Limit{Per: PerSubject}).Scope() != PerSubject {
		t.Error("explicit Per must be kept")
	}
	if (Limit{Per: "bogus"}).Scope() != "bogus" {
		t.Error("Scope must not hide an invalid value")
	}
}

func TestLimitedPer(t *testing.T) {
	e := LimitedPer("seats", 5, Month, PerSubject)
	if e.Feature != "seats" || e.Limit == nil || e.Limit.Max != 5 || e.Limit.Period != Month || e.Limit.Per != PerSubject {
		t.Errorf("LimitedPer = %+v", e)
	}
	l := Limited("seats", 5, Month)
	if l.Limit.Per != "" || l.Limit.Scope() != PerTenant {
		t.Errorf("Limited must stay tenant-scoped: %+v", l.Limit)
	}
}

func TestLimitJSONRoundTrip(t *testing.T) {
	// The default scope is omitted, so existing configs are byte-identical.
	data, err := json.Marshal(Limit{Max: 5, Period: Month})
	if err != nil || string(data) != `{"max":5,"period":"month"}` {
		t.Errorf("tenant-scoped limit = %s, %v", data, err)
	}
	data, err = json.Marshal(Limit{Max: 5, Period: Day, Per: PerSubject})
	if err != nil || string(data) != `{"max":5,"period":"day","per":"subject"}` {
		t.Errorf("subject-scoped limit = %s, %v", data, err)
	}
	var back Limit
	if err := json.Unmarshal(data, &back); err != nil || back.Per != PerSubject || back.Max != 5 || back.Period != Day {
		t.Errorf("unmarshal = %+v, %v", back, err)
	}
	var loose Limit
	if err := json.Unmarshal([]byte(`{"max":1,"per":"whatever"}`), &loose); err != nil {
		t.Fatal(err)
	}
	if loose.Per.Valid() {
		t.Error("unknown scope must decode and then fail Valid, so validation can report it")
	}
}
