package occurrence_test

import (
	"errors"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/dbowling/kamishibai/backend/internal/config"
	"github.com/dbowling/kamishibai/backend/internal/domain"
	"github.com/dbowling/kamishibai/backend/internal/occurrence"
	"github.com/dbowling/kamishibai/backend/internal/schema"
	"github.com/dbowling/kamishibai/backend/internal/testutil"
)

// fixture is a ready-to-use board with one member and one admin.
type fixture struct {
	app     *tests.TestApp
	svc     *occurrence.Service
	member  *core.Record
	admin   *core.Record
	autre   *core.Record // a user on no team at all
	team    *core.Record
	board   *core.Record
	now     time.Time
	nowFunc func() time.Time
}

// frozen is a fixed Thursday afternoon, used so period keys in assertions are
// stable. 2026-09-03 falls in ISO week 36.
func newFixture(t *testing.T) *fixture {
	t.Helper()

	app := testutil.NewApp(t)
	cal := config.Default().Calendar

	frozen := time.Date(2026, 9, 3, 14, 0, 0, 0, cal.Location())
	nowFunc := func() time.Time { return frozen }

	member := testutil.NewUser(t, app, "member@example.test", "Member", schema.RoleUser)
	admin := testutil.NewUser(t, app, "admin@example.test", "Admin", schema.RoleAdmin)
	autre := testutil.NewUser(t, app, "outsider@example.test", "Outsider", schema.RoleUser)
	team := testutil.NewTeam(t, app, "Platform", member)
	board := testutil.NewBoard(t, app, team, "Triage")

	return &fixture{
		app:     app,
		svc:     occurrence.NewService(cal).WithClock(nowFunc),
		member:  member,
		admin:   admin,
		autre:   autre,
		team:    team,
		board:   board,
		now:     frozen,
		nowFunc: nowFunc,
	}
}

// at returns a service whose clock is moved to the supplied time.
func (f *fixture) at(ts time.Time) *occurrence.Service {
	return f.svc.WithClock(func() time.Time { return ts })
}

func (f *fixture) card(t *testing.T, title string, cadence domain.Cadence) *core.Record {
	t.Helper()
	return testutil.NewCard(t, f.app, f.board, title, cadence)
}

// ---------------------------------------------------------------------------
// The lazy model
// ---------------------------------------------------------------------------

// The headline capacity property: a card nobody touches costs nothing.
func TestUntouchedCardStoresNothing(t *testing.T) {
	f := newFixture(t)
	card := f.card(t, "Verify Backups", domain.Daily)

	state, err := f.svc.StateOf(f.app, card)
	if err != nil {
		t.Fatalf("StateOf: %v", err)
	}

	if state.Status != domain.StatusNotStarted {
		t.Errorf("status = %q, want %q", state.Status, domain.StatusNotStarted)
	}
	if state.Occurrence != nil {
		t.Error("an untouched card should have no occurrence record")
	}
	if state.Period.Key != "2026-09-03" {
		t.Errorf("period key = %q, want 2026-09-03", state.Period.Key)
	}
	if n := testutil.CountRecords(t, f.app, schema.Occurrences); n != 0 {
		t.Errorf("occurrences table holds %d rows, want 0", n)
	}
}

// Reading state across many periods must not create rows either.
func TestReadingStateNeverWrites(t *testing.T) {
	f := newFixture(t)
	card := f.card(t, "Verify Backups", domain.Daily)

	// Read the card's state on 30 consecutive days.
	for day := 0; day < 30; day++ {
		svc := f.at(f.now.AddDate(0, 0, day))
		if _, err := svc.StateOf(f.app, card); err != nil {
			t.Fatalf("day %d: %v", day, err)
		}
	}

	if n := testutil.CountRecords(t, f.app, schema.Occurrences); n != 0 {
		t.Errorf("reading state 30 times created %d rows, want 0", n)
	}
}

// ---------------------------------------------------------------------------
// Attribution
// ---------------------------------------------------------------------------

