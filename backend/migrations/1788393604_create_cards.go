package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/dbowling/kamishibai/backend/internal/domain"
	"github.com/dbowling/kamishibai/backend/internal/schema"
)

// cards are the recurring triage tasks themselves. A card holds the *definition*
// of the work (what to do, where to look) and its cadence. It deliberately
// holds no status: state lives in occurrences, keyed by period.
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
		boards, err := app.FindCollectionByNameOrId(schema.Boards)
		if err != nil {
			return err
		}

		c := core.NewBaseCollection(schema.Cards)

		c.ListRule = types.Pointer(schema.TeamMember)
		c.ViewRule = types.Pointer(schema.TeamMember)
		// "Anyone on a team can make new cards."
		//
		// Create is authorised from the board rather than the card's own team,
		// because the team is derived server-side and is not present in the
		// submitted body that the create rule sees.
		c.CreateRule = types.Pointer(schema.BoardTeamMember)
		c.UpdateRule = types.Pointer(schema.TeamMember)
		// nil => superusers only. Cards are archived, never deleted, so their
		// occurrence history stays intact.
		c.DeleteRule = nil

		c.Fields.Add(
			&core.RelationField{
				Name:          schema.FieldBoard,
				CollectionId:  boards.Id,
				Required:      true,
				MaxSelect:     1,
				CascadeDelete: true,
			},
			// team is denormalised from board.team. It costs one extra id per
			// card and buys a single-hop access rule plus index-only reporting
			// queries that never have to join through boards. A hook keeps it
			// consistent so a client cannot set it to someone else's team.
			&core.RelationField{
				Name:          schema.FieldTeam,
				CollectionId:  teams.Id,
				Required:      true,
				MaxSelect:     1,
				CascadeDelete: true,
				Help:          "Derived from the board's team. Set automatically.",
			},
			&core.TextField{
				Name:        schema.FieldTitle,
				Required:    true,
				Max:         200,
				Presentable: true,
			},
			&core.TextField{
				Name: schema.FieldSummary,
				Max:  500,
				Help: "One-line description shown on the card face.",
			},
			&core.SelectField{
				Name:      schema.FieldCadence,
				Values:    domain.CadenceValues(),
				MaxSelect: 1,
				Required:  true,
				Help:      "How often this card resets to not-started.",
			},
			// Rich instructions: the body of the card. Holds the prose and any
			// inline links, e.g. "check the Grafana backup dashboard".
			&core.EditorField{
				Name:    schema.FieldInstructions,
				MaxSize: 100_000,
				Help:    "Rich text instructions for completing the task.",
			},
			// Quick-access links as structured data: [{"label":"...","url":"..."}]
			// Kept separate from instructions so the UI can render them as
			// buttons and so they stay machine-readable.
			&core.JSONField{
				Name:    schema.FieldLinks,
				MaxSize: 20_000,
				Help:    `Array of {"label","url"} objects rendered as quick links.`,
			},
			// Checklist steps: [{"text":"..."}]
			&core.JSONField{
				Name:    schema.FieldChecklist,
				MaxSize: 50_000,
				Help:    `Array of {"text"} steps, e.g. troubleshooting actions.`,
			},
			&core.NumberField{
				Name:    schema.FieldSortOrder,
				OnlyInt: true,
				Help:    "Ascending display order within the board.",
			},
			&core.DateField{
				Name: schema.FieldArchivedAt,
				Help: "Set when the card is archived. An admin can clear this to restore it.",
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

		// Rendering a board: active cards in display order.
		c.AddIndex("idx_cards_board_archived_sort", false, "board, archived_at, sort_order", "")
		// Rollups count active cards per board and cadence.
		c.AddIndex("idx_cards_board_cadence_archived", false, "board, cadence, archived_at", "")
		// Team-wide cadence views ("everything daily across my team").
		c.AddIndex("idx_cards_team_cadence", false, "team, cadence", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId(schema.Cards)
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
