package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/dbowling/kamishibai/backend/internal/domain"
	"github.com/dbowling/kamishibai/backend/internal/schema"
)

// report_rollups is the read side of the reporting story.
//
// Once a period closes, nothing about it can change, so the counts are computed
// once and frozen here: one row per board, cadence and period. Historical
// reporting then reads a table that grows with (boards x cadences x periods)
// instead of re-aggregating the occurrences table, and the "not started" count
// can be recorded as a real number even though no occurrence rows exist for it.
func init() {
	m.Register(func(app core.App) error {
		teams, err := app.FindCollectionByNameOrId(schema.Teams)
		if err != nil {
			return err
		}
		boards, err := app.FindCollectionByNameOrId(schema.Boards)
		if err != nil {
			return err
		}

		c := core.NewBaseCollection(schema.Rollups)

		c.ListRule = types.Pointer(schema.TeamMember)
		c.ViewRule = types.Pointer(schema.TeamMember)
		// Written exclusively by the rollup job (nil => superusers only), so a
		// client can never fabricate a completion statistic.
		c.CreateRule = nil
		c.UpdateRule = nil
		c.DeleteRule = nil

		c.Fields.Add(
			&core.RelationField{
				Name:          schema.FieldTeam,
				CollectionId:  teams.Id,
				Required:      true,
				MaxSelect:     1,
				CascadeDelete: true,
			},
			&core.RelationField{
				Name:          schema.FieldBoard,
				CollectionId:  boards.Id,
				Required:      true,
				MaxSelect:     1,
				CascadeDelete: true,
			},
			&core.SelectField{
				Name:      schema.FieldCadence,
				Values:    domain.CadenceValues(),
				MaxSelect: 1,
				Required:  true,
			},
			&core.TextField{
				Name:     schema.FieldPeriodKey,
				Required: true,
				Max:      20,
			},
			// Boundaries are stored alongside the key so that charting a series
			// does not need to re-derive dates, and so a report stays readable
			// even if the instance timezone is later changed.
			&core.DateField{Name: schema.FieldPeriodStart, Required: true},
			&core.DateField{Name: schema.FieldPeriodEnd, Required: true},

			// total_cards is the denominator: how many active cards of this
			// cadence existed on the board when the period closed.
			&core.NumberField{Name: schema.FieldTotalCards, OnlyInt: true, Min: types.Pointer(0.0)},
			&core.NumberField{Name: schema.FieldDoneCount, OnlyInt: true, Min: types.Pointer(0.0)},
			&core.NumberField{Name: schema.FieldInProgressCount, OnlyInt: true, Min: types.Pointer(0.0)},
			// Derived as total - done - in_progress. Stored explicitly because
			// it is the number people actually ask about ("what did we miss?")
			// and it has no rows of its own to count.
			&core.NumberField{Name: schema.FieldNotStartedCount, OnlyInt: true, Min: types.Pointer(0.0)},

			&core.NumberField{
				Name: schema.FieldCompletionRate,
				Min:  types.Pointer(0.0),
				Max:  types.Pointer(1.0),
				Help: "done_count / total_cards, in the range 0..1. Zero when the board had no cards.",
			},
			&core.DateField{
				Name:     schema.FieldClosedAt,
				Required: true,
				Help:     "When this snapshot was taken.",
			},
			&core.AutodateField{Name: schema.FieldCreated, OnCreate: true},
			&core.AutodateField{Name: schema.FieldUpdated, OnCreate: true, OnUpdate: true},
		)

		// One snapshot per board/cadence/period. Makes the rollup job idempotent:
		// it can re-run over the same window and simply update in place, which is
		// what lets it backfill after downtime.
		c.AddIndex("idx_rollups_board_cadence_period", true, "board, cadence, period_key", "")
		// Team-level trend queries.
		c.AddIndex("idx_rollups_team_cadence_period", false, "team, cadence, period_key", "")

		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId(schema.Rollups)
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