func TestStartRecordsWhoStarted(t *testing.T) {
	f := newFixture(t)
	card := f.card(t, "Verify Backups", domain.Daily)

	occ, err := f.svc.Start(f.app, card.Id, f.member, "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if got := occ.GetString(schema.FieldStatus); got != string(domain.StatusInProgress) {
		t.Errorf("status = %q, want in_progress", got)
	}
	if got := occ.GetString(schema.FieldStartedBy); got != f.member.Id {
		t.Errorf("started_by = %q, want %q", got, f.member.Id)
	}
	if occ.GetDateTime(schema.FieldStartedAt).IsZero() {
		t.Error("started_at was not stamped")
	}
	if got := occ.GetString(schema.FieldCompletedBy); got != "" {
		t.Errorf("completed_by = %q, want empty", got)
	}

	// The denormalised columns must be derived from the card, never trusted from
	// a caller.
	if got := occ.GetString(schema.FieldBoard); got != f.board.Id {
		t.Errorf("board = %q, want %q", got, f.board.Id)
	}
	if got := occ.GetString(schema.FieldTeam); got != f.team.Id {
		t.Errorf("team = %q, want %q", got, f.team.Id)
	}
	if got := occ.GetString(schema.FieldCadence); got != string(domain.Daily) {
		t.Errorf("cadence = %q, want daily", got)
	}
	if got := occ.GetString(schema.FieldPeriodKey); got != "2026-09-03" {
		t.Errorf("period_key = %q, want 2026-09-03", got)
	}
}

