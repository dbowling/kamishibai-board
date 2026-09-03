package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/dbowling/kamishibai/backend/internal/domain"
	"github.com/dbowling/kamishibai/backend/internal/schema"
)

// occurrences records what happened to one card during one period. It is the
// only table that grows with time, so its shape is where the capacity story
// lives.
//
// Two decisions keep it small:
//
//  1. Rows are written lazily. A row exists only once somebody starts or
//     completes the card. A daily card nobody touches for a year costs zero
//     rows, not 365, and "not started" is represented by the absence of a row.
//     This is also why the board appears to flip with no write at all: the
//     period key rolls over and the lookup simply finds nothing.
//
//  2. Closed periods are summarised into report_rollups, so historical
//     reporting reads a small table and never scans this one.
//
// The status field only ever holds the persisted states (in_progress, done);
// not_started is synthetic and never stored.
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
		cards, err := app.FindCollectionByNameOrId(schema.Cards)
		if err != nil {
			return err
		}

		c := core.NewBaseCollection(schema.Occurrences)

		// Members of the owning team can read occurrences, including archived
		// ones, so history and reporting stay available.
		c.ListRule = types.Pointer(schema.TeamMember)
		c.ViewRule = types.Pointer(schema.TeamMember)

		// Writes are closed to clients entirely (nil => superusers only).
		//
		// All mutations go through the purpose-built endpoints in
		// internal/api (start / complete / reopen). That is what makes
		// attribution trustworthy: the server stamps started_by and
		// completed_by from the authenticated request, derives period_key from
		// the server clock, and refuses writes against periods that have
		// already closed. If clients could POST here directly, any of those
		// could be forged.
		c.CreateRule = nil
		c.UpdateRule = nil
		c.DeleteRule = nil

		c.Fields.Add(
			&core.RelationField{
				Name:          schema.FieldCard,
				CollectionId:  cards.Id,
				Required:      true,
				MaxSelect:     1,
				CascadeDelete: true,
			},
			// board, team and cadence are denormalised from the card. Reporting
			// aggregates by team/board/cadence constantly, and carrying them
			// here keeps those queries index-only instead of joining up through
			// cards and boards on a table that grows without bound.
			&core.RelationField{
				Name:          schema.FieldBoard,
				CollectionId:  boards.Id,
				Required:      true,
				MaxSelect:     1,
				CascadeDelete: true,
				Help:          "Derived from the card. Set automatically.",
			},
			&core.RelationField{
				Name:          schema.FieldTeam,
				CollectionId:  teams.Id,
				Required:      true,
				MaxSelect:     1,
				CascadeDelete: true,
				Help:          "Derived from the card. Set automatically.",
			},
			&core.SelectField{
				Name:      schema.FieldCadence,
				Values:    domain.CadenceValues(),
				MaxSelect: 1,
				Required:  true,
				Help:      "Derived from the card. Set automatically.",
			},
			&core.TextField{
				Name:     schema.FieldPeriodKey,
				Required: true,
				Max:      20,
				Help:     "Canonical period identifier, e.g. 2026-09-03, 2026-W36, 2026-M09, 2026-Q3, 2026-Y.",
			},
			&core.SelectField{
				Name:      schema.FieldStatus,
				Values:    domain.PersistedStatusValues(),
				MaxSelect: 1,
				Required:  true,
				Help:      "Only in_progress and done are stored; not_started is the absence of a row.",
			},
			&core.RelationField{
				Name:         schema.FieldStartedBy,
				CollectionId: users.Id,
				MaxSelect:    1,
			},
			&core.DateField{Name: schema.FieldStartedAt},
			&core.RelationField{
				Name:         schema.FieldCompletedBy,
				CollectionId: users.Id,
				MaxSelect:    1,
			},
			&core.DateField{Name: schema.FieldCompletedAt},
			&core.TextField{
				Name: schema.FieldNotes,
				Max:  2000,
				Help: "Optional findings recorded while working the card.",
			},
			&core.AutodateField{Name: schema.FieldCreated, OnCreate: true},
			&core.AutodateField{Name: schema.FieldUpdated, OnCreate: true, OnUpdate: true},
		)

		// The load-bearing constraint. One row per card per period, enforced by
		// the database rather than by application logic, which is what makes the
		// lazy "create it if somebody touches it" upsert safe against two people
		// clicking Start at the same moment.
		c.AddIndex("idx_occurrences_card_period", true, "card, period_key", "")

		// Rendering a board: fetch every occurrence on the board for the handful
		// of period keys currently in play.
		c.AddIndex("idx_occurrences_board_period", false, "board, period_key", "")

		// Rollups and team-level reporting aggregate by team + cadence + period.
		c.AddIndex("idx_occurrences_team_cadence_period", false, "team, cadence, period_key", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId(schema.Occurrences)
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
