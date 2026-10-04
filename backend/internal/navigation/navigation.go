// Package navigation owns the structure of the sidebar: the order of teams and
// boards, and moving a board from one team to another.
//
// Both operations are administrator-only and both touch several rows that must
// change together, so they live here rather than in the HTTP handler. The admin
// check is the first thing each exported function does, which means no future
// caller (another endpoint, a CLI command) can reach the writes without passing
// it. This mirrors how internal/occurrence keeps its own authorisation.
//
// Moving a board is the one sanctioned way for a record to change teams. The
// generic collection API still refuses it (see internal/hooks), because that path
// would leave occurrences and rollups attributed to the old team. Here the whole
// history is re-pointed in the same transaction as the board itself.
package navigation

import (
	"errors"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/dbowling/kamishibai/backend/internal/access"
	"github.com/dbowling/kamishibai/backend/internal/schema"
)

var (
	// ErrForbidden means the caller is not an admin.
	ErrForbidden = errors.New("admin only")

	// ErrBoardNotFound means no such board exists.
	ErrBoardNotFound = errors.New("board not found")
)

// InvalidError is a request the caller got wrong. Its message is safe to show to
// the user, and the HTTP layer maps it to 400.
type InvalidError struct{ Message string }

func (e *InvalidError) Error() string { return e.Message }

func invalid(format string, args ...any) error {
	return &InvalidError{Message: fmt.Sprintf(format, args...)}
}

// DuplicateBoardMessage is returned when the target team already has an active
// board with the same name.
const DuplicateBoardMessage = "A board with that name already exists on the target team."

// Order is the requested display order. Each slice lists record ids in the order
// they should appear; the index becomes the sort_order.
type Order struct {
	Teams  []string            `json:"teams"`
	Boards map[string][]string `json:"boards"`
}

// SetOrder applies an Order in a single transaction.
//
// Everything is validated before the first write, and the whole thing is inside
// RunInTransaction as well, so a bad id halfway down the list cannot leave the
// sidebar half-reordered. A board listed under a team it does not belong to is
// rejected rather than moved: reordering must never change ownership, which has
// its own endpoint and its own checks.
func SetOrder(app core.App, user *core.Record, order Order) error {
	if !access.IsAdmin(user) {
		return ErrForbidden
	}

	return app.RunInTransaction(func(txApp core.App) error {
		type update struct {
			record *core.Record
			order  int
		}
		var updates []update

		if err := noDuplicates(order.Teams); err != nil {
			return err
		}
		for i, id := range order.Teams {
			team, err := txApp.FindRecordById(schema.Teams, id)
			if err != nil {
				return invalid("Unknown team %q.", id)
			}
			updates = append(updates, update{team, i})
		}

		for teamID, ids := range order.Boards {
			if _, err := txApp.FindRecordById(schema.Teams, teamID); err != nil {
				return invalid("Unknown team %q.", teamID)
			}
			if err := noDuplicates(ids); err != nil {
				return err
			}
			for i, id := range ids {
				board, err := txApp.FindRecordById(schema.Boards, id)
				if err != nil {
					return invalid("Unknown board %q.", id)
				}
				if board.GetString(schema.FieldTeam) != teamID {
					return invalid("Board %q does not belong to team %q.", id, teamID)
				}
				updates = append(updates, update{board, i})
			}
		}

		for _, u := range updates {
			// Skip no-ops so a drag that changes one row does not emit realtime
			// events and bump `updated` on every other row in the list.
			if u.record.GetInt(schema.FieldSortOrder) == u.order {
				continue
			}
			u.record.Set(schema.FieldSortOrder, u.order)
			if err := txApp.Save(u.record); err != nil {
				return fmt.Errorf("save sort order for %s: %w", u.record.Id, err)
			}
		}
		return nil
	})
}

func noDuplicates(ids []string) error {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			return invalid("Id %q is listed more than once.", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

// MoveResult reports what a move changed.
type MoveResult struct {
	BoardID     string
	TeamID      string
	Cards       int
	Occurrences int
	Rollups     int
}

// MoveBoard re-homes a board, with all of its history, onto another team.
//
// Cards, occurrences and rollups each carry a denormalised team that the access
// rules read, so they have to follow the board or the old team would keep seeing
// (and the new team could not see) the board's history. The board itself is saved
// through the app so hooks and realtime observe the change; the children are
// updated with one statement each, because there can be thousands of occurrences
// and nothing listens for per-row events on them.
func MoveBoard(app core.App, user *core.Record, boardID, targetTeamID string) (MoveResult, error) {
	var result MoveResult

	if !access.IsAdmin(user) {
		return result, ErrForbidden
	}

	err := app.RunInTransaction(func(txApp core.App) error {
		board, err := txApp.FindRecordById(schema.Boards, boardID)
		if err != nil {
			return ErrBoardNotFound
		}

		if targetTeamID == "" {
			return invalid("A target team is required.")
		}
		target, err := txApp.FindRecordById(schema.Teams, targetTeamID)
		if err != nil {
			return invalid("Unknown target team.")
		}

		sourceID := board.GetString(schema.FieldTeam)
		if sourceID == targetTeamID {
			return invalid("The board is already on that team.")
		}
		if access.IsArchived(board) {
			return invalid("Archived boards cannot be moved. Restore it first.")
		}
		source, err := txApp.FindRecordById(schema.Teams, sourceID)
		if err != nil {
			return fmt.Errorf("load source team: %w", err)
		}
		if access.IsArchived(source) || access.IsArchived(target) {
			return invalid("Boards cannot be moved to or from an archived team.")
		}

		// Active-name uniqueness is also a unique index, but checking first gives
		// a readable message instead of a constraint failure.
		clash, err := txApp.FindFirstRecordByFilter(schema.Boards,
			"team = {:team} && name = {:name} && archived_at = ''",
			dbx.Params{"team": targetTeamID, "name": board.GetString(schema.FieldName)})
		if err == nil && clash != nil {
			return invalid(DuplicateBoardMessage)
		}

		var maxOrder int
		if err := txApp.DB().
			Select("COALESCE(MAX(" + schema.FieldSortOrder + "), 0)").
			From(schema.Boards).
			Where(dbx.HashExp{schema.FieldTeam: targetTeamID}).
			Row(&maxOrder); err != nil {
			return fmt.Errorf("find next sort order: %w", err)
		}

		board.Set(schema.FieldTeam, targetTeamID)
		board.Set(schema.FieldSortOrder, maxOrder+1)
		if err := txApp.Save(board); err != nil {
			return fmt.Errorf("save board: %w", err)
		}

		counts := map[string]*int{
			schema.Cards:       &result.Cards,
			schema.Occurrences: &result.Occurrences,
			schema.Rollups:     &result.Rollups,
		}
		// Fixed order keeps the SQL sequence deterministic.
		for _, table := range []string{schema.Cards, schema.Occurrences, schema.Rollups} {
			res, err := txApp.DB().
				Update(table, dbx.Params{schema.FieldTeam: targetTeamID},
					dbx.HashExp{schema.FieldBoard: boardID}).
				Execute()
			if err != nil {
				return fmt.Errorf("move %s: %w", table, err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return fmt.Errorf("count moved %s: %w", table, err)
			}
			*counts[table] = int(n)
		}

		result.BoardID = board.Id
		result.TeamID = targetTeamID
		return nil
	})
	if err != nil {
		return MoveResult{}, err
	}
	return result, nil
}
