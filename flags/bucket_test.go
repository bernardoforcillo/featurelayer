package flags

import (
	"fmt"
	"testing"
)

func TestBucketGolden(t *testing.T) {
	golden := []struct {
		seed, attr string
		want       float64
	}{
		{"export.csv", "tenant-1", 18.31},
		{"export.csv", "tenant-2", 0.42},
		{"export.csv", "tenant-3", 82.53},
		{"new-editor", "tenant-1", 86.51}, // same attr, different seed → different bucket
		{"export.csv", "acme", 54.83},
		{"api.calls", "acme", 98.32},
	}
	for _, g := range golden {
		if got := bucketOf(g.seed, g.attr); got != g.want {
			t.Errorf("bucketOf(%q, %q) = %.2f, want %.2f", g.seed, g.attr, got, g.want)
		}
	}
}

func TestBucketStableAndUniform(t *testing.T) {
	for i := 0; i < 10; i++ {
		if bucketOf("seed", "key") != bucketOf("seed", "key") {
			t.Fatal("bucket must be deterministic")
		}
	}
	below := 0
	for i := 0; i < 100000; i++ {
		b := bucketOf("export.csv", fmt.Sprintf("t-%d", i))
		if b < 0 || b >= 100 {
			t.Fatalf("bucket %v out of [0,100)", b)
		}
		if b < 20 {
			below++
		}
	}
	// reference run measured 20.078%; assert a generous band
	if below < 19000 || below > 21000 {
		t.Errorf("share below 20%% = %d/100000, want ~20000", below)
	}
}
