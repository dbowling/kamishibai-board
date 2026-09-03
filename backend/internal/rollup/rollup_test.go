package rollup_test

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/dbowling/kamishibai/backend/internal/config"
	"github.com/dbowling/kamishibai/backend/internal/domain"
	"github.com/dbowling/kamishibai/backend/internal/occurrence"
	"github.com/dbowling/kamishibai/backend/internal/rollup"
	"github.com/dbowling/kamishibai/backend/internal/schema"
	"github.com/dbowling/kamishibai/backend/internal/testutil"
)

type fixture struct {
	app    *tests.TestApp
	cal    *domain.Calendar
	member *core.Record
	team   *core.Record
	board  *core.Record
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	app := testutil.NewApp(t)
	cal := config.Default().Calendar

	member := testutil.NewUser(t, app, "member@example.test", "Member", schema.RoleUser)
	team := testutil.NewTeam(t, app, "Platform", member)
	board := testutil.NewBoard(t, app, team, "Triage")

	return &fixture{app: app, cal: cal, member: member, team: team, board: board}
}

func (f *fixture) date(y int, m time.Month, d, hour int) time.Time {
	return time.Date(y, m, d, hour, 0, 0, 0, f.cal.Location())
}

// occurrenceSvc returns a service whose clock is pinned to ts.
func (f *fixture) occurrenceSvc(ts time.Time) *occurrence.Service {
	return occurrence.NewService(f.cal).WithClock(func() time.Time { return ts })
}

func (f *fixture) rollupSvc(ts time.Time) *rollup.Service {
	return rollup.NewService(f.cal).WithClock(func() time.Time { return ts })
}

func (f *fixture) period(t *testing.T, cadence domain.Cadence, ts time.Time) domain.Period {
	t.Helper()
	p, err := f.cal.At(cadence, ts)
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	return p
}

// ---------------------------------------------------------------------------
// Compute
// ---------------------------------------------------------------------------

// The not-started count is the number the lazy model cannot get by counting
// rows, so it is the one that matters most.
func TestComputeDerivesNotStartedFromMissingRows(t *testing.T) {
	f := newFixture(t)

	// Three daily cards, created before the period under test.
	created := f.date(2026, time.September, 1, 9)
	svc := f.occurrenceSvc(created)
	_ = svc

	a := testutil.NewCard(t, f.app, f.board, "Check alerts", domain.Daily)
	b := testutil.NewCard(t, f.app, f.board, "Verify backups", domain.Daily)
	testutil.NewCard(t, f.app, f.board, "Review tickets", domain.Daily)

	// On the day itself: one done, one started, one untouched.
	day := f.date(2026, time.September, 3, 10)
	dayOcc := f.occurrenceSvc(day)
	if _, err := dayOcc.Complete(f.app, a.Id, f.member, ""); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if _, err := dayOcc.Start(f.app, b.Id, f.member, ""); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Only two rows exist even though three cards were in play.
	if n := testutil.CountRecords(t, f.app, schema.Occurrences); n != 2 {
		t.Fatalf("occurrences holds %d rows, want 2", n)
	}

	period := f.period(t, domain.Daily, day)
	snap, err := f.rollupSvc(day.AddDate(0, 0, 1)).Compute(f.app, f.board, domain.Daily, period)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	if snap.TotalCards != 3 {
		t.Errorf("total = %d, want 3", snap.TotalCards)
	}
	if snap.Done != 1 {
		t.Errorf("done = %d, want 1", snap.Done)
	}
	if snap.InProgress != 1 {
		t.Errorf("in_progress = %d, want 1", snap.InProgress)
	}
	if snap.NotStarted != 1 {
		t.Errorf("not_started = %d, want 1 (derived, not counted)", snap.NotStarted)
	}
	if got, want := snap.CompletionRate, 1.0/3.0; !almostEqual(got, want) {
		t.Errorf("completion rate = %v, want %v", got, want)
	}
	if snap.Period.Key != "2026-09-03" {
		t.Errorf("period key = %q, want 2026-09-03", snap.Period.Key)
	}
}

