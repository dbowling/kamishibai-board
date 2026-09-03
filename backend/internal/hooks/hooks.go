// Package hooks holds the record-level business rules.
//
// The collection API rules in the migrations answer "may this person touch this
// row at all". They cannot express the rules that depend on comparing the old
// and new value of a field, or on deriving one field from another. Those live
// here:
//
//   - a user cannot promote themselves to admin
//   - a card's team is always derived from its board, never taken from the client
//   - nothing moves between teams, which would orphan its history
//   - created_by is stamped by the server and then immutable
//   - archiving stamps who did it; restoring is admin-only
package hooks

import (
	"github.com/pocketbase/pocketbase/core"

	"github.com/dbowling/kamishibai/backend/internal/access"
	"github.com/dbowling/kamishibai/backend/internal/schema"
)

// Register binds every hook to the app.
func Register(app core.App) {
	registerUserHooks(app)
	registerTeamHooks(app)
	registerBoardHooks(app)
	registerCardHooks(app)
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

func registerUserHooks(app core.App) {
	// Normalise the role on every create, including programmatic ones such as
	// seeding. An empty role would still fail closed in the access rules, but
	// storing it explicitly keeps the data honest and the UI simpler.
	app.OnRecordCreate(schema.Users).BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetString(schema.FieldRole) == "" {
			e.Record.Set(schema.FieldRole, schema.RoleUser)
		}
		return e.Next()
	})

	// Privilege escalation guard.
	//
	// The users update rule intentionally lets people edit their own profile.
	// Without this, "edit your own record" would also mean "set your own role to
	// admin", which would hand any user access to every team.
	app.OnRecordUpdateRequest(schema.Users).BindFunc(func(e *core.RecordRequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}

		previous := e.Record.Original().GetString(schema.FieldRole)
		if e.Record.GetString(schema.FieldRole) != previous && !access.IsAdmin(e.Auth) {
			return e.ForbiddenError("Only an admin can change a user's role.", nil)
		}
		return e.Next()
	})
}

// ---------------------------------------------------------------------------
// Teams
// ---------------------------------------------------------------------------

func registerTeamHooks(app core.App) {
	app.OnRecordCreateRequest(schema.Teams).BindFunc(func(e *core.RecordRequestEvent) error {
		stampCreatedBy(e)
		return e.Next()
	})

	app.OnRecordUpdateRequest(schema.Teams).BindFunc(func(e *core.RecordRequestEvent) error {
		preserveCreatedBy(e)
		if err := guardArchiveTransition(e); err != nil {
			return err
		}
		return e.Next()
	})
}

// ---------------------------------------------------------------------------
// Boards
// ---------------------------------------------------------------------------

func registerBoardHooks(app core.App) {
	app.OnRecordCreateRequest(schema.Boards).BindFunc(func(e *core.RecordRequestEvent) error {
		stampCreatedBy(e)
		if err := requireWritableTeam(e, e.Record.GetString(schema.FieldTeam)); err != nil {
			return err
		}
		return e.Next()
	})

	app.OnRecordUpdateRequest(schema.Boards).BindFunc(func(e *core.RecordRequestEvent) error {
		preserveCreatedBy(e)
		if err := forbidTeamChange(e); err != nil {
			return err
		}
		if err := guardArchiveTransition(e); err != nil {
			return err
		}
		return e.Next()
	})
}

// ---------------------------------------------------------------------------
// Cards
// ---------------------------------------------------------------------------

func registerCardHooks(app core.App) {
	app.OnRecordCreateRequest(schema.Cards).BindFunc(func(e *core.RecordRequestEvent) error {
		stampCreatedBy(e)

		// Derive the team before authorising, so the check runs against the board's
		// real owner rather than whatever the client claimed.
		if err := deriveCardTeam(e); err != nil {
			return err
		}
		if err := requireWritableTeam(e, e.Record.GetString(schema.FieldTeam)); err != nil {
			return err
		}
		return e.Next()
	})

	app.OnRecordUpdateRequest(schema.Cards).BindFunc(func(e *core.RecordRequestEvent) error {
		preserveCreatedBy(e)

		if err := deriveCardTeam(e); err != nil {
			return err
		}
		// Because the team is derived from the board, pointing a card at another
		// team's board shows up here as a team change and is refused.
		if err := forbidTeamChange(e); err != nil {
			return err
		}
		if err := guardArchiveTransition(e); err != nil {
			return err
		}

		// Cadence may change: a task that was daily can become weekly. Existing
		// occurrences keep the cadence they were recorded under, which is correct.
		// They describe what was expected at the time, and rewriting them would
		// falsify past reports.
		return e.Next()
	})
}

