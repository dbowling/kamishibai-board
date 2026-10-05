package occurrence_test

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/dbowling/kamishibai/backend/internal/domain"
	"github.com/dbowling/kamishibai/backend/internal/schema"
	"github.com/dbowling/kamishibai/backend/internal/testutil"
)

// These tests are about teams in different timezones sharing one instance. The
// same instant is a different calendar day (and sometimes week) depending on whose
// clock you read, and each team's periods must follow its own.

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%q): %v", name, err)
	}
	return loc
}

// zonedBoards puts a New York team and a Tokyo team side by side, each with one
// card of the given cadence.
type zonedBoards struct {
	ny, tokyo         *core.Record
	nyCard, tokyoCard *core.Record
}

func (f *fixture) zoned(t *testing.T, cadence domain.Cadence) zonedBoards {
	t.Helper()

	nyTeam := testutil.NewTeamInZone(t, f.app, "New York", "America/New_York", f.member)
	tokyoTeam := testutil.NewTeamInZone(t, f.app, "Tokyo", "Asia/Tokyo", f.member)
	nyBoard := testutil.NewBoard(t, f.app, nyTeam, "NY board")
	tokyoBoard := testutil.NewBoard(t, f.app, tokyoTeam, "Tokyo board")

	return zonedBoards{
		ny:        nyBoard,
		tokyo:     tokyoBoard,
		nyCard:    testutil.NewCard(t, f.app, nyBoard, "NY card", cadence),
		tokyoCard: testutil.NewCard(t, f.app, tokyoBoard, "Tokyo card", cadence),
	}
}

// 2026-10-05 20:00 in New York (EDT, UTC-4) is 2026-10-06 09:00 in Tokyo, so the
// two teams are on different days at the very same instant.
func TestDailyPeriodFollowsTheTeamsOwnZone(t *testing.T) {
	f := newFixture(t)
	z := f.zoned(t, domain.Daily)

	instant := time.Date(2026, 10, 5, 20, 0, 0, 0, mustLoc(t, "America/New_York"))
	svc := f.at(instant)

	nyState, err := svc.StateOf(f.app, z.nyCard)
	if err != nil {
		t.Fatalf("StateOf NY: %v", err)
	}
	tokyoState, err := svc.StateOf(f.app, z.tokyoCard)
	if err != nil {
		t.Fatalf("StateOf Tokyo: %v", err)
	}
	if nyState.Period.Key != "2026-10-05" {
		t.Errorf("NY key = %q, want 2026-10-05", nyState.Period.Key)
	}
	if tokyoState.Period.Key != "2026-10-06" {
		t.Errorf("Tokyo key = %q, want 2026-10-06", tokyoState.Period.Key)
	}

	// Completing each files the row under that team's own key.
	for _, card := range []*core.Record{z.nyCard, z.tokyoCard} {
		if _, err := svc.Complete(f.app, card.Id, f.member, ""); err != nil {
			t.Fatalf("Complete %s: %v", card.GetString(schema.FieldTitle), err)
		}
	}
	if occ, _ := svc.Find(f.app, z.nyCard.Id, "2026-10-05"); occ == nil {
		t.Error("NY completion was not filed under 2026-10-05")
	}
	if occ, _ := svc.Find(f.app, z.tokyoCard.Id, "2026-10-06"); occ == nil {
		t.Error("Tokyo completion was not filed under 2026-10-06")
	}
	if n := testutil.CountRecords(t, f.app, schema.Occurrences); n != 2 {
		t.Errorf("occurrences holds %d rows, want 2", n)
	}
}

// Sunday 2026-10-11 20:00 in New York is Monday 2026-10-12 09:00 in Tokyo, which
// is the first morning of the next ISO week there.
func TestWeeklyRolloverDiffersBetweenZones(t *testing.T) {
	f := newFixture(t)
	z := f.zoned(t, domain.Weekly)

	svc := f.at(time.Date(2026, 10, 11, 20, 0, 0, 0, mustLoc(t, "America/New_York")))

	nyState, err := svc.StateOf(f.app, z.nyCard)
	if err != nil {
		t.Fatalf("StateOf NY: %v", err)
	}
	tokyoState, err := svc.StateOf(f.app, z.tokyoCard)
	if err != nil {
		t.Fatalf("StateOf Tokyo: %v", err)
	}
	if nyState.Period.Key != "2026-W41" {
		t.Errorf("NY key = %q, want 2026-W41", nyState.Period.Key)
	}
	if tokyoState.Period.Key != "2026-W42" {
		t.Errorf("Tokyo key = %q, want 2026-W42", tokyoState.Period.Key)
	}
}

