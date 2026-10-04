package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/dbowling/kamishibai/backend/internal/schema"
)

// Make the sidebar structure an administrator's job.
//
// Two changes, both in service of letting admins manage the left-hand navigation
// from the app:
//
//   - teams gain a sort_order, so admins can arrange them. Boards already had
//     one. Existing teams default to 0, which ties; the UI breaks ties by name,
//     so nothing reshuffles until an admin drags something.
//
//   - boards are no longer created or edited by ordinary team members. Until now
//     any member could spin up or rename a board for their team, which is the
//     wrong trade once boards can also be reordered and moved between teams: the
//     structure of the sidebar is shared by everyone, so one person should not be
//     able to rearrange it for the rest. Cards, which are the day-to-day work, stay
//     open to members.
//
// Archiving a board is an update, so it follows the update rule and is admin-only
// as a consequence. Reads are untouched: members still see their team's boards.
func init() {
	m.Register(func(app core.App) error {
		teams, err := app.FindCollectionByNameOrId(schema.Teams)
		if err != nil {
			return err
		}
		teams.Fields.Add(&core.NumberField{
			Name:    schema.FieldSortOrder,
			OnlyInt: true,
			Help:    "Ascending display order in the sidebar.",
		})
		if err := app.Save(teams); err != nil {
			return err
		}

		boards, err := app.FindCollectionByNameOrId(schema.Boards)
		if err != nil {
			return err
		}
		boards.CreateRule = types.Pointer(schema.AdminOnly)
		boards.UpdateRule = types.Pointer(schema.AdminOnly)
		return app.Save(boards)
	}, func(app core.App) error {
		boards, err := app.FindCollectionByNameOrId(schema.Boards)
		if err != nil {
			return err
		}
		boards.CreateRule = types.Pointer(schema.TeamMember)
		boards.UpdateRule = types.Pointer(schema.TeamMember)
		if err := app.Save(boards); err != nil {
			return err
		}

		teams, err := app.FindCollectionByNameOrId(schema.Teams)
		if err != nil {
			return err
		}
		teams.Fields.RemoveByName(schema.FieldSortOrder)
		return app.Save(teams)
	})
}