// deriveCardTeam forces a card's team to match its board's team.
//
// The team column on cards is a denormalisation that the access rules depend on.
// If a client could set it freely it could make its card readable by a team that
// does not own the board, so the value is always recomputed here and the client's
// input discarded.
func deriveCardTeam(e *core.RecordRequestEvent) error {
	boardID := e.Record.GetString(schema.FieldBoard)
	if boardID == "" {
		return e.BadRequestError("A card must belong to a board.", nil)
	}

	board, err := e.App.FindRecordById(schema.Boards, boardID)
	if err != nil {
		return e.BadRequestError("Unknown board.", err)
	}
	if access.IsArchived(board) && !e.HasSuperuserAuth() {
		return e.BadRequestError("That board is archived and cannot take new or edited cards.", nil)
	}

	e.Record.Set(schema.FieldTeam, board.GetString(schema.FieldTeam))
	return nil
}

// ---------------------------------------------------------------------------
// Shared guards
// ---------------------------------------------------------------------------

// requireWritableTeam checks that the caller may write to the given team.
//
// The collection rules already restrict writes to team members. This adds the
// archived-team check and, for cards, applies it to the *derived* team.
func requireWritableTeam(e *core.RecordRequestEvent, teamID string) error {
	if e.HasSuperuserAuth() {
		return nil
	}
	if teamID == "" {
		return e.BadRequestError("A team is required.", nil)
	}

	team, err := e.App.FindRecordById(schema.Teams, teamID)
	if err != nil {
		return e.BadRequestError("Unknown team.", err)
	}
	if !access.CanWriteTeam(team, e.Auth) {
		return e.ForbiddenError("You are not a member of this team, or the team is archived.", nil)
	}
	return nil
}

// forbidTeamChange prevents a record from moving between teams.
//
// Occurrences carry a denormalised team, and rollups are aggregated per team.
// Moving a board or card after the fact would leave that history attributed to
// the wrong tenant, so it is refused outright rather than attempting a rewrite.
func forbidTeamChange(e *core.RecordRequestEvent) error {
	if e.HasSuperuserAuth() {
		return nil
	}

	previous := e.Record.Original().GetString(schema.FieldTeam)
	if previous != "" && e.Record.GetString(schema.FieldTeam) != previous {
		return e.BadRequestError("Records cannot be moved between teams, because their history is attributed to the original team.", nil)
	}
	return nil
}

// stampCreatedBy records the authenticated user as the creator, ignoring
// anything the client supplied.
func stampCreatedBy(e *core.RecordRequestEvent) {
	if e.Auth != nil {
		e.Record.Set(schema.FieldCreatedBy, e.Auth.Id)
	}
}

// preserveCreatedBy keeps created_by immutable across updates.
func preserveCreatedBy(e *core.RecordRequestEvent) {
	if e.HasSuperuserAuth() {
		return
	}
	e.Record.Set(schema.FieldCreatedBy, e.Record.Original().GetString(schema.FieldCreatedBy))
}

// guardArchiveTransition implements the archive/restore policy.
//
// Archiving is something any team member can do to tidy their own board.
// Restoring is reserved for admins, which is the asymmetry the requirement
// describes: things can be put away by anyone but only brought back by an admin.
// It also owns the archived_by column so a client cannot attribute an archival to
// someone else.
func guardArchiveTransition(e *core.RecordRequestEvent) error {
	if e.HasSuperuserAuth() {
		return nil
	}

	original := e.Record.Original()
	wasArchived := !original.GetDateTime(schema.FieldArchivedAt).IsZero()
	isArchived := !e.Record.GetDateTime(schema.FieldArchivedAt).IsZero()

	switch {
	case !wasArchived && isArchived:
		if e.Auth != nil {
			e.Record.Set(schema.FieldArchivedBy, e.Auth.Id)
		}

	case wasArchived && !isArchived:
		if !access.IsAdmin(e.Auth) {
			return e.ForbiddenError("Only an admin can restore an archived record.", nil)
		}
		e.Record.Set(schema.FieldArchivedBy, "")

	default:
		// Not an archive transition, so archived_by is not the client's to change.
		e.Record.Set(schema.FieldArchivedBy, original.GetString(schema.FieldArchivedBy))
	}
	return nil
}
