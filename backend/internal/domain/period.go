package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// DefaultTimezone is the timezone used to evaluate period boundaries when none
// is configured. The whole team shares a single timezone by design: a card that
// flips "on Monday" must flip at the same moment for everybody looking at the
// board.
const DefaultTimezone = "America/New_York"

// Period is a single occurrence window for a cadence, for example "the week of
// 2026-W36" or "the day 2026-09-03".
//
// Start is inclusive, End is exclusive. Both are expressed in the Calendar's
// location so that formatting them is unsurprising.
type Period struct {
	Cadence Cadence
	Key     string
	Start   time.Time
	End     time.Time
}

// Contains reports whether t falls inside the period.
func (p Period) Contains(t time.Time) bool {
	return !t.Before(p.Start) && t.Before(p.End)
}

// IsClosed reports whether the period has fully elapsed as of now. A closed
// period is safe to snapshot into the rollup table because no further work can
// be recorded against it.
func (p Period) IsClosed(now time.Time) bool {
	return !now.Before(p.End)
}

// Zero reports whether the period is the empty value.
func (p Period) Zero() bool { return p.Key == "" }

func (p Period) String() string {
	if p.Zero() {
		return "<no period>"
	}
	return string(p.Cadence) + ":" + p.Key
}

// Calendar resolves cadences into concrete periods within a fixed timezone.
//
// It is immutable and safe for concurrent use.
type Calendar struct {
	loc *time.Location
}

// NewCalendar returns a Calendar bound to loc. A nil loc falls back to UTC
// rather than panicking, so a misconfigured calendar degrades predictably.
func NewCalendar(loc *time.Location) *Calendar {
	if loc == nil {
		loc = time.UTC
	}
	return &Calendar{loc: loc}
}

// LoadCalendar builds a Calendar from an IANA timezone name such as
// "America/New_York". An empty name resolves to DefaultTimezone.
//
// The binary embeds the timezone database (see the time/tzdata import in
// main.go), so this works in a scratch container with no system tzdata.
func LoadCalendar(name string) (*Calendar, error) {
	if name == "" {
		name = DefaultTimezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", name, err)
	}
	return NewCalendar(loc), nil
}

// Location returns the timezone the calendar evaluates boundaries in.
func (cal *Calendar) Location() *time.Location { return cal.loc }

// Now returns the current time in the calendar's location.
func (cal *Calendar) Now() time.Time { return time.Now().In(cal.loc) }

// At returns the period of the given cadence that contains t.
//
// This is the whole flip mechanism: a card has no stored status, so "what state
// is this card in right now" is answered by looking up the occurrence row for
// At(cadence, now).Key. When the clock crosses a boundary the key changes, no
// occurrence row exists for the new key, and the card reads as not-started.
// Nothing has to be mutated for the board to flip.
func (cal *Calendar) At(cadence Cadence, t time.Time) (Period, error) {
	if !cadence.Valid() {
		return Period{}, fmt.Errorf("invalid cadence %q", cadence)
	}
	start := cal.startOf(cadence, t)
	return cal.periodFromStart(cadence, start), nil
}

// Current is At(cadence, time.Now()).
func (cal *Calendar) Current(cadence Cadence) (Period, error) {
	return cal.At(cadence, time.Now())
}

// MustAt is At but panics on an invalid cadence. Intended for tests and for
// call sites where the cadence is a compile-time constant.
func (cal *Calendar) MustAt(cadence Cadence, t time.Time) Period {
	p, err := cal.At(cadence, t)
	if err != nil {
		panic(err)
	}
	return p
}

// Shift returns the period n windows away from p. Negative n moves backwards.
func (cal *Calendar) Shift(p Period, n int) Period {
	if p.Zero() {
		return Period{}
	}
	start := cal.advance(p.Cadence, p.Start, n)
	return cal.periodFromStart(p.Cadence, start)
}

// Next returns the period immediately after p.
func (cal *Calendar) Next(p Period) Period { return cal.Shift(p, 1) }

// Previous returns the period immediately before p.
func (cal *Calendar) Previous(p Period) Period { return cal.Shift(p, -1) }

// Between returns every period of the given cadence that overlaps the
// half-open interval [from, to). The result is ordered oldest first.
//
// It is used to work out how many times a card was *expected* to be done over a
// reporting window, which is the denominator of its completion rate.
func (cal *Calendar) Between(cadence Cadence, from, to time.Time) ([]Period, error) {
	if !cadence.Valid() {
		return nil, fmt.Errorf("invalid cadence %q", cadence)
	}
	if !to.After(from) {
		return nil, nil
	}

	var out []Period
	cur, err := cal.At(cadence, from)
	if err != nil {
		return nil, err
	}
	for cur.Start.Before(to) {
		out = append(out, cur)
		cur = cal.Next(cur)

		// Defensive stop: a mis-specified interval (for example spanning
		// centuries of daily periods) should fail loudly rather than allocate
		// without bound.
		if len(out) > maxPeriodsPerQuery {
			return nil, fmt.Errorf("interval covers more than %d %s periods; narrow the range", maxPeriodsPerQuery, cadence)
		}
	}
	return out, nil
}