func TestComputeAllDone(t *testing.T) {
	f := newFixture(t)

	a := testutil.NewCard(t, f.app, f.board, "A", domain.Daily)
	b := testutil.NewCard(t, f.app, f.board, "B", domain.Daily)

	day := f.date(2026, time.September, 3, 10)
	svc := f.occurrenceSvc(day)
	for _, card := range []*core.Record{a, b} {
		if _, err := svc.Complete(f.app, card.Id, f.member, ""); err != nil {
			t.Fatalf("Complete: %v", err)
		}
	}

	snap, err := f.rollupSvc(day).Compute(f.app, f.board, domain.Daily, f.period(t, domain.Daily, day))
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if snap.CompletionRate != 1.0 {
		t.Errorf("completion rate = %v, want 1.0", snap.CompletionRate)
	}
	if snap.NotStarted != 0 {
		t.Errorf("not_started = %d, want 0", snap.NotStarted)
	}
}

func TestComputeNothingDone(t *testing.T) {
	f := newFixture(t)
	testutil.NewCard(t, f.app, f.board, "A", domain.Daily)
	testutil.NewCard(t, f.app, f.board, "B", domain.Daily)

	day := f.date(2026, time.September, 3, 10)
	snap, err := f.rollupSvc(day).Compute(f.app, f.board, domain.Daily, f.period(t, domain.Daily, day))
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	if snap.TotalCards != 2 || snap.NotStarted != 2 {
		t.Errorf("total = %d, not_started = %d, want 2 and 2", snap.TotalCards, snap.NotStarted)
	}
	if snap.CompletionRate != 0 {
		t.Errorf("completion rate = %v, want 0", snap.CompletionRate)
	}
	if snap.Empty() {
		t.Error("a board with cards but no work done is not an empty snapshot")
	}
}

// A card only counts against periods it actually existed for, otherwise adding a
// card would retroactively make past months look worse.
func TestComputeExcludesCardsCreatedAfterThePeriod(t *testing.T) {
	f := newFixture(t)

	// The period under test is September 2026; the card is created in October.
	// Fixtures stamp `created` with the real wall clock, so simulate this by
	// evaluating an earlier period instead.
	testutil.NewCard(t, f.app, f.board, "Added later", domain.Monthly)

	past := f.date(2020, time.January, 15, 10)
	snap, err := f.rollupSvc(f.date(2026, time.September, 3, 10)).
		Compute(f.app, f.board, domain.Monthly, f.period(t, domain.Monthly, past))
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	if snap.TotalCards != 0 {
		t.Errorf("total = %d, want 0: the card did not exist in %s", snap.TotalCards, snap.Period.Key)
	}
	if !snap.Empty() {
		t.Error("a period predating every card should produce an empty snapshot")
	}
}

// Archiving a card stops it counting against the team in later periods, but must
// not erase it from periods it was active for.
func TestComputeExcludesCardsArchivedBeforeThePeriod(t *testing.T) {
	f := newFixture(t)

	card := testutil.NewCard(t, f.app, f.board, "Retired check", domain.Daily)
	admin := testutil.NewUser(t, f.app, "admin@example.test", "Admin", schema.RoleAdmin)
	testutil.Archive(t, f.app, card, admin)

	// The card is archived "now", so a period well in the future no longer counts
	// it.
	future := f.date(2030, time.June, 10, 10)
	snap, err := f.rollupSvc(future).Compute(f.app, f.board, domain.Daily, f.period(t, domain.Daily, future))
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if snap.TotalCards != 0 {
		t.Errorf("total = %d, want 0 for a period after the card was archived", snap.TotalCards)
	}
}

func TestComputeIsScopedToCadence(t *testing.T) {
	f := newFixture(t)

	daily := testutil.NewCard(t, f.app, f.board, "Daily thing", domain.Daily)
	testutil.NewCard(t, f.app, f.board, "Weekly thing", domain.Weekly)

	day := f.date(2026, time.September, 3, 10)
	if _, err := f.occurrenceSvc(day).Complete(f.app, daily.Id, f.member, ""); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	snap, err := f.rollupSvc(day).Compute(f.app, f.board, domain.Daily, f.period(t, domain.Daily, day))
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if snap.TotalCards != 1 {
		t.Errorf("total = %d, want 1: the weekly card belongs to a different cadence", snap.TotalCards)
	}
	if snap.Done != 1 {
		t.Errorf("done = %d, want 1", snap.Done)
	}
}

