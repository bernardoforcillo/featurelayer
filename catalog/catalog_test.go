package catalog

import "testing"

func TestValidateKey(t *testing.T) {
	valid := []Key{"a", "export.csv", "api-calls", "a1_b2", "x.y-z_9"}
	for _, k := range valid {
		if err := ValidateKey(k); err != nil {
			t.Errorf("ValidateKey(%q) = %v, want nil", k, err)
		}
	}
	invalid := []Key{"", "Export", ".a", "-a", "_a", "a b", "à", "a/b", Key("a" + string(make([]byte, 128)))}
	for _, k := range invalid {
		if err := ValidateKey(k); err == nil {
			t.Errorf("ValidateKey(%q) = nil, want error", k)
		}
	}
}

func TestLifecycleValid(t *testing.T) {
	for _, l := range []Lifecycle{Draft, Beta, GA, Deprecated, Retired} {
		if !l.Valid() {
			t.Errorf("%q should be valid", l)
		}
	}
	for _, l := range []Lifecycle{"", "alpha", "GA"} {
		if l.Valid() {
			t.Errorf("%q should be invalid", l)
		}
	}
}
