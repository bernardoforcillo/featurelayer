package entitlement

import (
	"testing"
	"time"
)

func d(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestPeriodCalendar(t *testing.T) {
	var zero time.Time
	tests := []struct {
		p               Period
		now, start, end string
	}{
		{Day, "2026-09-01T15:04:05Z", "2026-09-01T00:00:00Z", "2026-09-02T00:00:00Z"},
		{Week, "2026-09-01T12:00:00Z", "2026-08-31T00:00:00Z", "2026-09-07T00:00:00Z"}, // 2026-09-01 is a Tuesday
		{Week, "2026-08-31T00:00:00Z", "2026-08-31T00:00:00Z", "2026-09-07T00:00:00Z"}, // boundary inclusive
		{Month, "2026-02-14T09:00:00Z", "2026-02-01T00:00:00Z", "2026-03-01T00:00:00Z"},
		{Year, "2026-09-01T00:00:00Z", "2026-01-01T00:00:00Z", "2027-01-01T00:00:00Z"},
	}
	for _, tt := range tests {
		if got := PeriodStart(tt.p, zero, d(tt.now)); !got.Equal(d(tt.start)) {
			t.Errorf("PeriodStart(%v, cal, %s) = %v, want %s", tt.p, tt.now, got, tt.start)
		}
		if got := PeriodEnd(tt.p, zero, d(tt.now)); !got.Equal(d(tt.end)) {
			t.Errorf("PeriodEnd(%v, cal, %s) = %v, want %s", tt.p, tt.now, got, tt.end)
		}
	}
	if !PeriodStart(None, zero, d("2026-09-01T00:00:00Z")).IsZero() {
		t.Error("None has no period start")
	}
	if PeriodKey(None, zero, d("2026-09-01T00:00:00Z")) != "" {
		t.Error("None has empty key")
	}
	if k := PeriodKey(Month, zero, d("2026-02-14T09:00:00Z")); k != "2026-02-01T00:00:00Z" {
		t.Errorf("key = %q", k)
	}
}

func TestPeriodAnchored(t *testing.T) {
	tests := []struct {
		p                       Period
		anchor, now, start, end string
	}{
		// month boundaries on the anchor's day/time, clamped in short months
		{Month, "2026-01-31T10:30:00Z", "2026-02-14T09:00:00Z", "2026-01-31T10:30:00Z", "2026-02-28T10:30:00Z"},
		{Month, "2026-01-31T10:30:00Z", "2026-03-01T00:00:00Z", "2026-02-28T10:30:00Z", "2026-03-31T10:30:00Z"},
		{Month, "2024-01-31T00:00:00Z", "2024-02-29T00:00:00Z", "2024-02-29T00:00:00Z", "2024-03-31T00:00:00Z"}, // leap year, boundary inclusive
		// before the anchor: counted backwards
		{Month, "2026-03-15T00:00:00Z", "2026-03-01T00:00:00Z", "2026-02-15T00:00:00Z", "2026-03-15T00:00:00Z"},
		// day / week are anchor + k*24h / k*168h
		{Day, "2026-01-01T08:00:00Z", "2026-01-02T07:59:59Z", "2026-01-01T08:00:00Z", "2026-01-02T08:00:00Z"},
		{Day, "2026-01-01T08:00:00Z", "2026-01-02T08:00:00Z", "2026-01-02T08:00:00Z", "2026-01-03T08:00:00Z"},
		{Week, "2026-01-01T08:00:00Z", "2026-01-10T00:00:00Z", "2026-01-08T08:00:00Z", "2026-01-15T08:00:00Z"},
		{Day, "2026-01-10T08:00:00Z", "2026-01-08T09:00:00Z", "2026-01-08T08:00:00Z", "2026-01-09T08:00:00Z"}, // before anchor
		// year on the anniversary
		{Year, "2025-06-10T00:00:00Z", "2026-06-09T23:59:59Z", "2025-06-10T00:00:00Z", "2026-06-10T00:00:00Z"},
		{Year, "2025-06-10T00:00:00Z", "2026-06-10T00:00:00Z", "2026-06-10T00:00:00Z", "2027-06-10T00:00:00Z"},
	}
	for _, tt := range tests {
		start, end := PeriodStart(tt.p, d(tt.anchor), d(tt.now)), PeriodEnd(tt.p, d(tt.anchor), d(tt.now))
		if !start.Equal(d(tt.start)) {
			t.Errorf("PeriodStart(%v, %s, %s) = %v, want %s", tt.p, tt.anchor, tt.now, start, tt.start)
		}
		if !end.Equal(d(tt.end)) {
			t.Errorf("PeriodEnd(%v, %s, %s) = %v, want %s", tt.p, tt.anchor, tt.now, end, tt.end)
		}
		// continuity: the next period starts exactly where this one ends
		if next := PeriodStart(tt.p, d(tt.anchor), end); !next.Equal(end) {
			t.Errorf("continuity broken: PeriodStart at end %v = %v", end, next)
		}
	}
	if k := PeriodKey(Month, d("2026-01-31T10:30:00Z"), d("2026-02-14T09:00:00Z")); k != "2026-01-31T10:30:00Z" {
		t.Errorf("anchored key = %q", k)
	}
}
