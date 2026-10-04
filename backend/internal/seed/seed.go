// Package seed loads a repeatable demo dataset.
//
// "Repeatable" is the requirement that shapes the whole package: running the
// seed twice must leave the database in the same state as running it once.
// Records are therefore located by natural key (email, team name, board name
// within a team, card title within a board) and updated in place rather than
// inserted blindly.
//
// The generated history is also deterministic: it is driven by a seeded PRNG, so
// the same command produces the same completion pattern every time and tests can
// assert against it.
package seed

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/dbowling/kamishibai/backend/internal/domain"
	"github.com/dbowling/kamishibai/backend/internal/rollup"
	"github.com/dbowling/kamishibai/backend/internal/schema"
)

// DefaultPassword is the password given to every seeded account.
//
// Safe only because seeding is a development and test tool: the accounts it
// creates all use the reserved .test domain, and Run refuses to touch anything
// it did not create.
const DefaultPassword = "kamishibai-dev-1234"

// DefaultHistoryPeriods is how many closed periods of history to invent.
const DefaultHistoryPeriods = 45

// Options controls a seed run.
type Options struct {
	// Reset removes previously seeded records before inserting. It only ever
	// deletes records matching the seed's own natural keys.
	Reset bool

	// HistoryPeriods is how many closed periods of completion history to
	// generate. Zero means none.
	HistoryPeriods int

	// Password for seeded accounts.
	Password string

	// Rollups controls whether reporting snapshots are computed after seeding.
	Rollups bool

	// Now overrides the clock, for deterministic tests.
	Now func() time.Time
}

// Result reports what a seed run did.
type Result struct {
	Users       int
	Teams       int
	Boards      int
	Cards       int
	Occurrences int
	Deleted     int
	Rollups     rollup.Report
}

// Seeder loads the demo dataset.
type Seeder struct {
	cal  *domain.Calendar
	opts Options
}

