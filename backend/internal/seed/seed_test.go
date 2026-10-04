package seed_test

import (
	"fmt"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/dbowling/kamishibai/backend/internal/config"
	"github.com/dbowling/kamishibai/backend/internal/domain"
	"github.com/dbowling/kamishibai/backend/internal/schema"
	"github.com/dbowling/kamishibai/backend/internal/seed"
	"github.com/dbowling/kamishibai/backend/internal/testutil"
)

// frozen keeps the seeded window stable so a period boundary cannot roll over
// mid-test and be mistaken for non-determinism.
func frozenClock() func() time.Time {
	cal := config.Default().Calendar
	ts := time.Date(2026, 9, 3, 14, 0, 0, 0, cal.Location())
	return func() time.Time { return ts }
}

func newSeeder(historyPeriods int) *seed.Seeder {
	return seed.New(config.Default().Calendar, seed.Options{
		HistoryPeriods: historyPeriods,
		Rollups:        true,
		Now:            frozenClock(),
	})
}

func TestSeedCreatesTheDataset(t *testing.T) {
	app := testutil.NewApp(t)

	result, err := newSeeder(10).Run(app)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	if result.Users == 0 || result.Teams == 0 || result.Boards == 0 || result.Cards == 0 {
		t.Fatalf("seed produced an incomplete dataset: %+v", result)
	}
	if result.Occurrences == 0 {
		t.Error("expected some generated history")
	}

	// Every cadence should be represented, so the board exercises all five reset
	// behaviours out of the box.
	seen := map[string]bool{}
	cards, err := app.FindAllRecords(schema.Cards)
	if err != nil {
		t.Fatalf("list cards: %v", err)
	}
	for _, card := range cards {
		seen[card.GetString(schema.FieldCadence)] = true
	}
	for _, cadence := range domain.Cadences() {
		if !seen[string(cadence)] {
			t.Errorf("no seeded card uses the %s cadence", cadence)
		}
	}
}

// The card from the brief, with its dashboard link and troubleshooting steps.
func TestSeedIncludesTheVerifyBackupsCard(t *testing.T) {
	app := testutil.NewApp(t)
	if _, err := newSeeder(0).Run(app); err != nil {
		t.Fatalf("seed: %v", err)
	}

	card, err := app.FindFirstRecordByData(schema.Cards, schema.FieldTitle, "Verify Backups")
	if err != nil {
		t.Fatalf("find Verify Backups card: %v", err)
	}

	if got := card.GetString(schema.FieldCadence); got != string(domain.Daily) {
		t.Errorf("cadence = %q, want daily", got)
	}
	if card.GetString(schema.FieldInstructions) == "" {
		t.Error("the card should carry instructions")
	}

	var links []struct {
		Label string `json:"label"`
		URL   string `json:"url"`
	}
	if err := card.UnmarshalJSONField(schema.FieldLinks, &links); err != nil {
		t.Fatalf("decode links: %v", err)
	}
	if len(links) == 0 {
		t.Fatal("the card should carry links")
	}
	foundGrafana := false
	for _, l := range links {
		if l.URL != "" && l.Label != "" && contains(l.URL, "grafana") {
			foundGrafana = true
		}
	}
	if !foundGrafana {
		t.Error("expected a Grafana dashboard link on the Verify Backups card")
	}

	var checklist []struct {
		Text string `json:"text"`
	}
	if err := card.UnmarshalJSONField(schema.FieldChecklist, &checklist); err != nil {
		t.Fatalf("decode checklist: %v", err)
	}
	if len(checklist) < 3 {
		t.Errorf("expected at least three troubleshooting steps, got %d", len(checklist))
	}
}

