package domain

import (
	"fmt"
	"strings"
)

// Status is the state of a single card within a single period.
type Status string

const (
	// StatusNotStarted is never written to the database.
	//
	// This is the core of the capacity design: a card is "not started" for a
	// period precisely because no occurrence row exists for that period yet. An
	// untouched daily card costs zero rows per day instead of one, and the
	// board "flips" at a period boundary without any write at all.
	//
	// It exists as a constant only so that API responses and the UI have a name
	// for the state they are rendering.
	StatusNotStarted Status = "not_started"

	// StatusInProgress means somebody has picked the card up this period.
	StatusInProgress Status = "in_progress"

	// StatusDone means the work was completed this period.
	StatusDone Status = "done"
)

// PersistedStatuses lists the statuses that can actually appear in the
// occurrences table. StatusNotStarted is deliberately absent.
func PersistedStatuses() []Status {
	return []Status{StatusInProgress, StatusDone}
}

// PersistedStatusValues returns PersistedStatuses as strings, for building the
// select-field options in migrations.
func PersistedStatusValues() []string {
	all := PersistedStatuses()
	out := make([]string, len(all))
	for i, s := range all {
		out[i] = string(s)
	}
	return out
}

// Persisted reports whether the status is one that gets stored.
func (s Status) Persisted() bool {
	return s == StatusInProgress || s == StatusDone
}

// Valid reports whether s is a status the application understands, including
// the synthetic not-started state.
func (s Status) Valid() bool {
	return s == StatusNotStarted || s.Persisted()
}

func (s Status) String() string { return string(s) }

// Label is a human-friendly name for the status.
func (s Status) Label() string {
	switch s {
	case StatusNotStarted:
		return "Not started"
	case StatusInProgress:
		return "In progress"
	case StatusDone:
		return "Done"
	default:
		return string(s)
	}
}

// ParseStatus converts a string into a Status.
func ParseStatus(s string) (Status, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "not_started", "not-started", "notstarted", "":
		return StatusNotStarted, nil
	case "in_progress", "in-progress", "inprogress", "started":
		return StatusInProgress, nil
	case "done", "complete", "completed":
		return StatusDone, nil
	default:
		return "", fmt.Errorf("unknown status %q", s)
	}
}