// New returns a Seeder, filling in default options.
func New(cal *domain.Calendar, opts Options) *Seeder {
	if opts.Password == "" {
		opts.Password = DefaultPassword
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Seeder{cal: cal, opts: opts}
}

// Run loads the dataset.
//
// The whole run is wrapped in a transaction so a failure part-way leaves nothing
// behind, which matters for a command people will re-run after fixing a typo.
func (s *Seeder) Run(app core.App) (Result, error) {
	var result Result

	err := app.RunInTransaction(func(txApp core.App) error {
		if s.opts.Reset {
			deleted, err := s.reset(txApp)
			if err != nil {
				return fmt.Errorf("reset: %w", err)
			}
			result.Deleted = deleted
		}

		users, err := s.seedUsers(txApp)
		if err != nil {
			return fmt.Errorf("users: %w", err)
		}
		result.Users = len(users)

		teams, err := s.seedTeams(txApp, users)
		if err != nil {
			return fmt.Errorf("teams: %w", err)
		}
		result.Teams = len(teams)

		boards, err := s.seedBoards(txApp, teams)
		if err != nil {
			return fmt.Errorf("boards: %w", err)
		}
		result.Boards = len(boards)

		cards, err := s.seedCards(txApp, boards)
		if err != nil {
			return fmt.Errorf("cards: %w", err)
		}
		result.Cards = len(cards)

		if s.opts.HistoryPeriods > 0 {
			written, err := s.seedHistory(txApp, cards, users)
			if err != nil {
				return fmt.Errorf("history: %w", err)
			}
			result.Occurrences = written
		}

		return nil
	})
	if err != nil {
		return Result{}, err
	}

	// Rollups run outside the seeding transaction: they read a lot and the data
	// they summarise is already committed by this point.
	if s.opts.Rollups && s.opts.HistoryPeriods > 0 {
		service := rollup.NewService(s.cal).WithClock(s.opts.Now)
		report, err := service.Run(app, s.opts.HistoryPeriods+1)
		if err != nil {
			return result, fmt.Errorf("rollups: %w", err)
		}
		result.Rollups = report
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Reset
// ---------------------------------------------------------------------------

// reset deletes the records this seeder manages.
//
// Scoped deliberately narrowly: only the exact team names and user emails in the
// demo dataset. A destructive command that could reach real data would be a poor
// trade for a little convenience. Boards, cards and occurrences are removed by
// the cascade on their team relation.
func (s *Seeder) reset(app core.App) (int, error) {
	deleted := 0

	for _, spec := range demoTeams() {
		team, err := findByFilter(app, schema.Teams, "name = {:name}", dbx.Params{"name": spec.Name})
		if err != nil {
			return deleted, err
		}
		if team == nil {
			continue
		}
		if err := app.Delete(team); err != nil {
			return deleted, fmt.Errorf("delete team %q: %w", spec.Name, err)
		}
		deleted++
	}

	for _, spec := range demoUsers() {
		user, err := app.FindAuthRecordByEmail(schema.Users, spec.Email)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return deleted, fmt.Errorf("find user %q: %w", spec.Email, err)
		}
		if err := app.Delete(user); err != nil {
			return deleted, fmt.Errorf("delete user %q: %w", spec.Email, err)
		}
		deleted++
	}

	return deleted, nil
}

// ---------------------------------------------------------------------------
// Users, teams, boards, cards
// ---------------------------------------------------------------------------

func (s *Seeder) seedUsers(app core.App) (map[string]*core.Record, error) {
	collection, err := app.FindCollectionByNameOrId(schema.Users)
	if err != nil {
		return nil, err
	}

	out := make(map[string]*core.Record)

	for _, spec := range demoUsers() {
		record, err := app.FindAuthRecordByEmail(schema.Users, spec.Email)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("find user %q: %w", spec.Email, err)
		}
		if record == nil {
			record = core.NewRecord(collection)
			record.Set("email", spec.Email)
			record.SetPassword(s.opts.Password)
		}

		record.Set(schema.FieldName, spec.Name)
		record.Set(schema.FieldRole, spec.Role)
		record.Set("verified", true)

		if err := app.Save(record); err != nil {
			return nil, fmt.Errorf("save user %q: %w", spec.Email, err)
		}
		out[spec.Email] = record
	}

	return out, nil
}

func (s *Seeder) seedTeams(app core.App, users map[string]*core.Record) (map[string]*core.Record, error) {
	collection, err := app.FindCollectionByNameOrId(schema.Teams)
	if err != nil {
		return nil, err
	}

	admin := users["admin@example.test"]
	out := make(map[string]*core.Record)

	for _, spec := range demoTeams() {
		record, err := findByFilter(app, schema.Teams, "name = {:name}", dbx.Params{"name": spec.Name})
		if err != nil {
			return nil, err
		}
		if record == nil {
			record = core.NewRecord(collection)
			record.Set(schema.FieldName, spec.Name)
		}

		memberIDs := make([]string, 0, len(spec.MemberEmail))
		for _, email := range spec.MemberEmail {
			user, ok := users[email]
			if !ok {
				return nil, fmt.Errorf("team %q references unknown user %q", spec.Name, email)
			}
			memberIDs = append(memberIDs, user.Id)
		}

		record.Set(schema.FieldDescription, spec.Description)
		record.Set(schema.FieldMembers, memberIDs)
		// Clear any archived state so a reseed restores a clean board.
		record.Set(schema.FieldArchivedAt, "")
		record.Set(schema.FieldArchivedBy, "")
		if admin != nil {
			record.Set(schema.FieldCreatedBy, admin.Id)
		}

		if err := app.Save(record); err != nil {
			return nil, fmt.Errorf("save team %q: %w", spec.Name, err)
		}
		out[spec.Name] = record
	}

	return out, nil
}

func (s *Seeder) seedBoards(app core.App, teams map[string]*core.Record) (map[string]*core.Record, error) {
	collection, err := app.FindCollectionByNameOrId(schema.Boards)
	if err != nil {
		return nil, err
	}

	out := make(map[string]*core.Record)

	for _, spec := range demoBoards() {
		team, ok := teams[spec.Team]
		if !ok {
			return nil, fmt.Errorf("board %q references unknown team %q", spec.Name, spec.Team)
		}

		record, err := findByFilter(app, schema.Boards,
			"team = {:team} && name = {:name}",
			dbx.Params{"team": team.Id, "name": spec.Name})
		if err != nil {
			return nil, err
		}
		if record == nil {
			record = core.NewRecord(collection)
			record.Set(schema.FieldTeam, team.Id)
			record.Set(schema.FieldName, spec.Name)
		}

		record.Set(schema.FieldDescription, spec.Description)
		record.Set(schema.FieldSortOrder, spec.SortOrder)
		record.Set(schema.FieldArchivedAt, "")
		record.Set(schema.FieldArchivedBy, "")
		record.Set(schema.FieldCreatedBy, team.GetString(schema.FieldCreatedBy))

		if err := app.Save(record); err != nil {
			return nil, fmt.Errorf("save board %q: %w", spec.Name, err)
		}
		out[spec.Name] = record
	}

	return out, nil
}

func (s *Seeder) seedCards(app core.App, boards map[string]*core.Record) ([]*core.Record, error) {
	collection, err := app.FindCollectionByNameOrId(schema.Cards)
	if err != nil {
		return nil, err
	}

	out := make([]*core.Record, 0, len(demoCards()))

	for _, spec := range demoCards() {
		board, ok := boards[spec.Board]
		if !ok {
			return nil, fmt.Errorf("card %q references unknown board %q", spec.Title, spec.Board)
		}

		record, err := findByFilter(app, schema.Cards,
			"board = {:board} && title = {:title}",
			dbx.Params{"board": board.Id, "title": spec.Title})
		if err != nil {
			return nil, err
		}
		if record == nil {
			record = core.NewRecord(collection)
			record.Set(schema.FieldBoard, board.Id)
			record.Set(schema.FieldTitle, spec.Title)
		}

		links, err := json.Marshal(orEmptyLinks(spec.Links))
		if err != nil {
			return nil, fmt.Errorf("encode links for %q: %w", spec.Title, err)
		}
		checklist, err := json.Marshal(orEmptyChecklist(spec.Checklist))
		if err != nil {
			return nil, fmt.Errorf("encode checklist for %q: %w", spec.Title, err)
		}

		// team mirrors the board, exactly as the card hook enforces for API writes.
		record.Set(schema.FieldTeam, board.GetString(schema.FieldTeam))
		record.Set(schema.FieldSummary, spec.Summary)
		record.Set(schema.FieldCadence, string(spec.Cadence))
		record.Set(schema.FieldInstructions, spec.Instructions)
		record.Set(schema.FieldLinks, string(links))
		record.Set(schema.FieldChecklist, string(checklist))
		record.Set(schema.FieldSortOrder, spec.SortOrder)
		record.Set(schema.FieldArchivedAt, "")
		record.Set(schema.FieldArchivedBy, "")
		record.Set(schema.FieldCreatedBy, board.GetString(schema.FieldCreatedBy))

		if err := app.Save(record); err != nil {
			return nil, fmt.Errorf("save card %q: %w", spec.Title, err)
		}
		if err := s.backdateCard(app, record); err != nil {
			return nil, err
		}
		out = append(out, record)
	}

	return out, nil
}

// backdateCard makes a seeded card predate the history invented for it.
//
// PocketBase stamps `created` with the real wall-clock time on every insert, so
// a freshly seeded card looks as if it was born moments ago. Rollup computation
// only counts cards created before a period ends, so without this every
// historical rollup would report total_cards = 0 (while still showing done and
// in-progress counts from the invented occurrences). Setting `created` to the
// start of the earliest invented period makes the card exist for the whole
// window. The one extra, oldest period that the rollup lookback covers
// (HistoryPeriods+1) therefore sees no cards and no occurrences, is Empty and is
// skipped, which is the intended behaviour.
//
// The autodate field overwrites any value set through Save, so the column is
// written directly, as testutil does. The in-memory record is updated too, so a
// later Save of this record cannot write the stale timestamp back. It runs on
// every seed (not just on create), which keeps re-runs idempotent and makes the
// result independent of when the database was first seeded.
func (s *Seeder) backdateCard(app core.App, card *core.Record) error {
	if s.opts.HistoryPeriods <= 0 {
		return nil
	}

	title := card.GetString(schema.FieldTitle)
	cadence := domain.Cadence(card.GetString(schema.FieldCadence))

	// ClosedBefore returns newest first, so the earliest period is the last one.
	periods, err := s.cal.ClosedBefore(cadence, s.opts.Now(), s.opts.HistoryPeriods)
	if err != nil {
		return fmt.Errorf("card %q: %w", title, err)
	}
	if len(periods) == 0 {
		return nil
	}

	created, err := types.ParseDateTime(periods[len(periods)-1].Start)
	if err != nil {
		return fmt.Errorf("card %q: %w", title, err)
	}

	_, err = app.DB().Update(schema.Cards,
		dbx.Params{schema.FieldCreated: created.String()},
		dbx.HashExp{"id": card.Id}).Execute()
	if err != nil {
		return fmt.Errorf("backdate card %q: %w", title, err)
	}
	card.Set(schema.FieldCreated, created)

	return nil
}

// ---------------------------------------------------------------------------
// History
// ---------------------------------------------------------------------------

// seedHistory invents completion history for closed periods.
//
// This exists so the reporting screens have something to show immediately. It
// writes occurrence rows directly rather than going through the occurrence
// service, because that service deliberately refuses to write to closed periods.
// Seeding is the one legitimate exception, and confining it here keeps that
// exception visible.
//
// The pattern is deterministic: each card's PRNG is seeded from its title, so the
// same card always gets the same history and reports are stable across runs.
func (s *Seeder) seedHistory(app core.App, cards []*core.Record, users map[string]*core.Record) (int, error) {
	collection, err := app.FindCollectionByNameOrId(schema.Occurrences)
	if err != nil {
		return 0, err
	}

	// Only real team members can plausibly have done the work.
	membersByTeam := make(map[string][]*core.Record)
	for _, spec := range demoTeams() {
		for _, email := range spec.MemberEmail {
			if user, ok := users[email]; ok {
				membersByTeam[spec.Name] = append(membersByTeam[spec.Name], user)
			}
		}
	}
	teamNameByID := make(map[string]string)
	for _, spec := range demoTeams() {
		team, err := findByFilter(app, schema.Teams, "name = {:name}", dbx.Params{"name": spec.Name})
		if err != nil {
			return 0, err
		}
		if team != nil {
			teamNameByID[team.Id] = spec.Name
		}
	}

	reliability := make(map[string]float64, len(demoCards()))
	for _, spec := range demoCards() {
		reliability[spec.Title] = spec.Reliability
	}

	now := s.opts.Now()
	written := 0

	for _, card := range cards {
		cadence := domain.Cadence(card.GetString(schema.FieldCadence))
		title := card.GetString(schema.FieldTitle)

		periods, err := s.cal.ClosedBefore(cadence, now, s.opts.HistoryPeriods)
		if err != nil {
			return written, fmt.Errorf("card %q: %w", title, err)
		}

		candidates := membersByTeam[teamNameByID[card.GetString(schema.FieldTeam)]]
		if len(candidates) == 0 {
			continue
		}

		rate, ok := reliability[title]
		if !ok {
			rate = 0.8
		}

		for _, period := range periods {
			// Seed the PRNG from the card *and the period*, so every decision below
			// is a pure function of that pair.
			//
			// A single PRNG advanced across the loop would not survive a re-run: the
			// first run draws extra values whenever it creates a record, so on the
			// second run the "already seeded, skip" path would consume fewer values,
			// the stream would drift out of step, and a different set of periods
			// would be chosen. That is exactly the non-idempotency this avoids.
			prng := rand.New(rand.NewPCG(hashSeed(title), hashSeed(period.Key)))

			roll := prng.Float64()

			// Above the reliability threshold the card was simply missed, which is
			// what leaves a gap and makes the not-started counts meaningful.
			if roll > rate {
				continue
			}

			// A small slice of the completed work is left in progress, so the board
			// and reports show all three states.
			leftInProgress := roll > rate-0.05

			starter := candidates[prng.IntN(len(candidates))]
			finisher := candidates[prng.IntN(len(candidates))]

			// Place the timestamps inside the period so they are consistent with the
			// period key they are filed under.
			span := period.End.Sub(period.Start)
			startedAt := period.Start.Add(time.Duration(prng.Float64() * float64(span) * 0.6))
			completedAt := startedAt.Add(time.Duration(prng.Float64() * float64(span) * 0.3))
			if !completedAt.Before(period.End) {
				completedAt = period.End.Add(-time.Minute)
			}

			existing, err := findByFilter(app, schema.Occurrences,
				"card = {:card} && period_key = {:key}",
				dbx.Params{"card": card.Id, "key": period.Key})
			if err != nil {
				return written, err
			}
			if existing != nil {
				continue // already seeded on a previous run
			}

			startedTS, err := types.ParseDateTime(startedAt)
			if err != nil {
				return written, err
			}

			record := core.NewRecord(collection)
			record.Set(schema.FieldCard, card.Id)
			record.Set(schema.FieldBoard, card.GetString(schema.FieldBoard))
			record.Set(schema.FieldTeam, card.GetString(schema.FieldTeam))
			record.Set(schema.FieldCadence, string(cadence))
			record.Set(schema.FieldPeriodKey, period.Key)
			record.Set(schema.FieldStartedBy, starter.Id)
			record.Set(schema.FieldStartedAt, startedTS)

			if leftInProgress {
				record.Set(schema.FieldStatus, string(domain.StatusInProgress))
			} else {
				completedTS, err := types.ParseDateTime(completedAt)
				if err != nil {
					return written, err
				}
				record.Set(schema.FieldStatus, string(domain.StatusDone))
				record.Set(schema.FieldCompletedBy, finisher.Id)
				record.Set(schema.FieldCompletedAt, completedTS)
			}

			if err := app.Save(record); err != nil {
				return written, fmt.Errorf("save occurrence for %q %s: %w", title, period.Key, err)
			}
			written++
		}
	}

	return written, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// findByFilter returns the first matching record, or nil when there is none.
func findByFilter(app core.App, collection, filter string, params dbx.Params) (*core.Record, error) {
	record, err := app.FindFirstRecordByFilter(collection, filter, params)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query %s: %w", collection, err)
	}
	return record, nil
}

// hashSeed derives a stable PRNG seed from a string.
func hashSeed(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

func orEmptyLinks(in []linkSpec) []linkSpec {
	if in == nil {
		return []linkSpec{}
	}
	return in
}

func orEmptyChecklist(in []checklistSpec) []checklistSpec {
	if in == nil {
		return []checklistSpec{}
	}
	return in
}