func TestComputeIsScopedToBoard(t *testing.T) {
	f := newFixture(t)

	other := testutil.NewBoard(t, f.app, f.team, "Security Triage")
	mine := testutil.NewCard(t, f.app, f.board, "Mine", domain.Daily)
	theirs := testutil.NewCard(t, f.app, other, "Theirs", domain.Daily)

	day := f.date(2026, time.September, 3, 10)
	svc := f.occurrenceSvc(day)
	for _, card := range []*core.Record{mine, theirs} {
		if _, err := svc.Complete(f.app, card.Id, f.member, ""); err != nil {
			t.Fatalf("Complete: %v", err)
		}
	}

	snap, err := f.rollupSvc(day).Compute(f.app, f.board, domain.Daily, f.period(t, domain.Daily, day))
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if snap.TotalCards != 1 || snap.Done != 1 {
		t.Errorf("total = %d, done = %d, want 1 and 1 (other boards excluded)", snap.TotalCards, snap.Done)
	}
	if snap.BoardID != f.board.Id {
		t.Errorf("board = %q, want %q", snap.BoardID, f.board.Id)
	}
	if snap.TeamID != f.team.Id {
		t.Errorf("team = %q, want %q", snap.TeamID, f.team.Id)
	}
}

// ---------------------------------------------------------------------------
// Persist and Run
// ---------------------------------------------------------------------------

func TestPersistStoresEveryCount(t *testing.T) {
	f := newFixture(t)
	card := testutil.NewCard(t, f.app, f.board, "A", domain.Daily)
	testutil.NewCard(t, f.app, f.board, "B", domain.Daily)

	day := f.date(2026, time.September, 3, 10)
	if _, err := f.occurrenceSvc(day).Complete(f.app, card.Id, f.member, ""); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	svc := f.rollupSvc(day.AddDate(0, 0, 1))
	period := f.period(t, domain.Daily, day)
	snap, err := svc.Compute(f.app, f.board, domain.Daily, period)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	created, err := svc.Persist(f.app, snap)
	if err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if !created {
		t.Error("first Persist should report a created row")
	}

	stored, err := svc.Find(f.app, f.board.Id, domain.Daily, period.Key)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if stored == nil {
		t.Fatal("no rollup row found after Persist")
	}

	checks := map[string]int{
		schema.FieldTotalCards:      2,
		schema.FieldDoneCount:       1,
		schema.FieldInProgressCount: 0,
		schema.FieldNotStartedCount: 1,
	}
	for field, want := range checks {
		if got := stored.GetInt(field); got != want {
			t.Errorf("%s = %d, want %d", field, got, want)
		}
	}
	if got := stored.GetFloat(schema.FieldCompletionRate); !almostEqual(got, 0.5) {
		t.Errorf("completion_rate = %v, want 0.5", got)
	}
	if stored.GetDateTime(schema.FieldPeriodStart).IsZero() || stored.GetDateTime(schema.FieldPeriodEnd).IsZero() {
		t.Error("period boundaries should be stored alongside the key")
	}
	if stored.GetDateTime(schema.FieldClosedAt).IsZero() {
		t.Error("closed_at should record when the snapshot was taken")
	}
}

// Re-running must converge rather than duplicate: this is what makes backfilling
// safe.
func TestPersistIsIdempotent(t *testing.T) {
	f := newFixture(t)
	card := testutil.NewCard(t, f.app, f.board, "A", domain.Daily)

	day := f.date(2026, time.September, 3, 10)
	if _, err := f.occurrenceSvc(day).Complete(f.app, card.Id, f.member, ""); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	svc := f.rollupSvc(day.AddDate(0, 0, 1))
	period := f.period(t, domain.Daily, day)
	snap, err := svc.Compute(f.app, f.board, domain.Daily, period)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	if _, err := svc.Persist(f.app, snap); err != nil {
		t.Fatalf("first Persist: %v", err)
	}
	created, err := svc.Persist(f.app, snap)
	if err != nil {
		t.Fatalf("second Persist: %v", err)
	}
	if created {
		t.Error("second Persist should update in place, not create")
	}
	if n := testutil.CountRecords(t, f.app, schema.Rollups); n != 1 {
		t.Errorf("rollups holds %d rows, want 1", n)
	}
}