// The headline requirement for seeding: running it twice must leave the database
// exactly as one run would.
func TestSeedIsIdempotent(t *testing.T) {
	app := testutil.NewApp(t)
	seeder := newSeeder(15)

	if _, err := seeder.Run(app); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	first := snapshot(t, app)

	second, err := seeder.Run(app)
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	after := snapshot(t, app)

	if second.Occurrences != 0 {
		t.Errorf("the second run generated %d new occurrences, want 0", second.Occurrences)
	}

	for _, collection := range []string{
		schema.Users, schema.Teams, schema.Boards, schema.Cards, schema.Occurrences, schema.Rollups,
	} {
		if first.counts[collection] != after.counts[collection] {
			t.Errorf("%s row count changed from %d to %d on the second run",
				collection, first.counts[collection], after.counts[collection])
		}
	}

	// Not just the same number of rows, but the same rows.
	if len(first.occurrenceKeys) != len(after.occurrenceKeys) {
		t.Fatalf("occurrence set size changed: %d then %d",
			len(first.occurrenceKeys), len(after.occurrenceKeys))
	}
	for i := range first.occurrenceKeys {
		if first.occurrenceKeys[i] != after.occurrenceKeys[i] {
			t.Errorf("occurrence %d changed from %q to %q",
				i, first.occurrenceKeys[i], after.occurrenceKeys[i])
		}
	}
}

// Two independent databases seeded with the same options must come out identical,
// which is what makes seeded data usable as a test baseline.
func TestSeedIsDeterministicAcrossDatabases(t *testing.T) {
	appA := testutil.NewApp(t)
	appB := testutil.NewApp(t)

	if _, err := newSeeder(15).Run(appA); err != nil {
		t.Fatalf("seed A: %v", err)
	}
	if _, err := newSeeder(15).Run(appB); err != nil {
		t.Fatalf("seed B: %v", err)
	}

	a := snapshot(t, appA)
	b := snapshot(t, appB)

	if len(a.occurrenceKeys) != len(b.occurrenceKeys) {
		t.Fatalf("different history sizes: %d and %d", len(a.occurrenceKeys), len(b.occurrenceKeys))
	}
	for i := range a.occurrenceKeys {
		if a.occurrenceKeys[i] != b.occurrenceKeys[i] {
			t.Fatalf("history diverged at %d: %q and %q", i, a.occurrenceKeys[i], b.occurrenceKeys[i])
		}
	}
}

func TestSeedWithoutHistory(t *testing.T) {
	app := testutil.NewApp(t)

	result, err := newSeeder(0).Run(app)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if result.Occurrences != 0 {
		t.Errorf("generated %d occurrences with history disabled, want 0", result.Occurrences)
	}
	if n := testutil.CountRecords(t, app, schema.Cards); n == 0 {
		t.Error("cards should still be seeded without history")
	}
}

// Generated history must never be filed against a period that is still open, or
// the board would show work for "today" that nobody did.
func TestSeededHistoryOnlyCoversClosedPeriods(t *testing.T) {
	app := testutil.NewApp(t)
	if _, err := newSeeder(15).Run(app); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cal := config.Default().Calendar
	now := frozenClock()()

	occurrences, err := app.FindAllRecords(schema.Occurrences)
	if err != nil {
		t.Fatalf("list occurrences: %v", err)
	}
	if len(occurrences) == 0 {
		t.Fatal("no history was generated")
	}

	for _, occ := range occurrences {
		key := occ.GetString(schema.FieldPeriodKey)
		period, err := cal.FromKey(key)
		if err != nil {
			t.Errorf("occurrence %s has an unparseable period key %q: %v", occ.Id, key, err)
			continue
		}
		if !period.IsClosed(now) {
			t.Errorf("occurrence %s is filed against period %q, which is still open", occ.Id, key)
		}

		// The recorded cadence must match the key's own format.
		cadence := domain.Cadence(occ.GetString(schema.FieldCadence))
		if period.Cadence != cadence {
			t.Errorf("occurrence %s says cadence %q but its key %q is a %q key",
				occ.Id, cadence, key, period.Cadence)
		}
	}
}

