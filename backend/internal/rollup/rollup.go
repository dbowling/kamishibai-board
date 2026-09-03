// Package rollup freezes completion statistics for periods that have closed.
//
// The occurrences table is deliberately sparse: it holds nothing for work that
// was never started. That is excellent for write capacity but awkward for
// reporting, because "how much did we miss last month" cannot be counted from
// rows that do not exist. It also means every historical query would have to
// re-derive the denominator.
//
// So once a period closes, and its contents can no longer change, the counts are
// computed once and stored as a single row per board, cadence and period.
// Reporting then reads a small, dense table.
package rollup

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/dbowling/kamishibai/backend/internal/domain"
	"github.com/dbowling/kamishibai/backend/internal/schema"
)

// Snapshot is the computed state of one board, cadence and period.
type Snapshot struct {
	TeamID  string
	BoardID string
	Cadence domain.Cadence
	Period  domain.Period

	// TotalCards is the denominator: how many cards of this cadence the board was
	// expected to work during this period.
	TotalCards int
	Done       int
	InProgress int
	NotStarted int

	// CompletionRate is Done/TotalCards in the range 0..1, or 0 when the board
	// had no cards of this cadence.
	CompletionRate float64
}

// Empty reports whether the snapshot describes nothing at all, meaning the board
// had no cards of this cadence in this period and no work was recorded.
//
// Empty snapshots are not stored. Without this, a board holding only daily cards
// would still accrue weekly, monthly, quarterly and annual rows forever.
func (s Snapshot) Empty() bool {
	return s.TotalCards == 0 && s.Done == 0 && s.InProgress == 0
}

// Report summarises what a rollup run did.
type Report struct {
	BoardsScanned  int
	PeriodsChecked int
	Created        int
	Updated        int
	Skipped        int
}

// Service computes and persists snapshots.
type Service struct {
	cal *domain.Calendar
	now func() time.Time
}

// NewService returns a Service that evaluates periods using cal.
func NewService(cal *domain.Calendar) *Service {
	return &Service{cal: cal, now: time.Now}
}

// WithClock returns a copy of the service using a custom clock.
func (s *Service) WithClock(now func() time.Time) *Service {
	clone := *s
	clone.now = now
	return &clone
}

// Compute derives the snapshot for a board, cadence and period from the data on
// disk. It does not write anything.
func (s *Service) Compute(app core.App, board *core.Record, cadence domain.Cadence, period domain.Period) (Snapshot, error) {
	snap := Snapshot{
		TeamID:  board.GetString(schema.FieldTeam),
		BoardID: board.Id,
		Cadence: cadence,
		Period:  period,
	}

	start, err := types.ParseDateTime(period.Start)
	if err != nil {
		return Snapshot{}, fmt.Errorf("convert period start: %w", err)
	}
	end, err := types.ParseDateTime(period.End)
	if err != nil {
		return Snapshot{}, fmt.Errorf("convert period end: %w", err)
	}

	// The denominator is the set of cards that actually applied during this
	// period, not the set that exists now. A card created last week was not
	// expected last month, and a card archived last month should stop counting
	// against the team afterwards. Getting this wrong would quietly distort every
	// historical percentage whenever the board changed shape.
	total, err := app.CountRecords(
		schema.Cards,
		dbx.HashExp{
			schema.FieldBoard:   board.Id,
			schema.FieldCadence: string(cadence),
		},
		dbx.NewExp(schema.FieldCreated+" < {:end}", dbx.Params{"end": end.String()}),
		dbx.NewExp(
			"("+schema.FieldArchivedAt+" = '' OR "+schema.FieldArchivedAt+" IS NULL OR "+schema.FieldArchivedAt+" > {:start})",
			dbx.Params{"start": start.String()},
		),
	)
	if err != nil {
		return Snapshot{}, fmt.Errorf("count cards: %w", err)
	}
	snap.TotalCards = int(total)

	// The denormalised board, cadence and period_key columns on occurrences make
	// these two counts index-only lookups, with no join back through cards.
	scope := dbx.HashExp{
		schema.FieldBoard:     board.Id,
		schema.FieldCadence:   string(cadence),
		schema.FieldPeriodKey: period.Key,
	}

	done, err := app.CountRecords(schema.Occurrences, scope,
		dbx.HashExp{schema.FieldStatus: string(domain.StatusDone)})
	if err != nil {
		return Snapshot{}, fmt.Errorf("count completed occurrences: %w", err)
	}
	snap.Done = int(done)

	inProgress, err := app.CountRecords(schema.Occurrences, scope,
		dbx.HashExp{schema.FieldStatus: string(domain.StatusInProgress)})
	if err != nil {
		return Snapshot{}, fmt.Errorf("count in-progress occurrences: %w", err)
	}
	snap.InProgress = int(inProgress)

	// Not-started has no rows to count, which is the whole point of the lazy
	// model, so it is derived. Clamped at zero to stay sane if the card set
	// shifted in a way that makes the recorded work exceed the denominator.
	snap.NotStarted = snap.TotalCards - snap.Done - snap.InProgress
	if snap.NotStarted < 0 {
		snap.NotStarted = 0
	}

	if snap.TotalCards > 0 {
		snap.CompletionRate = float64(snap.Done) / float64(snap.TotalCards)
	}

	return snap, nil
}