func TestCompleteRecordsBothStarterAndCompleter(t *testing.T) {
	f := newFixture(t)
	card := f.card(t, "Verify Backups", domain.Daily)

	if _, err := f.svc.Start(f.app, card.Id, f.member, ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	occ, err := f.svc.Complete(f.app, card.Id, f.admin, "all green")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if got := occ.GetString(schema.FieldStatus); got != string(domain.StatusDone) {
		t.Errorf("status = %q, want done", got)
	}
	// The person who started it keeps that credit even though someone else
	// finished it.
	if got := occ.GetString(schema.FieldStartedBy); got != f.member.Id {
		t.Errorf("started_by = %q, want the original starter %q", got, f.member.Id)
	}
	if got := occ.GetString(schema.FieldCompletedBy); got != f.admin.Id {
		t.Errorf("completed_by = %q, want %q", got, f.admin.Id)
	}
	if occ.GetDateTime(schema.FieldCompletedAt).IsZero() {
		t.Error("completed_at was not stamped")
	}
	if got := occ.GetString(schema.FieldNotes); got != "all green" {
		t.Errorf("notes = %q, want %q", got, "all green")
	}

	// Still exactly one row for this card and period.
	if n := testutil.CountRecords(t, f.app, schema.Occurrences); n != 1 {
		t.Errorf("occurrences holds %d rows, want 1", n)
	}
}

// Completing straight from not-started is a normal path: many triage tasks are
// quick and nobody presses Start.
func TestCompleteWithoutStartingAttributesBoth(t *testing.T) {
	f := newFixture(t)
	card := f.card(t, "Verify Backups", domain.Daily)

	occ, err := f.svc.Complete(f.app, card.Id, f.member, "")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if got := occ.GetString(schema.FieldStartedBy); got != f.member.Id {
		t.Errorf("started_by = %q, want %q", got, f.member.Id)
	}
	if got := occ.GetString(schema.FieldCompletedBy); got != f.member.Id {
		t.Errorf("completed_by = %q, want %q", got, f.member.Id)
	}
	if occ.GetDateTime(schema.FieldStartedAt).IsZero() {
		t.Error("started_at should be inferred when completing directly")
	}
}

func TestStartIsIdempotentAndKeepsTheOriginalStarter(t *testing.T) {
	f := newFixture(t)
	card := f.card(t, "Verify Backups", domain.Daily)

	first, err := f.svc.Start(f.app, card.Id, f.member, "")
	if err != nil {
		t.Fatalf("first Start: %v", err)
	}
	second, err := f.svc.Start(f.app, card.Id, f.admin, "")
	if err != nil {
		t.Fatalf("second Start: %v", err)
	}

	if first.Id != second.Id {
		t.Errorf("a second Start created a new row (%q then %q)", first.Id, second.Id)
	}
	if got := second.GetString(schema.FieldStartedBy); got != f.member.Id {
		t.Errorf("started_by = %q, want the first starter %q", got, f.member.Id)
	}
	if n := testutil.CountRecords(t, f.app, schema.Occurrences); n != 1 {
		t.Errorf("occurrences holds %d rows, want 1", n)
	}
}

func TestCompleteIsIdempotent(t *testing.T) {
	f := newFixture(t)
	card := f.card(t, "Verify Backups", domain.Daily)

	first, err := f.svc.Complete(f.app, card.Id, f.member, "")
	if err != nil {
		t.Fatalf("first Complete: %v", err)
	}
	second, err := f.svc.Complete(f.app, card.Id, f.admin, "")
	if err != nil {
		t.Fatalf("second Complete: %v", err)
	}

	if first.Id != second.Id {
		t.Error("completing twice created a second row")
	}
	if got := second.GetString(schema.FieldCompletedBy); got != f.member.Id {
		t.Errorf("completed_by = %q, want the original completer %q", got, f.member.Id)
	}
}

func TestStartRejectedOnCompletedCard(t *testing.T) {
	f := newFixture(t)
	card := f.card(t, "Verify Backups", domain.Daily)

	if _, err := f.svc.Complete(f.app, card.Id, f.member, ""); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	_, err := f.svc.Start(f.app, card.Id, f.member, "")
	if !errors.Is(err, occurrence.ErrAlreadyComplete) {
		t.Errorf("Start on a completed card = %v, want ErrAlreadyComplete", err)
	}
}

func TestReopenClearsCompletionButKeepsStarter(t *testing.T) {
	f := newFixture(t)
	card := f.card(t, "Verify Backups", domain.Daily)

	if _, err := f.svc.Start(f.app, card.Id, f.member, ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := f.svc.Complete(f.app, card.Id, f.admin, ""); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	occ, err := f.svc.Reopen(f.app, card.Id, f.member, "false alarm")
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}

	if got := occ.GetString(schema.FieldStatus); got != string(domain.StatusInProgress) {
		t.Errorf("status = %q, want in_progress", got)
	}
	if got := occ.GetString(schema.FieldCompletedBy); got != "" {
		t.Errorf("completed_by = %q, want cleared", got)
	}
	if !occ.GetDateTime(schema.FieldCompletedAt).IsZero() {
		t.Error("completed_at should be cleared on reopen")
	}
	// Reopening must not erase who picked the work up.
	if got := occ.GetString(schema.FieldStartedBy); got != f.member.Id {
		t.Errorf("started_by = %q, want %q preserved", got, f.member.Id)
	}
}

func TestReopenWithNothingRecorded(t *testing.T) {
	f := newFixture(t)
	card := f.card(t, "Verify Backups", domain.Daily)

	_, err := f.svc.Reopen(f.app, card.Id, f.member, "")
	if !errors.Is(err, occurrence.ErrNothingToReopen) {
		t.Errorf("Reopen with no occurrence = %v, want ErrNothingToReopen", err)
	}
	if n := testutil.CountRecords(t, f.app, schema.Occurrences); n != 0 {
		t.Errorf("a failed Reopen wrote %d rows, want 0", n)
	}
}

// ---------------------------------------------------------------------------
// Flipping
// ---------------------------------------------------------------------------

// The behaviour the whole design exists for: when the period rolls over the card
// reads as not-started again, with no job having mutated anything, and the
// previous period's record is still there.
func TestCardFlipsWhenThePeriodRollsOver(t *testing.T) {
	f := newFixture(t)
	card := f.card(t, "Verify Backups", domain.Daily)

	if _, err := f.svc.Complete(f.app, card.Id, f.member, "checked"); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	// Same day: done.
	state, err := f.svc.StateOf(f.app, card)
	if err != nil {
		t.Fatalf("StateOf: %v", err)
	}
	if state.Status != domain.StatusDone {
		t.Fatalf("status today = %q, want done", state.Status)
	}

	// Next day, without anything having run in between.
	tomorrow := f.at(f.now.AddDate(0, 0, 1))
	state, err = tomorrow.StateOf(f.app, card)
	if err != nil {
		t.Fatalf("StateOf tomorrow: %v", err)
	}
	if state.Status != domain.StatusNotStarted {
		t.Errorf("status tomorrow = %q, want not_started", state.Status)
	}
	if state.Period.Key != "2026-09-04" {
		t.Errorf("period key tomorrow = %q, want 2026-09-04", state.Period.Key)
	}

	// Yesterday's completion is retained as history.
	if n := testutil.CountRecords(t, f.app, schema.Occurrences); n != 1 {
		t.Errorf("occurrences holds %d rows, want 1 (yesterday's record kept)", n)
	}
}

// On Monday both the daily and the weekly cards reset, while a monthly card
// mid-month does not.
func TestMondayResetsDailyAndWeeklyButNotMonthly(t *testing.T) {
	f := newFixture(t)

	daily := f.card(t, "Check alerts", domain.Daily)
	weekly := f.card(t, "Review capacity", domain.Weekly)
	monthly := f.card(t, "Rotate credentials", domain.Monthly)

	cal := config.Default().Calendar
	// Friday 2026-09-04 ... complete everything.
	friday := time.Date(2026, 9, 4, 10, 0, 0, 0, cal.Location())
	fridaySvc := f.at(friday)
	for _, card := range []*core.Record{daily, weekly, monthly} {
		if _, err := fridaySvc.Complete(f.app, card.Id, f.member, ""); err != nil {
			t.Fatalf("Complete %s: %v", card.GetString(schema.FieldTitle), err)
		}
	}

	// The following Monday, 2026-09-07.
	monday := time.Date(2026, 9, 7, 9, 0, 0, 0, cal.Location())
	mondaySvc := f.at(monday)

	states, err := mondaySvc.StatesFor(f.app, []*core.Record{daily, weekly, monthly})
	if err != nil {
		t.Fatalf("StatesFor: %v", err)
	}

	if got := states[daily.Id].Status; got != domain.StatusNotStarted {
		t.Errorf("daily on Monday = %q, want not_started", got)
	}
	if got := states[weekly.Id].Status; got != domain.StatusNotStarted {
		t.Errorf("weekly on Monday = %q, want not_started", got)
	}
	// Same calendar month, so the monthly card is still done.
	if got := states[monthly.Id].Status; got != domain.StatusDone {
		t.Errorf("monthly on Monday = %q, want done (still September)", got)
	}

	// Confirm the weekly key actually advanced a week.
	if got := states[weekly.Id].Period.Key; got != "2026-W37" {
		t.Errorf("weekly period key = %q, want 2026-W37", got)
	}
}

// StatesFor mixes cadences, so it must match each card against its own period
// key rather than any key in the batch.
func TestStatesForMatchesEachCardToItsOwnPeriod(t *testing.T) {
	f := newFixture(t)

	daily := f.card(t, "Check alerts", domain.Daily)
	weekly := f.card(t, "Review capacity", domain.Weekly)
	quarterly := f.card(t, "DR drill", domain.Quarterly)

	// Only the weekly card gets completed.
	if _, err := f.svc.Complete(f.app, weekly.Id, f.member, ""); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	states, err := f.svc.StatesFor(f.app, []*core.Record{daily, weekly, quarterly})
	if err != nil {
		t.Fatalf("StatesFor: %v", err)
	}

	if got := states[weekly.Id].Status; got != domain.StatusDone {
		t.Errorf("weekly = %q, want done", got)
	}
	if got := states[daily.Id].Status; got != domain.StatusNotStarted {
		t.Errorf("daily = %q, want not_started (the weekly row must not leak across)", got)
	}
	if got := states[quarterly.Id].Status; got != domain.StatusNotStarted {
		t.Errorf("quarterly = %q, want not_started", got)
	}

	// Each card reports its own cadence's key.
	wantKeys := map[string]string{
		daily.Id:     "2026-09-03",
		weekly.Id:    "2026-W36",
		quarterly.Id: "2026-Q3",
	}
	for cardID, want := range wantKeys {
		if got := states[cardID].Period.Key; got != want {
			t.Errorf("card %s period key = %q, want %q", cardID, got, want)
		}
	}
}

func TestStatesForEmptyInput(t *testing.T) {
	f := newFixture(t)
	states, err := f.svc.StatesFor(f.app, nil)
	if err != nil {
		t.Fatalf("StatesFor(nil): %v", err)
	}
	if len(states) != 0 {
		t.Errorf("expected no states, got %d", len(states))
	}
}

// ---------------------------------------------------------------------------
// Authorisation
// ---------------------------------------------------------------------------

func TestNonMemberCannotRecordWork(t *testing.T) {
	f := newFixture(t)
	card := f.card(t, "Verify Backups", domain.Daily)

	if _, err := f.svc.Start(f.app, card.Id, f.autre, ""); !errors.Is(err, occurrence.ErrForbidden) {
		t.Errorf("Start as a non-member = %v, want ErrForbidden", err)
	}
	if _, err := f.svc.Complete(f.app, card.Id, f.autre, ""); !errors.Is(err, occurrence.ErrForbidden) {
		t.Errorf("Complete as a non-member = %v, want ErrForbidden", err)
	}
	if n := testutil.CountRecords(t, f.app, schema.Occurrences); n != 0 {
		t.Errorf("a forbidden request wrote %d rows, want 0", n)
	}
}

// Admins are on every team implicitly, without appearing in any member list.
func TestAdminCanRecordWorkOnAnyTeam(t *testing.T) {
	f := newFixture(t)
	card := f.card(t, "Verify Backups", domain.Daily)

	if members := f.team.GetStringSlice(schema.FieldMembers); len(members) != 1 {
		t.Fatalf("fixture team should have exactly one member, got %d", len(members))
	}

	if _, err := f.svc.Complete(f.app, card.Id, f.admin, ""); err != nil {
		t.Errorf("admin should be able to complete any card: %v", err)
	}
}

func TestNilUserRejected(t *testing.T) {
	f := newFixture(t)
	card := f.card(t, "Verify Backups", domain.Daily)

	if _, err := f.svc.Start(f.app, card.Id, nil, ""); !errors.Is(err, occurrence.ErrForbidden) {
		t.Errorf("Start with no user = %v, want ErrForbidden", err)
	}
}

func TestUnknownCard(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Start(f.app, "does-not-exist", f.member, ""); !errors.Is(err, occurrence.ErrCardNotFound) {
		t.Errorf("Start on a missing card = %v, want ErrCardNotFound", err)
	}
}

// ---------------------------------------------------------------------------
// Archiving freezes work
// ---------------------------------------------------------------------------

func TestArchivedRecordsAreReadOnly(t *testing.T) {
	cases := []struct {
		name   string
		pick   func(f *fixture, card *core.Record) *core.Record
		reason string
	}{
		{"card", func(f *fixture, card *core.Record) *core.Record { return card }, "the card itself"},
		{"board", func(f *fixture, _ *core.Record) *core.Record { return f.board }, "the board above it"},
		{"team", func(f *fixture, _ *core.Record) *core.Record { return f.team }, "the team above it"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			card := f.card(t, "Verify Backups", domain.Daily)

			testutil.Archive(t, f.app, tc.pick(f, card), f.admin)

			_, err := f.svc.Complete(f.app, card.Id, f.member, "")
			if !errors.Is(err, occurrence.ErrArchived) {
				t.Errorf("archiving %s should freeze the card, got %v", tc.reason, err)
			}
			if n := testutil.CountRecords(t, f.app, schema.Occurrences); n != 0 {
				t.Errorf("wrote %d rows against an archived record, want 0", n)
			}
		})
	}
}

