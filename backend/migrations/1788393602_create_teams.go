package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/dbowling/kamishibai/backend/internal/schema"
)

// teams is the tenant boundary. Users can belong to many teams; admins are
// implicitly on all of them via their role rather than by being listed here.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId(schema.Users)
		if err != nil {
			return err
		}

		c := core.NewBaseCollection(schema.Teams)

		c.ListRule = types.Pointer(schema.OwnTeam)
		c.ViewRule = types.Pointer(schema.OwnTeam)
		// Creating and renaming teams is an admin action. Everything *inside* a
		// team (boards, cards) is open to its members.
		c.CreateRule = types.Pointer(schema.AdminOnly)
		c.UpdateRule = types.Pointer(schema.AdminOnly)
		// nil => superusers only. Teams are archived, never deleted.
		c.DeleteRule = nil

		c.Fields.Add(
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
			// Membership as a multi-relation rather than a join collection. It
			// keeps the access rules to a single `members ?= @request.auth.id`
			// check instead of a back-relation traversal on every request.
			&core.RelationField{
				Name:          schema.FieldMembers,
				CollectionId:  users.Id,
				MaxSelect:     500,
				CascadeDelete: false,
				Help:          "Users on this team. Admins have access regardless of this list.",
			},
			&core.DateField{
				Name: schema.FieldArchivedAt,
				Help: "Set when the team is archived. Archived teams stay readable so history survives.",
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

		// Team names are unique among *active* teams only, so archiving a team
		// frees its name for reuse.
		c.AddIndex("idx_teams_name_active", true, schema.FieldName, "archived_at = ''")
		c.AddIndex("idx_teams_archived_at", false, schema.FieldArchivedAt, "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId(schema.Teams)
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
