package domain

import (
	"testing"
	"time"
)

// testCalendar is the shared US Eastern calendar the whole team runs on.
func testCalendar(t testing.TB) *Calendar {
	t.Helper()
	cal, err := LoadCalendar(DefaultTimezone)
	if err != nil {
		t.Fatalf("LoadCalendar: %v", err)
	}
	return cal
}

// at parses a wall-clock time as it would be read on a clock in the calendar's
// timezone.
func at(t testing.TB, cal *Calendar, layout, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation(layout, value, cal.Location())
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return parsed
}

const wall = "2006-01-02 15:04:05"

func TestCalendarKeysAndBoundaries(t *testing.T) {
	cal := testCalendar(t)

	cases := []struct {
		name      string
		cadence   Cadence
		when      string
		wantKey   string
		wantStart string
		wantEnd   string
	}{
		{
			name: "daily mid-afternoon", cadence: Daily,
			when: "2026-09-03 14:22:07", wantKey: "2026-09-03",
			wantStart: "2026-09-03 00:00:00", wantEnd: "2026-09-04 00:00:00",
		},
		{
			// Thursday 2026-09-03 sits in the week that began Monday the 31st of
			// August, so the weekly key belongs to ISO week 36.
			name: "weekly resolves back to Monday", cadence: Weekly,
			when: "2026-09-03 14:22:07", wantKey: "2026-W36",
			wantStart: "2026-08-31 00:00:00", wantEnd: "2026-09-07 00:00:00",
		},
		{
			name: "weekly on the Monday itself", cadence: Weekly,
			when: "2026-08-31 00:00:00", wantKey: "2026-W36",
			wantStart: "2026-08-31 00:00:00", wantEnd: "2026-09-07 00:00:00",
		},
		{
			// Sunday is the last day of the work week, not the first.
			name: "weekly on the closing Sunday", cadence: Weekly,
			when: "2026-09-06 23:59:59", wantKey: "2026-W36",
			wantStart: "2026-08-31 00:00:00", wantEnd: "2026-09-07 00:00:00",
		},
		{
			name: "monthly", cadence: Monthly,
			when: "2026-09-03 14:22:07", wantKey: "2026-M09",
			wantStart: "2026-09-01 00:00:00", wantEnd: "2026-10-01 00:00:00",
		},
		{
			name: "quarterly Q3", cadence: Quarterly,
			when: "2026-09-03 14:22:07", wantKey: "2026-Q3",
			wantStart: "2026-07-01 00:00:00", wantEnd: "2026-10-01 00:00:00",
		},
		{
			name: "quarterly Q1 lower edge", cadence: Quarterly,
			when: "2026-01-01 00:00:00", wantKey: "2026-Q1",
			wantStart: "2026-01-01 00:00:00", wantEnd: "2026-04-01 00:00:00",
		},
		{
			name: "quarterly Q4 upper edge", cadence: Quarterly,
			when: "2026-12-31 23:59:59", wantKey: "2026-Q4",
			wantStart: "2026-10-01 00:00:00", wantEnd: "2027-01-01 00:00:00",
		},
		{
			name: "annual", cadence: Annual,
			when: "2026-09-03 14:22:07", wantKey: "2026-Y",
			wantStart: "2026-01-01 00:00:00", wantEnd: "2027-01-01 00:00:00",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cal.At(tc.cadence, at(t, cal, wall, tc.when))
			if err != nil {
				t.Fatalf("At: %v", err)
			}
			if got.Key != tc.wantKey {
				t.Errorf("key = %q, want %q", got.Key, tc.wantKey)
			}
			if want := at(t, cal, wall, tc.wantStart); !got.Start.Equal(want) {
				t.Errorf("start = %s, want %s", got.Start, want)
			}
			if want := at(t, cal, wall, tc.wantEnd); !got.End.Equal(want) {
				t.Errorf("end = %s, want %s", got.End, want)
			}
		})
	}
}

