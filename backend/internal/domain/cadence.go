// Package domain holds the pure scheduling logic for the kamishibai board.
//
// Nothing in this package touches the database or PocketBase. It is the
// authoritative answer to two questions:
//
//   - "which period is a card currently in?"  (see Calendar.Current)
//   - "has that period rolled over yet?"      (see Period.Contains / IsClosed)
//
// Keeping this dependency-free is deliberate: the flip behaviour is the part
// most likely to harbour off-by-one and daylight-saving bugs, so it needs to be
// cheap to unit test in isolation.
package domain

import (
	"fmt"
	"sort"
	"strings"
)

// Cadence is how often a triage card is expected to be repeated.
type Cadence string

const (
	Daily     Cadence = "daily"
	Weekly    Cadence = "weekly"
	Monthly   Cadence = "monthly"
	Quarterly Cadence = "quarterly"
	Annual    Cadence = "annual"
)

// Cadences lists every supported cadence, ordered from shortest to longest
// period. The order is stable and is relied on for display and for iterating
// during rollup generation.
func Cadences() []Cadence {
	return []Cadence{Daily, Weekly, Monthly, Quarterly, Annual}
}

// CadenceValues returns the cadences as plain strings, for building the
// select-field options in migrations.
func CadenceValues() []string {
	all := Cadences()
	out := make([]string, len(all))
	for i, c := range all {
		out[i] = string(c)
	}
	return out
}

// Valid reports whether c is a recognised cadence.
func (c Cadence) Valid() bool {
	switch c {
	case Daily, Weekly, Monthly, Quarterly, Annual:
		return true
	default:
		return false
	}
}

func (c Cadence) String() string { return string(c) }

// Label is a human-friendly name for the cadence.
func (c Cadence) Label() string {
	switch c {
	case Daily:
		return "Daily"
	case Weekly:
		return "Weekly"
	case Monthly:
		return "Monthly"
	case Quarterly:
		return "Quarterly"
	case Annual:
		return "Annual"
	default:
		return string(c)
	}
}

// ParseCadence converts a string into a Cadence, accepting a few friendly
// aliases so CLI flags and seed files can be forgiving.
func ParseCadence(s string) (Cadence, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "daily", "day", "d":
		return Daily, nil
	case "weekly", "week", "w":
		return Weekly, nil
	case "monthly", "month", "m":
		return Monthly, nil
	case "quarterly", "quarter", "q":
		return Quarterly, nil
	case "annual", "annually", "yearly", "year", "y", "a":
		return Annual, nil
	default:
		return "", fmt.Errorf("unknown cadence %q (expected one of %s)", s, strings.Join(CadenceValues(), ", "))
	}
}

// SortCadences orders cadences shortest-period-first, regardless of the order
// they were supplied in.
func SortCadences(in []Cadence) {
	rank := map[Cadence]int{}
	for i, c := range Cadences() {
		rank[c] = i
	}
	sort.SliceStable(in, func(i, j int) bool {
		return rank[in[i]] < rank[in[j]]
	})
}
