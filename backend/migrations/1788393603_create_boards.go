package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/dbowling/kamishibai/backend/internal/schema"
)

// boards belong to a team and hold cards. A team can run several boards, for
// example "Infrastructure Triage" and "Security Triage".
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId(schema.Users)
		if err != nil {
			return err
		}
		teams, err := app.FindCollectionByNameOrId(schema.Teams)
		if err != nil {
			return err
		}

		c := core.NewBaseCollection(schema.Boards)

		c.ListRule = types.Pointer(schema.TeamMember)
		c.ViewRule = types.Pointer(schema.TeamMember)
		// Any member of the team can spin up a board for that team.
		c.CreateRule = types.Pointer(schema.TeamMember)
		c.UpdateRule = types.Pointer(schema.TeamMember)
		// nil => superusers only. Boards are archived, never deleted.
		c.DeleteRule = nil

		c.Fields.Add(
			&core.RelationField{
				Name:         schema.FieldTeam,
				CollectionId: teams.Id,
				Required:     true,
				MaxSelect:    1,
				// If a team is ever hard-deleted by a superuser, take its boards
				// with it rather than leaving orphans. The normal path is archive.
				CascadeDelete: true,
			},
			&core.TextField{
				Name:        schema.FieldName,
				Required:    true,
				Max:         100,
				Presentable: true,
			},
			&core.TextField{
				Name: schema.FieldDescription,
				Max:  500,
			},
			&core.NumberField{
				Name:    schema.FieldSortOrder,
				OnlyInt: true,
				Help:    "Ascending display order within the team.",
			},
			&core.DateField{
				Name: schema.FieldArchivedAt,
				Help: "Set when the board is archived. An admin can clear this to restore it.",
			},
			&core.RelationField{
				Name:         schema.FieldArchivedBy,
				CollectionId: users.Id,
				MaxSelect:    1,
			},
			&core.RelationField{
				Name:         schema.FieldCreatedBy,
				CollectionId: users.Id,
				MaxSelect:    1,
			},
			&core.AutodateField{Name: schema.FieldCreated, OnCreate: true},
			&core.AutodateField{Name: schema.FieldUpdated, OnCreate: true, OnUpdate: true},
		)

		// Board names are unique per team among active boards.
		c.AddIndex("idx_boards_team_name_active", true, "team, name", "archived_at = ''")
		// Drives the "boards for my team" listing.
		c.AddIndex("idx_boards_team_archived", false, "team, archived_at", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId(schema.Boards)
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