// Persist writes a snapshot, updating any existing row for the same board,
// cadence and period. It reports whether a new row was created.
//
// Idempotent by design: the unique index on (board, cadence, period_key) means
// re-running over the same window converges rather than duplicating.
func (s *Service) Persist(app core.App, snap Snapshot) (created bool, err error) {
	existing, err := s.Find(app, snap.BoardID, snap.Cadence, snap.Period.Key)
	if err != nil {
		return false, err
	}

	record := existing
	if record == nil {
		collection, err := app.FindCollectionByNameOrId(schema.Rollups)
		if err != nil {
			return false, fmt.Errorf("load rollups collection: %w", err)
		}
		record = core.NewRecord(collection)
		created = true
	}

	start, err := types.ParseDateTime(snap.Period.Start)
	if err != nil {
		return false, fmt.Errorf("convert period start: %w", err)
	}
	end, err := types.ParseDateTime(snap.Period.End)
	if err != nil {
		return false, fmt.Errorf("convert period end: %w", err)
	}
	closedAt, err := types.ParseDateTime(s.now())
	if err != nil {
		return false, fmt.Errorf("convert closed_at: %w", err)
	}

	record.Set(schema.FieldTeam, snap.TeamID)
	record.Set(schema.FieldBoard, snap.BoardID)
	record.Set(schema.FieldCadence, string(snap.Cadence))
	record.Set(schema.FieldPeriodKey, snap.Period.Key)
	record.Set(schema.FieldPeriodStart, start)
	record.Set(schema.FieldPeriodEnd, end)
	record.Set(schema.FieldTotalCards, snap.TotalCards)
	record.Set(schema.FieldDoneCount, snap.Done)
	record.Set(schema.FieldInProgressCount, snap.InProgress)
	record.Set(schema.FieldNotStartedCount, snap.NotStarted)
	record.Set(schema.FieldCompletionRate, snap.CompletionRate)
	record.Set(schema.FieldClosedAt, closedAt)

	if err := app.Save(record); err != nil {
		return false, fmt.Errorf("save rollup: %w", err)
	}
	return created, nil
}

// Find returns the stored snapshot for a board, cadence and period, or nil.
func (s *Service) Find(app core.App, boardID string, cadence domain.Cadence, periodKey string) (*core.Record, error) {
	record, err := app.FindFirstRecordByFilter(
		schema.Rollups,
		"board = {:board} && cadence = {:cadence} && period_key = {:key}",
		dbx.Params{"board": boardID, "cadence": string(cadence), "key": periodKey},
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find rollup: %w", err)
	}
	return record, nil
}

// Run brings the rollup table up to date.
//
// For every board and cadence it walks back over the most recent `lookback`
// closed periods and makes sure each has a snapshot. Deliberately not "just do
// the period that closed a minute ago": if the process was down over a weekend,
// or a period closed while a deploy was rolling, walking back fills the hole
// instead of leaving a permanent gap in the history. Snapshots are idempotent so
// the repeated work is harmless.
func (s *Service) Run(app core.App, lookback int) (Report, error) {
	var report Report

	if lookback < 1 {
		return report, fmt.Errorf("lookback must be at least 1, got %d", lookback)
	}

	// Archived boards are included: a board archived mid-period still deserves
	// its final snapshot, and no new work can land on it anyway.
	boards, err := app.FindAllRecords(schema.Boards)
	if err != nil {
		return report, fmt.Errorf("list boards: %w", err)
	}

	now := s.now()

	for _, board := range boards {
		report.BoardsScanned++

		for _, cadence := range domain.Cadences() {
			periods, err := s.cal.ClosedBefore(cadence, now, lookback)
			if err != nil {
				return report, fmt.Errorf("board %q, cadence %s: %w", board.Id, cadence, err)
			}

			for _, period := range periods {
				report.PeriodsChecked++

				snap, err := s.Compute(app, board, cadence, period)
				if err != nil {
					return report, fmt.Errorf("board %q, period %s: %w", board.Id, period, err)
				}

				// Don't manufacture rows for cadences this board never used.
				if snap.Empty() {
					report.Skipped++
					continue
				}

				created, err := s.Persist(app, snap)
				if err != nil {
					return report, fmt.Errorf("board %q, period %s: %w", board.Id, period, err)
				}
				if created {
					report.Created++
				} else {
					report.Updated++
				}
			}
		}
	}

	return report, nil
}
