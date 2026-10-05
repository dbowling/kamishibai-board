package rollup_test

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/dbowling/kamishibai/backend/internal/domain"
	"github.com/dbowling/kamishibai/backend/internal/occurrence"
	"github.com/dbowling/kamishibai/backend/internal/rollup"
	"github.com/dbowling/kamishibai/backend/internal/schema"
	"github.com/dbowling/kamishibai/backend/internal/testutil"
)

// Two teams share an instant but not a calendar. At 2026-10-05 20:00 in New York
// (09:00 on the 6th in Tokyo), Tokyo's 5 October has closed and New York's has
// not, so one run has to treat the two boards differently.
func TestRunUsesEachTeamsOwnCalendar(t *testing.T) {
	f := newFixture(t)

	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}

	nyTeam := testutil.NewTeamInZone(t, f.app, "New York", "America/New_York", f.member)
	tokyoTeam := testutil.NewTeamInZone(t, f.app, "Tokyo", "Asia/Tokyo", f.member)
	nyBoard := testutil.NewBoard(t, f.app, nyTeam, "NY board")
	tokyoBoard := testutil.NewBoard(t, f.app, tokyoTeam, "Tokyo board")
	nyCard := f.card(t, nyBoard, "NY card", domain.Daily)
	tokyoCard := f.card(t, tokyoBoard, "Tokyo card", domain.Daily)

	cals := domain.NewCalendars(f.cal)
	at := func(ts time.Time) *occurrence.Service {
		return occurrence.NewServiceWithCalendars(cals).WithClock(func() time.Time { return ts })
	}

	// Work recorded during each team's own 5 October (and NY's 4th, its latest
	// closed day).
	if _, err := at(time.Date(2026, 10, 5, 12, 0, 0, 0, tokyo)).Complete(f.app, tokyoCard.Id, f.member, ""); err != nil {
		t.Fatalf("Complete Tokyo: %v", err)
	}
	if _, err := at(time.Date(2026, 10, 4, 12, 0, 0, 0, ny)).Complete(f.app, nyCard.Id, f.member, ""); err != nil {
		t.Fatalf("Complete NY: %v", err)
	}

	now := time.Date(2026, 10, 5, 20, 0, 0, 0, ny)
	svc := rollup.NewServiceWithCalendars(cals).WithClock(func() time.Time { return now })
	if _, err := svc.Run(f.app, 1); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Tokyo: 5 October is closed, and its boundaries are Tokyo midnights.
	tokyoRow, err := svc.Find(f.app, tokyoBoard.Id, domain.Daily, "2026-10-05")
	if err != nil || tokyoRow == nil {
		t.Fatalf("no Tokyo snapshot for 2026-10-05 (err=%v)", err)
	}
	assertInstant(t, "Tokyo start", tokyoRow, schema.FieldPeriodStart, time.Date(2026, 10, 5, 0, 0, 0, 0, tokyo))
	assertInstant(t, "Tokyo end", tokyoRow, schema.FieldPeriodEnd, time.Date(2026, 10, 6, 0, 0, 0, 0, tokyo))

	// New York: 5 October is still open, so it must not be rolled up yet. Its
	// latest closed day is the 4th.
	if row, err := svc.Find(f.app, nyBoard.Id, domain.Daily, "2026-10-05"); err != nil || row != nil {
		t.Errorf("NY has a snapshot for the still-open 2026-10-05 (err=%v)", err)
	}
	nyRow, err := svc.Find(f.app, nyBoard.Id, domain.Daily, "2026-10-04")
	if err != nil || nyRow == nil {
		t.Fatalf("no NY snapshot for 2026-10-04 (err=%v)", err)
	}
	assertInstant(t, "NY start", nyRow, schema.FieldPeriodStart, time.Date(2026, 10, 4, 0, 0, 0, 0, ny))
	assertInstant(t, "NY end", nyRow, schema.FieldPeriodEnd, time.Date(2026, 10, 5, 0, 0, 0, 0, ny))
	if got := nyRow.GetInt(schema.FieldDoneCount); got != 1 {
		t.Errorf("NY done = %d, want 1", got)
	}
}

func assertInstant(t *testing.T, label string, rec *core.Record, field string, want time.Time) {
	t.Helper()
	if got := rec.GetDateTime(field).Time(); !got.Equal(want) {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}
