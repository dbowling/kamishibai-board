package domain

import (
	"fmt"
	"sync"
	"time"
)

// ValidTimezone reports whether name is an IANA timezone this binary can load.
//
// It is stricter than time.LoadLocation in two ways, both about names that load
// successfully but mean something other than what the person typed:
//
//   - "" is rejected. LoadLocation treats it as UTC, but for a team or a user an
//     empty value means "inherit", which the caller has to decide before asking
//     whether a name is valid.
//   - "Local" is rejected. It resolves to whatever zone the server process runs
//     in, so the same value would mean different boundaries on a laptop and in a
//     container.
func ValidTimezone(name string) error {
	switch name {
	case "":
		return fmt.Errorf("timezone is empty")
	case "Local":
		return fmt.Errorf("timezone %q depends on the server's own zone; use an IANA name such as %q", name, DefaultTimezone)
	}
	if _, err := time.LoadLocation(name); err != nil {
		return fmt.Errorf("unknown timezone %q: use an IANA name such as %q", name, DefaultTimezone)
	}
	return nil
}

// Calendars resolves a timezone name to a Calendar, with a fallback for "none
// set".
//
// Each team evaluates its periods in its own zone, and most teams leave it
// empty to inherit the instance default. Loading a location parses tzdata, which
// is cheap but not free, and this sits on the path of every board render and
// card mutation, so each name is loaded once and the Calendar reused. Calendars
// are immutable, so sharing one between goroutines is safe.
//
// Like the rest of this package it has no database dependency: callers read the
// zone off the team record and hand the string in.
type Calendars struct {
	fallback *Calendar
	cache    sync.Map // timezone name -> *Calendar
}

// NewCalendars returns a resolver whose empty name resolves to defaultCal.
func NewCalendars(defaultCal *Calendar) *Calendars {
	return &Calendars{fallback: defaultCal}
}

// Default returns the calendar an empty timezone resolves to.
func (c *Calendars) Default() *Calendar { return c.fallback }

// For returns the calendar for an IANA timezone name, or the default calendar
// when name is empty.
//
// A name that does not load is an error rather than a quiet fall back to the
// default or to UTC: a stored zone can only be bad through a bug or a hand edit,
// and silently using another zone would shift that team's boundaries without
// anyone noticing. The same reasoning as a malformed KAMISHIBAI_TIMEZONE.
func (c *Calendars) For(name string) (*Calendar, error) {
	if name == "" {
		return c.fallback, nil
	}
	if cached, ok := c.cache.Load(name); ok {
		return cached.(*Calendar), nil
	}

	if err := ValidTimezone(name); err != nil {
		return nil, err
	}
	cal, err := LoadCalendar(name)
	if err != nil {
		return nil, err
	}

	// Two goroutines can race to load the same name. LoadOrStore makes sure they
	// both end up holding the same pointer.
	actual, _ := c.cache.LoadOrStore(name, cal)
	return actual.(*Calendar), nil
}
