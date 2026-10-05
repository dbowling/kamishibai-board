// Package teamcal answers "which calendar does this team's data live on".
//
// Each team evaluates its periods in its own timezone, falling back to the
// instance default when it has none. The occurrence, rollup and API packages all
// need that answer, and each reading the team's zone off the record and handling
// a bad value for itself would be three chances to disagree, so the lookup lives
// here once. domain stays free of database types; this is the thin layer that
// joins it to a team record.
package teamcal

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"github.com/dbowling/kamishibai/backend/internal/domain"
	"github.com/dbowling/kamishibai/backend/internal/schema"
)

// Timezone returns the IANA name a team evaluates its periods in: its own, or the
// instance default's when it has not set one.
func Timezone(cals *domain.Calendars, team *core.Record) string {
	if zone := team.GetString(schema.FieldTimezone); zone != "" {
		return zone
	}
	return cals.Default().Location().String()
}

// For returns the calendar for a team record.
//
// A stored zone that does not load is an error, never a fall back to the default
// or to UTC. The hooks validate zones on the way in, so this only happens through
// a bug or a hand edit of the database, and quietly using another zone would move
// that team's boundaries without anyone noticing.
func For(cals *domain.Calendars, team *core.Record) (*domain.Calendar, error) {
	cal, err := cals.For(team.GetString(schema.FieldTimezone))
	if err != nil {
		return nil, fmt.Errorf("team %q: %w", team.Id, err)
	}
	return cal, nil
}

// ForBoard returns the calendar for the team that owns a board.
func ForBoard(app core.App, cals *domain.Calendars, board *core.Record) (*domain.Calendar, error) {
	team, err := app.FindRecordById(schema.Teams, board.GetString(schema.FieldTeam))
	if err != nil {
		return nil, fmt.Errorf("load team for board %q: %w", board.Id, err)
	}
	return For(cals, team)
}