// The flip is implicit: crossing a boundary changes the key, and nothing is
// mutated. These assertions pin that behaviour down.
func TestPeriodRollsOverAtBoundary(t *testing.T) {
	cal := testCalendar(t)

	// One nanosecond either side of Monday 00:00.
	sundayNight := at(t, cal, wall, "2026-09-07 00:00:00").Add(-time.Nanosecond)
	mondayStart := at(t, cal, wall, "2026-09-07 00:00:00")

	for _, cadence := range []Cadence{Daily, Weekly} {
		t.Run(string(cadence), func(t *testing.T) {
			before := cal.MustAt(cadence, sundayNight)
			after := cal.MustAt(cadence, mondayStart)

			if before.Key == after.Key {
				t.Fatalf("expected the %s key to change at the boundary, both were %q", cadence, before.Key)
			}
			if next := cal.Next(before); next.Key != after.Key {
				t.Errorf("Next(%s) = %s, want %s", before.Key, next.Key, after.Key)
			}
			if prev := cal.Previous(after); prev.Key != before.Key {
				t.Errorf("Previous(%s) = %s, want %s", after.Key, prev.Key, before.Key)
			}
			if !before.Contains(sundayNight) {
				t.Errorf("%s should contain the instant before its end", before)
			}
			if before.Contains(mondayStart) {
				t.Errorf("%s must not contain its own end (End is exclusive)", before)
			}
			if !before.IsClosed(mondayStart) {
				t.Errorf("%s should be closed once the next period starts", before)
			}
			if after.IsClosed(mondayStart) {
				t.Errorf("%s should still be open at its first instant", after)
			}
		})
	}
}

// Monthly and quarterly cards must not flip early just because a shorter cadence
// rolled over.
func TestLongerCadencesDoNotFlipDaily(t *testing.T) {
	cal := testCalendar(t)

	day1 := at(t, cal, wall, "2026-09-03 09:00:00")
	day2 := at(t, cal, wall, "2026-09-04 09:00:00")

	for _, cadence := range []Cadence{Monthly, Quarterly, Annual} {
		a := cal.MustAt(cadence, day1)
		b := cal.MustAt(cadence, day2)
		if a.Key != b.Key {
			t.Errorf("%s key changed between consecutive days: %q then %q", cadence, a.Key, b.Key)
		}
	}
}

// ---------------------------------------------------------------------------
// Daylight saving
// ---------------------------------------------------------------------------

// In 2026 US Eastern springs forward on March 8 and falls back on November 1.
// A "day" is therefore not always 24 hours, and boundaries must still land on
// local midnight.
func TestDaylightSavingTransitions(t *testing.T) {
	cal := testCalendar(t)

	t.Run("spring forward day is 23 hours", func(t *testing.T) {
		p := cal.MustAt(Daily, at(t, cal, wall, "2026-03-08 12:00:00"))
		if p.Key != "2026-03-08" {
			t.Fatalf("key = %q, want 2026-03-08", p.Key)
		}
		if got := p.End.Sub(p.Start); got != 23*time.Hour {
			t.Errorf("duration = %v, want 23h", got)
		}
		assertLocalMidnight(t, p)
	})

	t.Run("fall back day is 25 hours", func(t *testing.T) {
		p := cal.MustAt(Daily, at(t, cal, wall, "2026-11-01 12:00:00"))
		if p.Key != "2026-11-01" {
			t.Fatalf("key = %q, want 2026-11-01", p.Key)
		}
		if got := p.End.Sub(p.Start); got != 25*time.Hour {
			t.Errorf("duration = %v, want 25h", got)
		}
		assertLocalMidnight(t, p)
	})

	t.Run("week containing spring forward is 167 hours", func(t *testing.T) {
		// 2026-03-08 is a Sunday, so it closes the week that opened on Monday
		// 2026-03-02 (ISO week 10). That week loses an hour to the transition.
		p := cal.MustAt(Weekly, at(t, cal, wall, "2026-03-08 12:00:00"))
		if p.Key != "2026-W10" {
			t.Fatalf("key = %q, want 2026-W10", p.Key)
		}
		if want := at(t, cal, wall, "2026-03-02 00:00:00"); !p.Start.Equal(want) {
			t.Errorf("start = %s, want %s", p.Start, want)
		}
		if want := at(t, cal, wall, "2026-03-09 00:00:00"); !p.End.Equal(want) {
			t.Errorf("end = %s, want %s", p.End, want)
		}
		if got := p.End.Sub(p.Start); got != 167*time.Hour {
			t.Errorf("duration = %v, want 167h (a week that loses an hour)", got)
		}
		assertLocalMidnight(t, p)
	})

	t.Run("month containing fall back still spans Nov 1 to Dec 1", func(t *testing.T) {
		p := cal.MustAt(Monthly, at(t, cal, wall, "2026-11-15 12:00:00"))
		if p.Key != "2026-M11" {
			t.Fatalf("key = %q, want 2026-M11", p.Key)
		}
		assertLocalMidnight(t, p)
	})
}

