package domain

import "testing"

func TestCalendarsEmptyNameFallsBackToDefault(t *testing.T) {
	def := testCalendar(t)
	cals := NewCalendars(def)

	got, err := cals.For("")
	if err != nil {
		t.Fatalf("For(\"\"): %v", err)
	}
	if got != def {
		t.Error("empty name did not resolve to the default calendar")
	}
}

func TestCalendarsCacheReturnsTheSameCalendar(t *testing.T) {
	cals := NewCalendars(testCalendar(t))

	first, err := cals.For("Asia/Tokyo")
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	second, err := cals.For("Asia/Tokyo")
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if first != second {
		t.Error("a second lookup built a new Calendar instead of reusing the cached one")
	}
	if got := first.Location().String(); got != "Asia/Tokyo" {
		t.Errorf("location = %q, want Asia/Tokyo", got)
	}
}

func TestCalendarsRejectInvalidNames(t *testing.T) {
	cals := NewCalendars(testCalendar(t))

	for _, name := range []string{"Mars/Base", "Local", "not a zone"} {
		if _, err := cals.For(name); err == nil {
			t.Errorf("For(%q) succeeded, want an error", name)
		}
	}
}

func TestValidTimezone(t *testing.T) {
	// "UTC" loads fine and is a legitimate team zone.
	for _, name := range []string{"UTC", "America/New_York", "Europe/London", "Asia/Tokyo"} {
		if err := ValidTimezone(name); err != nil {
			t.Errorf("ValidTimezone(%q) = %v, want nil", name, err)
		}
	}

	// "" and "Local" are accepted by time.LoadLocation but never mean what a
	// stored zone should mean.
	for _, name := range []string{"", "Local", "Mars/Base"} {
		if err := ValidTimezone(name); err == nil {
			t.Errorf("ValidTimezone(%q) = nil, want an error", name)
		}
	}
}
