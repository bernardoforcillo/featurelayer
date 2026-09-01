package featurelayer

import (
	"encoding/json"
	"testing"
)

func TestConfigJSONRoundTrip(t *testing.T) {
	cfg := fullTestConfig()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var back Config
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSnapshot(back); err != nil {
		t.Fatalf("round-tripped config must validate: %v", err)
	}
	data2, err := json.Marshal(back)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(data2) {
		t.Error("marshal → unmarshal → marshal must be stable")
	}
}