// assertLocalMidnight guards the invariant that every boundary is exactly
// midnight local time, which is what stops DST arithmetic from drifting.
func assertLocalMidnight(t *testing.T, p Period) {
	t.Helper()
	for label, ts := range map[string]time.Time{"start": p.Start, "end": p.End} {
		h, m, s := ts.Clock()
		if h != 0 || m != 0 || s != 0 || ts.Nanosecond() != 0 {
			t.Errorf("%s %s is not local midnight: %s", p, label, ts)
		}
	}
}

// ---------------------------------------------------------------------------
// ISO week / year edge cases
// ---------------------------------------------------------------------------

// The ISO week-numbering year does not always match the calendar year. A weekly
// card spanning New Year must stay on one key for the whole week.
func TestISOWeekYearBoundaries(t *testing.T) {
	cal := testCalendar(t)

	cases := []struct {
		when      string
		wantKey   string
		wantStart string
	}{
		// 2026-01-01 is a Thursday, so it falls in ISO week 1 of 2026, which
		// begins back in December 2025.
		{"2026-01-01 12:00:00", "2026-W01", "2025-12-29 00:00:00"},
		{"2025-12-29 00:00:00", "2026-W01", "2025-12-29 00:00:00"},
		{"2025-12-28 23:59:59", "2025-W52", "2025-12-22 00:00:00"},
		// 2027-01-01 is a Friday and belongs to the 53rd week of 2026.
		{"2027-01-01 12:00:00", "2026-W53", "2026-12-28 00:00:00"},
		{"2027-01-04 00:00:00", "2027-W01", "2027-01-04 00:00:00"},
	}

	for _, tc := range cases {
		t.Run(tc.when, func(t *testing.T) {
			p := cal.MustAt(Weekly, at(t, cal, wall, tc.when))
			if p.Key != tc.wantKey {
				t.Errorf("key = %q, want %q", p.Key, tc.wantKey)
			}
			if want := at(t, cal, wall, tc.wantStart); !p.Start.Equal(want) {
				t.Errorf("start = %s, want %s", p.Start, want)
			}
			if p.Start.Weekday() != time.Monday {
				t.Errorf("weekly period must start on a Monday, got %s", p.Start.Weekday())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Key parsing
// ---------------------------------------------------------------------------

func TestKeyRoundTrip(t *testing.T) {
	cal := testCalendar(t)
	start := at(t, cal, wall, "2024-02-27 06:30:00")

	for _, cadence := range Cadences() {
		t.Run(string(cadence), func(t *testing.T) {
			// Walk a couple of years of consecutive periods and make sure every
			// key parses back to exactly the period that produced it.
			p := cal.MustAt(cadence, start)
			steps := map[Cadence]int{Daily: 800, Weekly: 120, Monthly: 30, Quarterly: 12, Annual: 6}[cadence]

			for i := 0; i < steps; i++ {
				back, err := cal.FromKey(p.Key)
				if err != nil {
					t.Fatalf("FromKey(%q): %v", p.Key, err)
				}
				if back.Cadence != p.Cadence || back.Key != p.Key ||
					!back.Start.Equal(p.Start) || !back.End.Equal(p.End) {
					t.Fatalf("round trip mismatch for %q:\n got %+v\nwant %+v", p.Key, back, p)
				}
				if inferred, err := CadenceFromKey(p.Key); err != nil || inferred != cadence {
					t.Fatalf("CadenceFromKey(%q) = %q, %v; want %q", p.Key, inferred, err, cadence)
				}
				// Consecutive periods must abut exactly, leaving no gap or overlap.
				next := cal.Next(p)
				if !next.Start.Equal(p.End) {
					t.Fatalf("gap between %s (ends %s) and %s (starts %s)", p.Key, p.End, next.Key, next.Start)
				}
				p = next
			}
		})
	}
}

func TestFromKeyRejectsInvalidKeys(t *testing.T) {
	cal := testCalendar(t)

	bad := []string{
		"",
		"nonsense",
		"2026-02-31",  // not a real date
		"2026-13-01",  // month 13
		"2026-00-10",  // month 0
		"2025-W53",    // 2025 only has 52 ISO weeks
		"2026-W00",    // weeks are 1-based
		"2026-W54",    // out of range
		"2026-M13",    // month 13
		"2026-M00",    // month 0
		"2026-Q0",     // quarters are 1-4
		"2026-Q5",     // out of range
		"26-09-03",    // short year
		"2026-9-3",    // unpadded
		"2026-Y1",     // trailing junk
		"2026-09-03 ", // stray whitespace
		" 2026-09-03", // stray whitespace
	}

	for _, key := range bad {
		if _, err := cal.FromKey(key); err == nil {
			t.Errorf("FromKey(%q) succeeded, want an error", key)
		}
	}

	// 2026 genuinely has 53 ISO weeks, so this one must be accepted.
	if _, err := cal.FromKey("2026-W53"); err != nil {
		t.Errorf("FromKey(2026-W53) = %v, want success", err)
	}
}

func TestValidateKeyChecksCadenceMatches(t *testing.T) {
	cal := testCalendar(t)

	if err := cal.ValidateKey(Daily, "2026-09-03"); err != nil {
		t.Errorf("matching cadence rejected: %v", err)
	}
	// A well-formed key for the wrong cadence must be refused, otherwise a
	// client could file a weekly completion against a daily card.
	if err := cal.ValidateKey(Daily, "2026-W36"); err == nil {
		t.Error("expected a daily card to reject a weekly period key")
	}
	if err := cal.ValidateKey(Weekly, "2026-09-03"); err == nil {
		t.Error("expected a weekly card to reject a daily period key")
	}
}

// ---------------------------------------------------------------------------
// Ranges
// ---------------------------------------------------------------------------

func TestBetween(t *testing.T) {
	cal := testCalendar(t)

	t.Run("daily across a month boundary", func(t *testing.T) {
		got, err := cal.Between(Daily,
			at(t, cal, wall, "2026-08-30 09:00:00"),
			at(t, cal, wall, "2026-09-02 09:00:00"))
		if err != nil {
			t.Fatalf("Between: %v", err)
		}
		want := []string{"2026-08-30", "2026-08-31", "2026-09-01", "2026-09-02"}
		assertKeys(t, got, want)
	})

	t.Run("includes the period containing from", func(t *testing.T) {
		// "from" lands mid-week, so the partial week it belongs to still counts:
		// the card was expected to be done that week.
		got, err := cal.Between(Weekly,
			at(t, cal, wall, "2026-09-03 09:00:00"),
			at(t, cal, wall, "2026-09-21 09:00:00"))
		if err != nil {
			t.Fatalf("Between: %v", err)
		}
		assertKeys(t, got, []string{"2026-W36", "2026-W37", "2026-W38", "2026-W39"})
	})

	t.Run("empty when the range is inverted or empty", func(t *testing.T) {
		from := at(t, cal, wall, "2026-09-03 09:00:00")
		for _, to := range []time.Time{from, from.Add(-time.Hour)} {
			got, err := cal.Between(Daily, from, to)
			if err != nil {
				t.Fatalf("Between: %v", err)
			}
			if len(got) != 0 {
				t.Errorf("expected no periods, got %d", len(got))
			}
		}
	})

	t.Run("rejects an absurdly wide range", func(t *testing.T) {
		from := at(t, cal, wall, "1900-01-01 00:00:00")
		to := at(t, cal, wall, "2100-01-01 00:00:00")
		if _, err := cal.Between(Daily, from, to); err == nil {
			t.Error("expected an error for a range covering ~73000 daily periods")
		}
	})

	t.Run("counts every quarter in a year", func(t *testing.T) {
		got, err := cal.Between(Quarterly,
			at(t, cal, wall, "2026-01-01 00:00:00"),
			at(t, cal, wall, "2027-01-01 00:00:00"))
		if err != nil {
			t.Fatalf("Between: %v", err)
		}
		assertKeys(t, got, []string{"2026-Q1", "2026-Q2", "2026-Q3", "2026-Q4"})
	})
}

func TestClosedBefore(t *testing.T) {
	cal := testCalendar(t)
	now := at(t, cal, wall, "2026-09-03 14:00:00")

	t.Run("returns most recent closed periods first", func(t *testing.T) {
		got, err := cal.ClosedBefore(Daily, now, 3)
		if err != nil {
			t.Fatalf("ClosedBefore: %v", err)
		}
		assertKeys(t, got, []string{"2026-09-02", "2026-09-01", "2026-08-31"})
	})

	t.Run("never includes the period still in progress", func(t *testing.T) {
		got, err := cal.ClosedBefore(Daily, now, 5)
		if err != nil {
			t.Fatalf("ClosedBefore: %v", err)
		}
		for _, p := range got {
			if p.Key == "2026-09-03" {
				t.Error("the in-progress period must not be reported as closed")
			}
			if !p.IsClosed(now) {
				t.Errorf("%s is not actually closed as of %s", p, now)
			}
		}
	})

	t.Run("zero or negative limit yields nothing", func(t *testing.T) {
		for _, limit := range []int{0, -1} {
			got, err := cal.ClosedBefore(Weekly, now, limit)
			if err != nil {
				t.Fatalf("ClosedBefore: %v", err)
			}
			if len(got) != 0 {
				t.Errorf("limit %d returned %d periods", limit, len(got))
			}
		}
	})

	t.Run("works for every cadence", func(t *testing.T) {
		for _, cadence := range Cadences() {
			got, err := cal.ClosedBefore(cadence, now, 2)
			if err != nil {
				t.Fatalf("ClosedBefore(%s): %v", cadence, err)
			}
			if len(got) != 2 {
				t.Fatalf("ClosedBefore(%s) returned %d periods, want 2", cadence, len(got))
			}
			if !got[0].End.After(got[1].End) {
				t.Errorf("%s results are not newest-first: %s then %s", cadence, got[0].Key, got[1].Key)
			}
		}
	})
}

func assertKeys(t *testing.T, got []Period, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d periods %v, want %d %v", len(got), keysOf(got), len(want), want)
	}
	for i := range want {
		if got[i].Key != want[i] {
			t.Errorf("period %d = %q, want %q", i, got[i].Key, want[i])
		}
	}
}

func keysOf(ps []Period) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Key
	}
	return out
}

// ---------------------------------------------------------------------------
// Misc
// ---------------------------------------------------------------------------

func TestAtRejectsUnknownCadence(t *testing.T) {
	cal := testCalendar(t)
	if _, err := cal.At(Cadence("fortnightly"), time.Now()); err == nil {
		t.Error("expected an error for an unsupported cadence")
	}
}

func TestLoadCalendar(t *testing.T) {
	t.Run("empty name falls back to the default timezone", func(t *testing.T) {
		cal, err := LoadCalendar("")
		if err != nil {
			t.Fatalf("LoadCalendar: %v", err)
		}
		if cal.Location().String() != DefaultTimezone {
			t.Errorf("location = %s, want %s", cal.Location(), DefaultTimezone)
		}
	})

	t.Run("unknown timezone is an error", func(t *testing.T) {
		if _, err := LoadCalendar("Mars/Olympus_Mons"); err == nil {
			t.Error("expected an error for an unknown timezone")
		}
	})

	t.Run("nil location degrades to UTC", func(t *testing.T) {
		if got := NewCalendar(nil).Location(); got != time.UTC {
			t.Errorf("location = %v, want UTC", got)
		}
	})
}

func TestParseCadence(t *testing.T) {
	good := map[string]Cadence{
		"daily": Daily, "Daily": Daily, " DAILY ": Daily, "d": Daily,
		"weekly": Weekly, "week": Weekly,
		"monthly": Monthly, "m": Monthly,
		"quarterly": Quarterly, "quarter": Quarterly, "q": Quarterly,
		"annual": Annual, "annually": Annual, "yearly": Annual, "y": Annual,
	}
	for in, want := range good {
		got, err := ParseCadence(in)
		if err != nil {
			t.Errorf("ParseCadence(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseCadence(%q) = %q, want %q", in, got, want)
		}
	}

	for _, in := range []string{"", "fortnightly", "biweekly", "hourly"} {
		if _, err := ParseCadence(in); err == nil {
			t.Errorf("ParseCadence(%q) succeeded, want an error", in)
		}
	}
}

func TestSortCadences(t *testing.T) {
	in := []Cadence{Annual, Daily, Quarterly, Weekly, Monthly}
	SortCadences(in)
	want := Cadences()
	for i := range want {
		if in[i] != want[i] {
			t.Fatalf("sorted = %v, want %v", in, want)
		}
	}
}

func TestZeroPeriod(t *testing.T) {
	cal := testCalendar(t)
	var p Period
	if !p.Zero() {
		t.Error("the empty Period should report Zero")
	}
	if got := cal.Shift(p, 1); !got.Zero() {
		t.Error("shifting the empty Period should stay empty")
	}
	if p.String() != "<no period>" {
		t.Errorf("String() = %q", p.String())
	}
}