// Denormalised columns are load-bearing for the access rules, so seeded rows must
// respect them too.
func TestSeededRowsHaveConsistentDenormalisedFields(t *testing.T) {
	app := testutil.NewApp(t)
	if _, err := newSeeder(10).Run(app); err != nil {
		t.Fatalf("seed: %v", err)
	}

	boards := map[string]*core.Record{}
	boardRecords, err := app.FindAllRecords(schema.Boards)
	if err != nil {
		t.Fatalf("list boards: %v", err)
	}
	for _, b := range boardRecords {
		boards[b.Id] = b
	}

	cards := map[string]*core.Record{}
	cardRecords, err := app.FindAllRecords(schema.Cards)
	if err != nil {
		t.Fatalf("list cards: %v", err)
	}
	for _, c := range cardRecords {
		cards[c.Id] = c

		board := boards[c.GetString(schema.FieldBoard)]
		if board == nil {
			t.Errorf("card %s points at a missing board", c.Id)
			continue
		}
		if c.GetString(schema.FieldTeam) != board.GetString(schema.FieldTeam) {
			t.Errorf("card %s team does not match its board's team", c.Id)
		}
	}

	occurrences, err := app.FindAllRecords(schema.Occurrences)
	if err != nil {
		t.Fatalf("list occurrences: %v", err)
	}
	for _, occ := range occurrences {
		card := cards[occ.GetString(schema.FieldCard)]
		if card == nil {
			t.Errorf("occurrence %s points at a missing card", occ.Id)
			continue
		}
		for _, field := range []string{schema.FieldBoard, schema.FieldTeam, schema.FieldCadence} {
			if occ.GetString(field) != card.GetString(field) {
				t.Errorf("occurrence %s %s = %q, want %q from its card",
					occ.Id, field, occ.GetString(field), card.GetString(field))
			}
		}

		// Attribution must be present on every recorded row.
		if occ.GetString(schema.FieldStartedBy) == "" {
			t.Errorf("occurrence %s has no started_by", occ.Id)
		}
		if domain.Status(occ.GetString(schema.FieldStatus)) == domain.StatusDone &&
			occ.GetString(schema.FieldCompletedBy) == "" {
			t.Errorf("completed occurrence %s has no completed_by", occ.Id)
		}
	}
}

func TestSeedProducesRollups(t *testing.T) {
	app := testutil.NewApp(t)
	result, err := newSeeder(15).Run(app)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	if result.Rollups.Created == 0 {
		t.Error("expected reporting snapshots to be created")
	}
	if n := testutil.CountRecords(t, app, schema.Rollups); n == 0 {
		t.Error("no rollup rows were written")
	}

	// Every stored snapshot must be internally coherent. In particular the
	// denominator must be non-zero: seeded cards have to predate the history
	// invented for them, or Compute (which only counts cards created before a
	// period ends) reports total_cards = 0 for every historical period.
	rollups, err := app.FindAllRecords(schema.Rollups)
	if err != nil {
		t.Fatalf("list rollups: %v", err)
	}
	for _, r := range rollups {
		total := r.GetInt(schema.FieldTotalCards)
		done := r.GetInt(schema.FieldDoneCount)
		inProgress := r.GetInt(schema.FieldInProgressCount)
		notStarted := r.GetInt(schema.FieldNotStartedCount)

		if total <= 0 {
			t.Errorf("rollup %s has total_cards = %d, want > 0", r.Id, total)
			continue
		}
		if done+inProgress+notStarted != total {
			t.Errorf("rollup %s: done %d + in_progress %d + not_started %d != total %d",
				r.Id, done, inProgress, notStarted, total)
		}
		want := float64(done) / float64(total)
		if got := r.GetFloat(schema.FieldCompletionRate); math.Abs(got-want) > 1e-6 {
			t.Errorf("rollup %s: completion_rate = %v, want %v (done/total)", r.Id, got, want)
		}
	}
}