// A board mixing zones never happens (a board has one team), but StatesFor takes
// any cards and must not assume they share one.
func TestStatesForResolvesEachCardsOwnTeam(t *testing.T) {
	f := newFixture(t)
	z := f.zoned(t, domain.Daily)

	svc := f.at(time.Date(2026, 10, 5, 20, 0, 0, 0, mustLoc(t, "America/New_York")))
	states, err := svc.StatesFor(f.app, []*core.Record{z.nyCard, z.tokyoCard})
	if err != nil {
		t.Fatalf("StatesFor: %v", err)
	}
	if got := states[z.nyCard.Id].Period.Key; got != "2026-10-05" {
		t.Errorf("NY key = %q, want 2026-10-05", got)
	}
	if got := states[z.tokyoCard.Id].Period.Key; got != "2026-10-06" {
		t.Errorf("Tokyo key = %q, want 2026-10-06", got)
	}
}

// UK clocks go back on 2026-10-25, so that day is 25 hours long. The boundaries
// must still be London midnights, not 24 hours apart.
func TestDSTDayInANonDefaultZoneKeepsLocalMidnights(t *testing.T) {
	f := newFixture(t)

	london := mustLoc(t, "Europe/London")
	team := testutil.NewTeamInZone(t, f.app, "London", "Europe/London", f.member)
	board := testutil.NewBoard(t, f.app, team, "London board")
	card := testutil.NewCard(t, f.app, board, "London card", domain.Daily)

	state, err := f.at(time.Date(2026, 10, 25, 12, 0, 0, 0, london)).StateOf(f.app, card)
	if err != nil {
		t.Fatalf("StateOf: %v", err)
	}

	wantStart := time.Date(2026, 10, 25, 0, 0, 0, 0, london) // BST, UTC+1
	wantEnd := time.Date(2026, 10, 26, 0, 0, 0, 0, london)   // GMT, UTC+0
	if state.Period.Key != "2026-10-25" {
		t.Errorf("key = %q, want 2026-10-25", state.Period.Key)
	}
	if !state.Period.Start.Equal(wantStart) {
		t.Errorf("start = %v, want %v", state.Period.Start, wantStart)
	}
	if !state.Period.End.Equal(wantEnd) {
		t.Errorf("end = %v, want %v", state.Period.End, wantEnd)
	}
	if got := state.Period.End.Sub(state.Period.Start); got != 25*time.Hour {
		t.Errorf("day length = %v, want 25h", got)
	}
}

// A stored zone that cannot be loaded must surface, not silently become the
// default: that would move the team's boundaries without anyone noticing.
func TestInvalidStoredTeamZoneIsAnError(t *testing.T) {
	f := newFixture(t)

	// Written straight through app.Save, bypassing the validating request hook.
	team := testutil.NewTeamInZone(t, f.app, "Broken", "Mars/Base", f.member)
	board := testutil.NewBoard(t, f.app, team, "Broken board")
	card := testutil.NewCard(t, f.app, board, "Broken card", domain.Daily)

	if _, err := f.svc.StateOf(f.app, card); err == nil {
		t.Error("StateOf succeeded for a team with an unloadable zone, want an error")
	}
	if _, err := f.svc.Complete(f.app, card.Id, f.member, ""); err == nil {
		t.Error("Complete succeeded for a team with an unloadable zone, want an error")
	}
	if n := testutil.CountRecords(t, f.app, schema.Occurrences); n != 0 {
		t.Errorf("occurrences holds %d rows, want 0", n)
	}
}

// An empty zone is the regression check for every existing team: it must resolve
// to exactly what the instance default does.
func TestEmptyTeamZoneMatchesTheInstanceDefault(t *testing.T) {
	f := newFixture(t)
	card := f.card(t, "Default card", domain.Daily)

	if got := f.team.GetString(schema.FieldTimezone); got != "" {
		t.Fatalf("fixture team zone = %q, want empty", got)
	}

	// 23:30 in New York on 3 September is already 4 September in UTC, so a wrong
	// fallback to UTC would show up here.
	late := time.Date(2026, 9, 3, 23, 30, 0, 0, f.svc.Calendar().Location())
	state, err := f.at(late).StateOf(f.app, card)
	if err != nil {
		t.Fatalf("StateOf: %v", err)
	}
	want, err := f.svc.Calendar().At(domain.Daily, late)
	if err != nil {
		t.Fatalf("At: %v", err)
	}
	if state.Period != want {
		t.Errorf("period = %v, want the default calendar's %v", state.Period, want)
	}
	if state.Period.Key != "2026-09-03" {
		t.Errorf("key = %q, want 2026-09-03", state.Period.Key)
	}
}