// maxPeriodsPerQuery caps Between so a bad reporting range cannot exhaust
// memory. ~27 years of daily periods is far beyond any real query.
const maxPeriodsPerQuery = 10000

// ClosedBefore returns up to limit periods of the given cadence that have fully
// elapsed as of now, ordered most recent first.
//
// The rollup job walks this list to find periods that still need a snapshot,
// which lets it self-heal after downtime instead of silently missing a window.
func (cal *Calendar) ClosedBefore(cadence Cadence, now time.Time, limit int) ([]Period, error) {
	if limit <= 0 {
		return nil, nil
	}
	cur, err := cal.At(cadence, now)
	if err != nil {
		return nil, err
	}

	out := make([]Period, 0, limit)
	p := cal.Previous(cur) // the current period is by definition still open
	for len(out) < limit {
		if !p.IsClosed(now) {
			break
		}
		out = append(out, p)
		p = cal.Previous(p)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Period keys
// ---------------------------------------------------------------------------
//
// Keys are canonical, human-readable, and sort lexicographically in
// chronological order within a cadence, which makes them cheap to index, group
// by, and eyeball in the database:
//
//	daily      2026-09-03
//	weekly     2026-W36     (ISO-8601 week, Monday-based)
//	monthly    2026-M09
//	quarterly  2026-Q3
//	annual     2026-Y
//
// The formats are mutually exclusive, so a key alone identifies its cadence.

var (
	dailyKeyRe     = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
	weeklyKeyRe    = regexp.MustCompile(`^(\d{4})-W(\d{2})$`)
	monthlyKeyRe   = regexp.MustCompile(`^(\d{4})-M(\d{2})$`)
	quarterlyKeyRe = regexp.MustCompile(`^(\d{4})-Q([1-4])$`)
	annualKeyRe    = regexp.MustCompile(`^(\d{4})-Y$`)
)

// CadenceFromKey infers the cadence a period key belongs to.
func CadenceFromKey(key string) (Cadence, error) {
	switch {
	case dailyKeyRe.MatchString(key):
		return Daily, nil
	case weeklyKeyRe.MatchString(key):
		return Weekly, nil
	case monthlyKeyRe.MatchString(key):
		return Monthly, nil
	case quarterlyKeyRe.MatchString(key):
		return Quarterly, nil
	case annualKeyRe.MatchString(key):
		return Annual, nil
	default:
		return "", fmt.Errorf("unrecognised period key %q", key)
	}
}

// FromKey rebuilds a full Period (including its boundaries) from a key.
//
// It round-trips exactly with the keys produced by At, and rejects keys that
// look well-formed but describe an impossible period such as 2026-W53.
func (cal *Calendar) FromKey(key string) (Period, error) {
	cadence, err := CadenceFromKey(key)
	if err != nil {
		return Period{}, err
	}

	var start time.Time
	switch cadence {
	case Daily:
		m := dailyKeyRe.FindStringSubmatch(key)
		y, mo, d := atoi(m[1]), atoi(m[2]), atoi(m[3])
		start = time.Date(y, time.Month(mo), d, 0, 0, 0, 0, cal.loc)
		// Reject 2026-02-31 and friends, which time.Date would silently roll over.
		if start.Year() != y || int(start.Month()) != mo || start.Day() != d {
			return Period{}, fmt.Errorf("period key %q is not a real date", key)
		}

	case Weekly:
		m := weeklyKeyRe.FindStringSubmatch(key)
		isoYear, isoWeek := atoi(m[1]), atoi(m[2])
		if isoWeek < 1 || isoWeek > 53 {
			return Period{}, fmt.Errorf("period key %q has an out-of-range week", key)
		}
		start = cal.isoWeekStart(isoYear, isoWeek)
		// A 52-week ISO year has no week 53; verify the round trip.
		gotYear, gotWeek := start.ISOWeek()
		if gotYear != isoYear || gotWeek != isoWeek {
			return Period{}, fmt.Errorf("period key %q does not exist in the ISO calendar", key)
		}

	case Monthly:
		m := monthlyKeyRe.FindStringSubmatch(key)
		y, mo := atoi(m[1]), atoi(m[2])
		if mo < 1 || mo > 12 {
			return Period{}, fmt.Errorf("period key %q has an out-of-range month", key)
		}
		start = time.Date(y, time.Month(mo), 1, 0, 0, 0, 0, cal.loc)

	case Quarterly:
		m := quarterlyKeyRe.FindStringSubmatch(key)
		y, q := atoi(m[1]), atoi(m[2])
		start = time.Date(y, time.Month((q-1)*3+1), 1, 0, 0, 0, 0, cal.loc)

	case Annual:
		m := annualKeyRe.FindStringSubmatch(key)
		start = time.Date(atoi(m[1]), time.January, 1, 0, 0, 0, 0, cal.loc)
	}

	return cal.periodFromStart(cadence, start), nil
}

// ValidateKey checks that key is a well-formed period key for the given
// cadence. Used to reject client-supplied occurrence keys.
func (cal *Calendar) ValidateKey(cadence Cadence, key string) error {
	p, err := cal.FromKey(key)
	if err != nil {
		return err
	}
	if p.Cadence != cadence {
		return fmt.Errorf("period key %q is a %s key, expected %s", key, p.Cadence, cadence)
	}
	return nil
}

// ---------------------------------------------------------------------------
// internals
// ---------------------------------------------------------------------------

// periodFromStart completes a Period given its (already normalised) start.
func (cal *Calendar) periodFromStart(cadence Cadence, start time.Time) Period {
	return Period{
		Cadence: cadence,
		Key:     cal.keyFor(cadence, start),
		Start:   start,
		End:     cal.advance(cadence, start, 1),
	}
}

// startOf normalises t down to the first instant of its period.
func (cal *Calendar) startOf(cadence Cadence, t time.Time) time.Time {
	lt := t.In(cal.loc)
	y, mo, d := lt.Date()

	switch cadence {
	case Daily:
		return time.Date(y, mo, d, 0, 0, 0, 0, cal.loc)
	case Weekly:
		// The work week starts Monday, matching ISO-8601.
		return time.Date(y, mo, d-mondayOffset(lt.Weekday()), 0, 0, 0, 0, cal.loc)
	case Monthly:
		return time.Date(y, mo, 1, 0, 0, 0, 0, cal.loc)
	case Quarterly:
		return time.Date(y, time.Month(((int(mo)-1)/3)*3+1), 1, 0, 0, 0, 0, cal.loc)
	case Annual:
		return time.Date(y, time.January, 1, 0, 0, 0, 0, cal.loc)
	default:
		return time.Time{}
	}
}

// advance moves a period start n windows forward (or backward when negative).
//
// Every case rebuilds the date through time.Date rather than adding a fixed
// duration. That is what keeps boundaries correct across daylight-saving
// transitions and uneven month lengths: "one day later" means the next
// midnight, not exactly 24 hours later.
func (cal *Calendar) advance(cadence Cadence, start time.Time, n int) time.Time {
	y, mo, d := start.In(cal.loc).Date()

	switch cadence {
	case Daily:
		return time.Date(y, mo, d+n, 0, 0, 0, 0, cal.loc)
	case Weekly:
		return time.Date(y, mo, d+7*n, 0, 0, 0, 0, cal.loc)
	case Monthly:
		return time.Date(y, mo+time.Month(n), 1, 0, 0, 0, 0, cal.loc)
	case Quarterly:
		return time.Date(y, mo+time.Month(3*n), 1, 0, 0, 0, 0, cal.loc)
	case Annual:
		return time.Date(y+n, time.January, 1, 0, 0, 0, 0, cal.loc)
	default:
		return time.Time{}
	}
}

func (cal *Calendar) keyFor(cadence Cadence, start time.Time) string {
	s := start.In(cal.loc)

	switch cadence {
	case Daily:
		return s.Format("2006-01-02")
	case Weekly:
		isoYear, isoWeek := s.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", isoYear, isoWeek)
	case Monthly:
		return fmt.Sprintf("%04d-M%02d", s.Year(), int(s.Month()))
	case Quarterly:
		return fmt.Sprintf("%04d-Q%d", s.Year(), (int(s.Month())-1)/3+1)
	case Annual:
		return fmt.Sprintf("%04d-Y", s.Year())
	default:
		return ""
	}
}

// isoWeekStart returns the Monday that begins the given ISO week.
//
// January 4th is by definition always in ISO week 1, which gives a fixed
// anchor to count from.
func (cal *Calendar) isoWeekStart(isoYear, isoWeek int) time.Time {
	jan4 := time.Date(isoYear, time.January, 4, 0, 0, 0, 0, cal.loc)
	week1Monday := time.Date(isoYear, time.January, 4-mondayOffset(jan4.Weekday()), 0, 0, 0, 0, cal.loc)
	y, mo, d := week1Monday.Date()
	return time.Date(y, mo, d+7*(isoWeek-1), 0, 0, 0, 0, cal.loc)
}

// mondayOffset maps a weekday to how many days have passed since Monday.
// Go numbers Sunday as 0, so Sunday is the *last* day of the work week here.
func mondayOffset(wd time.Weekday) int {
	return (int(wd) + 6) % 7
}

// atoi parses digits already validated by a regexp, so errors are impossible.
func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