func TestRunSnapshotsClosedPeriodsOnly(t *testing.T) {
	f := newFixture(t)
	card := testutil.NewCard(t, f.app, f.board, "Daily check", domain.Daily)

	// Complete the card on the 1st, 2nd and 3rd of September.
	for _, day := range []int{1, 2, 3} {
		ts := f.date(2026, time.September, day, 10)
		if _, err := f.occurrenceSvc(ts).Complete(f.app, card.Id, f.member, ""); err != nil {
			t.Fatalf("Complete on the %dth: %v", day, err)
		}
	}

	// Run on the 3rd. The 1st and 2nd have closed; the 3rd is still open.
	now := f.date(2026, time.September, 3, 14)
	report, err := f.rollupSvc(now).Run(f.app, 5)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Created == 0 {
		t.Fatal("Run created no snapshots")
	}

	svc := f.rollupSvc(now)
	for _, key := range []string{"2026-09-01", "2026-09-02"} {
		got, err := svc.Find(f.app, f.board.Id, domain.Daily, key)
		if err != nil {
			t.Fatalf("Find %s: %v", key, err)
		}
		if got == nil {
			t.Errorf("expected a snapshot for the closed period %s", key)
		}
	}

	// The period still in progress must not be frozen: work can still land in it.
	open, err := svc.Find(f.app, f.board.Id, domain.Daily, "2026-09-03")
	if err != nil {
		t.Fatalf("Find open period: %v", err)
	}
	if open != nil {
		t.Error("the in-progress period must not be snapshotted")
	}
}

// A board that only uses one cadence should not accumulate rows for the others.
func TestRunSkipsCadencesTheBoardDoesNotUse(t *testing.T) {
	f := newFixture(t)
	testutil.NewCard(t, f.app, f.board, "Daily check", domain.Daily)

	now := f.date(2026, time.September, 3, 14)
	report, err := f.rollupSvc(now).Run(f.app, 3)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Skipped == 0 {
		t.Error("expected the unused cadences to be skipped")
	}

	rows, err := f.app.FindAllRecords(schema.Rollups)
	if err != nil {
		t.Fatalf("list rollups: %v", err)
	}
	for _, row := range rows {
		if got := row.GetString(schema.FieldCadence); got != string(domain.Daily) {
			t.Errorf("found a %s rollup for a board that only has daily cards", got)
		}
	}
}

// The lookback window is what lets the job recover from downtime instead of
// leaving permanent holes.
func TestRunBackfillsAfterDowntime(t *testing.T) {
	f := newFixture(t)
	card := testutil.NewCard(t, f.app, f.board, "Daily check", domain.Daily)

	// Work happens every day for a week while the rollup job is not running.
	for day := 1; day <= 7; day++ {
		ts := f.date(2026, time.September, day, 10)
		if _, err := f.occurrenceSvc(ts).Complete(f.app, card.Id, f.member, ""); err != nil {
			t.Fatalf("Complete on the %dth: %v", day, err)
		}
	}

	// The job finally runs on the 8th with a lookback wide enough to catch up.
	now := f.date(2026, time.September, 8, 0)
	if _, err := f.rollupSvc(now).Run(f.app, 10); err != nil {
		t.Fatalf("Run: %v", err)
	}

	svc := f.rollupSvc(now)
	for day := 1; day <= 7; day++ {
		key := f.period(t, domain.Daily, f.date(2026, time.September, day, 10)).Key
		got, err := svc.Find(f.app, f.board.Id, domain.Daily, key)
		if err != nil {
			t.Fatalf("Find %s: %v", key, err)
		}
		if got == nil {
			t.Errorf("missing backfilled snapshot for %s", key)
		}
	}
}

func TestRunRejectsInvalidLookback(t *testing.T) {
	f := newFixture(t)
	if _, err := f.rollupSvc(time.Now()).Run(f.app, 0); err == nil {
		t.Error("expected an error for a zero lookback")
	}
}

func TestRunOnEmptyDatabase(t *testing.T) {
	f := newFixture(t)
	report, err := f.rollupSvc(f.date(2026, time.September, 3, 14)).Run(f.app, 3)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Created != 0 {
		t.Errorf("created %d snapshots for a board with no cards, want 0", report.Created)
	}
}

func almostEqual(a, b float64) bool {
	const epsilon = 1e-9
	d := a - b
	return d < epsilon && d > -epsilon
}
