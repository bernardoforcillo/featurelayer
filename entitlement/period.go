package entitlement

import "time"

// PeriodStart returns the start of the period containing now.
// A zero anchor means UTC-calendar alignment; a non-zero anchor makes
// boundaries fall on the anchor's day-of-month and time-of-day (day
// clamped to shorter months). Instants before the anchor are counted
// backwards from it. For None the zero time is returned.
func PeriodStart(p Period, anchor, now time.Time) time.Time {
	s, _ := periodBounds(p, anchor, now)
	return s
}

// PeriodEnd returns the start of the next period (zero time for None).
func PeriodEnd(p Period, anchor, now time.Time) time.Time {
	_, e := periodBounds(p, anchor, now)
	return e
}

// PeriodKey returns "" for None, else PeriodStart in RFC3339 UTC.
func PeriodKey(p Period, anchor, now time.Time) string {
	if p == None {
		return ""
	}
	return PeriodStart(p, anchor, now).UTC().Format(time.RFC3339)
}

func periodBounds(p Period, anchor, now time.Time) (time.Time, time.Time) {
	now = now.UTC()
	switch p {
	case Day, Week:
		d := 24 * time.Hour
		if p == Week {
			d = 7 * 24 * time.Hour
		}
		if anchor.IsZero() {
			day := now.Truncate(24 * time.Hour) // epoch-aligned = UTC midnight
			if p == Day {
				return day, day.Add(d)
			}
			offset := (int(day.Weekday()) + 6) % 7 // Monday = 0
			start := day.AddDate(0, 0, -offset)
			return start, start.Add(d)
		}
		a := anchor.UTC()
		diff := now.Sub(a)
		k := diff / d
		if diff < 0 && diff%d != 0 {
			k--
		}
		start := a.Add(k * d)
		return start, start.Add(d)
	case Month, Year:
		step := 1
		if p == Year {
			step = 12
		}
		if anchor.IsZero() {
			var start time.Time
			if p == Month {
				start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
			} else {
				start = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
			}
			return start, start.AddDate(0, step, 0)
		}
		a := anchor.UTC()
		n := ((now.Year()-a.Year())*12 + int(now.Month()) - int(a.Month()))
		n = (n / step) * step
		start := addMonthsClamped(a, n)
		for start.After(now) {
			n -= step
			start = addMonthsClamped(a, n)
		}
		for {
			next := addMonthsClamped(a, n+step)
			if next.After(now) {
				return start, next
			}
			n += step
			start = next
		}
	}
	return time.Time{}, time.Time{}
}

// addMonthsClamped adds n months to a, clamping the day to the target
// month's length (Jan 31 + 1 month = Feb 28/29, never Mar 2/3).
func addMonthsClamped(a time.Time, n int) time.Time {
	y, m := a.Year(), int(a.Month())-1+n
	y += m / 12
	m %= 12
	if m < 0 {
		m += 12
		y--
	}
	month := time.Month(m + 1)
	day := a.Day()
	if last := time.Date(y, month+1, 0, 0, 0, 0, 0, time.UTC).Day(); day > last {
		day = last
	}
	return time.Date(y, month, day, a.Hour(), a.Minute(), a.Second(), a.Nanosecond(), time.UTC)
}