// Archived history stays readable, which is what makes reporting over archived
// boards possible.
func TestArchivedCardStillReportsItsState(t *testing.T) {
	f := newFixture(t)
	card := f.card(t, "Verify Backups", domain.Daily)

	if _, err := f.svc.Complete(f.app, card.Id, f.member, ""); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	testutil.Archive(t, f.app, card, f.admin)

	state, err := f.svc.StateOf(f.app, card)
	if err != nil {
		t.Fatalf("StateOf on an archived card: %v", err)
	}
	if state.Status != domain.StatusDone {
		t.Errorf("status = %q, want done to remain readable after archiving", state.Status)
	}
}

// ---------------------------------------------------------------------------
// Concurrency
// ---------------------------------------------------------------------------

// Racing writers must not produce duplicate rows for the same card and period.
// The unique index is the guard; the service retries the loser.
func TestConcurrentWritesProduceExactlyOneRow(t *testing.T) {
	f := newFixture(t)
	card := f.card(t, "Verify Backups", domain.Daily)

	const writers = 8
	errs := make(chan error, writers)

	for i := 0; i < writers; i++ {
		go func() {
			_, err := f.svc.Complete(f.app, card.Id, f.member, "")
			errs <- err
		}()
	}
	for i := 0; i < writers; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent Complete failed: %v", err)
		}
	}

	if n := testutil.CountRecords(t, f.app, schema.Occurrences); n != 1 {
		t.Errorf("occurrences holds %d rows, want exactly 1", n)
	}
}
