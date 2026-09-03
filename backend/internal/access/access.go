// Package access centralises the authorisation questions the application asks.
//
// The collection API rules in the migrations cover ordinary CRUD through
// PocketBase's own endpoints. This package is for the custom endpoints and
// hooks, where the same decisions have to be made in Go. Keeping them here means
// there is one definition of "is this person allowed to touch this team" rather
// than a membership check copy-pasted into every handler.
package access

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/dbowling/kamishibai/backend/internal/schema"
)

// IsAdmin reports whether the user holds the admin role.
//
// Anything other than exactly "admin" is treated as a regular user, so a
// missing, empty or unrecognised role fails closed.
func IsAdmin(user *core.Record) bool {
	return user != nil && user.GetString(schema.FieldRole) == schema.RoleAdmin
}

// IsMember reports whether user is listed on the given team record.
//
// Admins are deliberately not special-cased here: this answers the literal
// question "are they on the member list". Use CanUseTeam for the access
// decision.
func IsMember(team, user *core.Record) bool {
	if team == nil || user == nil {
		return false
	}
	return slices.Contains(team.GetStringSlice(schema.FieldMembers), user.Id)
}

// CanReadTeam reports whether user may read records belonging to team.
//
// Admins are implicitly on every team, which is why membership is never
// materialised for them. Archived teams stay readable so that history and
// reporting survive archival.
func CanReadTeam(team, user *core.Record) bool {
	return IsAdmin(user) || IsMember(team, user)
}

// CanWriteTeam reports whether user may create or modify records within team.
//
// Stricter than CanReadTeam: an archived team is frozen, so members can still
// look at it but cannot record new work against it.
func CanWriteTeam(team, user *core.Record) bool {
	return CanReadTeam(team, user) && !IsArchived(team)
}

// IsArchived reports whether a record has been soft-deleted.
func IsArchived(record *core.Record) bool {
	if record == nil {
		return false
	}
	return !record.GetDateTime(schema.FieldArchivedAt).IsZero()
}

// LoadTeam fetches the team a record belongs to via its `team` relation.
func LoadTeam(app core.App, record *core.Record) (*core.Record, error) {
	teamID := record.GetString(schema.FieldTeam)
	if teamID == "" {
		return nil, fmt.Errorf("record %q has no team", record.Id)
	}

	team, err := app.FindRecordById(schema.Teams, teamID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("team %q not found", teamID)
		}
		return nil, fmt.Errorf("load team %q: %w", teamID, err)
	}
	return team, nil
}

// VisibleTeamIDs returns the ids of every team the user may read.
//
// Admins get every team, including archived ones, because they are the ones who
// restore things. Regular users get the teams they are listed on.
func VisibleTeamIDs(app core.App, user *core.Record) ([]string, error) {
	if user == nil {
		return nil, nil
	}

	var teams []*core.Record
	var err error

	if IsAdmin(user) {
		teams, err = app.FindAllRecords(schema.Teams)
	} else {
		// members is a multi-relation, so the path must be traversed to the
		// related record's id. `members ?= {:user}` parses but matches nothing.
		teams, err = app.FindRecordsByFilter(
			schema.Teams,
			"members.id ?= {:user}",
			schema.FieldName,
			0, 0,
			dbx.Params{"user": user.Id},
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list visible teams: %w", err)
	}

	ids := make([]string, 0, len(teams))
	for _, t := range teams {
		ids = append(ids, t.Id)
	}
	return ids, nil
}