// Cards must be backdated to before the history invented for them, so reports see
// them as existing during those periods.
func TestSeededCardsPredateTheirHistory(t *testing.T) {
	app := testutil.NewApp(t)
	const periods = 15
	if _, err := newSeeder(periods).Run(app); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cal := config.Default().Calendar
	now := frozenClock()()

	cards, err := app.FindAllRecords(schema.Cards)
	if err != nil {
		t.Fatalf("list cards: %v", err)
	}
	if len(cards) == 0 {
		t.Fatal("no cards were seeded")
	}
	for _, card := range cards {
		cadence := domain.Cadence(card.GetString(schema.FieldCadence))
		closed, err := cal.ClosedBefore(cadence, now, periods)
		if err != nil || len(closed) == 0 {
			t.Fatalf("closed periods for %s: %v", cadence, err)
		}
		earliest := closed[len(closed)-1] // ClosedBefore is newest first

		created := card.GetDateTime(schema.FieldCreated).Time()
		if created.After(earliest.Start) {
			t.Errorf("card %q created %s is after its earliest history period starts (%s)",
				card.GetString(schema.FieldTitle), created, earliest.Start)
		}
	}
}

func TestSeedResetRemovesOnlySeededRecords(t *testing.T) {
	app := testutil.NewApp(t)

	if _, err := newSeeder(5).Run(app); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// A team that the seeder does not own must survive a reset.
	outsider := testutil.NewUser(t, app, "real.person@example.com", "Real Person", schema.RoleUser)
	keep := testutil.NewTeam(t, app, "A Real Team", outsider)
	keepBoard := testutil.NewBoard(t, app, keep, "Real Board")

	resetSeeder := seed.New(config.Default().Calendar, seed.Options{
		Reset:          true,
		HistoryPeriods: 5,
		Rollups:        false,
		Now:            frozenClock(),
	})
	if _, err := resetSeeder.Run(app); err != nil {
		t.Fatalf("reset seed: %v", err)
	}

	if _, err := app.FindRecordById(schema.Teams, keep.Id); err != nil {
		t.Errorf("reset deleted a team it did not create: %v", err)
	}
	if _, err := app.FindRecordById(schema.Boards, keepBoard.Id); err != nil {
		t.Errorf("reset deleted a board it did not create: %v", err)
	}
	if _, err := app.FindRecordById(schema.Users, outsider.Id); err != nil {
		t.Errorf("reset deleted a user it did not create: %v", err)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

type dbSnapshot struct {
	counts         map[string]int
	occurrenceKeys []string
}

// snapshot captures row counts plus the exact set of card/period pairs, so tests
// can compare content rather than just totals.
func snapshot(t *testing.T, app core.App) dbSnapshot {
	t.Helper()

	out := dbSnapshot{counts: map[string]int{}}
	for _, collection := range []string{
		schema.Users, schema.Teams, schema.Boards, schema.Cards, schema.Occurrences, schema.Rollups,
	} {
		n, err := app.CountRecords(collection)
		if err != nil {
			t.Fatalf("count %s: %v", collection, err)
		}
		out.counts[collection] = int(n)
	}

	occurrences, err := app.FindAllRecords(schema.Occurrences)
	if err != nil {
		t.Fatalf("list occurrences: %v", err)
	}

	// Key on card title rather than record id so two independently seeded
	// databases are comparable.
	for _, occ := range occurrences {
		card, err := app.FindRecordById(schema.Cards, occ.GetString(schema.FieldCard))
		if err != nil {
			t.Fatalf("load card for occurrence %s: %v", occ.Id, err)
		}
		out.occurrenceKeys = append(out.occurrenceKeys, fmt.Sprintf("%s|%s|%s",
			card.GetString(schema.FieldTitle),
			occ.GetString(schema.FieldPeriodKey),
			occ.GetString(schema.FieldStatus)))
	}
	sort.Strings(out.occurrenceKeys)

	return out
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if equalFold(haystack[i:i+len(needle)], needle) {
			return true
		}
	}
	return false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
