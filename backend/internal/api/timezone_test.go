package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/dbowling/kamishibai/backend/internal/domain"
	"github.com/dbowling/kamishibai/backend/internal/schema"
	"github.com/dbowling/kamishibai/backend/internal/testutil"
)

// Time zone behaviour over HTTP: who may set one, what is accepted, and which
// zone the read endpoints report and bucket by.

// tokyoBoard adds a Tokyo team, with the fixture's member on it, and a board.
func (f *fixture) tokyoBoard(t *testing.T) (*core.Record, *core.Record) {
	t.Helper()
	team := testutil.NewTeamInZone(t, f.app, "Tokyo", "Asia/Tokyo", f.member)
	return team, testutil.NewBoard(t, f.app, team, "Tokyo board")
}

// ---------------------------------------------------------------------------
// Validation and permissions
// ---------------------------------------------------------------------------

func TestTeamZoneIsValidatedOnCreateAndUpdate(t *testing.T) {
	f := newFixture(t)

	res := f.client.POST(t, "/api/collections/teams/records", f.adminToken,
		map[string]any{schema.FieldName: "Mars", schema.FieldTimezone: "Mars/Base"})
	if res.Status != http.StatusBadRequest {
		t.Errorf("create with a bad zone = %d, want 400: %s", res.Status, res.Body)
	}

	res = f.client.POST(t, "/api/collections/teams/records", f.adminToken,
		map[string]any{schema.FieldName: "Tokyo", schema.FieldTimezone: " Asia/Tokyo "})
	if res.Status != http.StatusOK {
		t.Fatalf("create with a good zone = %d, want 200: %s", res.Status, res.Body)
	}
	var created struct {
		Timezone string `json:"timezone"`
	}
	res.JSON(t, &created)
	if created.Timezone != "Asia/Tokyo" {
		t.Errorf("stored zone = %q, want it trimmed to Asia/Tokyo", created.Timezone)
	}

	url := "/api/collections/teams/records/" + f.team.Id
	res = f.client.PATCH(t, url, f.adminToken, map[string]any{schema.FieldTimezone: "Mars/Base"})
	if res.Status != http.StatusBadRequest {
		t.Errorf("update with a bad zone = %d, want 400: %s", res.Status, res.Body)
	}
	res = f.client.PATCH(t, url, f.adminToken, map[string]any{schema.FieldTimezone: "Local"})
	if res.Status != http.StatusBadRequest {
		t.Errorf("update with \"Local\" = %d, want 400: %s", res.Status, res.Body)
	}

	res = f.client.PATCH(t, url, f.adminToken, map[string]any{schema.FieldTimezone: "Europe/London"})
	if res.Status != http.StatusOK {
		t.Fatalf("update with a good zone = %d, want 200: %s", res.Status, res.Body)
	}

	// Clearing it hands the team back to the instance default.
	res = f.client.PATCH(t, url, f.adminToken, map[string]any{schema.FieldTimezone: ""})
	if res.Status != http.StatusOK {
		t.Fatalf("clearing the zone = %d, want 200: %s", res.Status, res.Body)
	}
}

// The zone decides when a team's periods roll over, so a team member (who can
// read the team) must not be able to change it.
func TestNonAdminCannotChangeATeamZone(t *testing.T) {
	f := newFixture(t)

	res := f.client.PATCH(t, "/api/collections/teams/records/"+f.team.Id, f.memberToken,
		map[string]any{schema.FieldTimezone: "Asia/Tokyo"})
	if res.Status == http.StatusOK {
		t.Fatalf("a member changed their team's zone (%d): %s", res.Status, res.Body)
	}

	team, err := f.app.FindRecordById(schema.Teams, f.team.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got := team.GetString(schema.FieldTimezone); got != "" {
		t.Errorf("team zone = %q after a refused update, want it unchanged (empty)", got)
	}
}

func TestUserCanSetOwnDisplayZone(t *testing.T) {
	f := newFixture(t)
	url := "/api/collections/users/records/" + f.member.Id

	res := f.client.PATCH(t, url, f.memberToken, map[string]any{schema.FieldTimezone: "Pacific/Auckland"})
	if res.Status != http.StatusOK {
		t.Fatalf("setting a valid zone = %d, want 200: %s", res.Status, res.Body)
	}
	user, err := f.app.FindRecordById(schema.Users, f.member.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got := user.GetString(schema.FieldTimezone); got != "Pacific/Auckland" {
		t.Errorf("stored zone = %q, want Pacific/Auckland", got)
	}

	res = f.client.PATCH(t, url, f.memberToken, map[string]any{schema.FieldTimezone: "Mars/Base"})
	if res.Status != http.StatusBadRequest {
		t.Errorf("setting an invalid zone = %d, want 400: %s", res.Status, res.Body)
	}

	// Somebody else's display zone is not theirs to set.
	res = f.client.PATCH(t, "/api/collections/users/records/"+f.otherUser.Id, f.memberToken,
		map[string]any{schema.FieldTimezone: "Asia/Tokyo"})
	if res.Status == http.StatusOK {
		t.Errorf("a user changed another user's zone (%d)", res.Status)
	}
}

// ---------------------------------------------------------------------------
// What the read endpoints report
// ---------------------------------------------------------------------------

func TestBoardEndpointsReportTheTeamsEffectiveZone(t *testing.T) {
	f := newFixture(t)
	_, tokyo := f.tokyoBoard(t)

	cases := []struct {
		name  string
		board *core.Record
		want  string
	}{
		{"team with its own zone", tokyo, "Asia/Tokyo"},
		{"team inheriting the default", f.board, domain.DefaultTimezone},
	}

	for _, tc := range cases {
		for _, path := range []string{"/state", "/report?cadence=daily", "/activity"} {
			res := f.client.GET(t, "/api/kamishibai/boards/"+tc.board.Id+path, f.memberToken)
			if res.Status != http.StatusOK {
				t.Fatalf("%s %s = %d: %s", tc.name, path, res.Status, res.Body)
			}
			var body struct {
				Timezone string `json:"timezone"`
			}
			res.JSON(t, &body)
			if body.Timezone != tc.want {
				t.Errorf("%s %s: timezone = %q, want %q", tc.name, path, body.Timezone, tc.want)
			}
		}
	}
}

func TestBoardStatePeriodsAreInTheTeamsZone(t *testing.T) {
	f := newFixture(t)
	_, tokyo := f.tokyoBoard(t)

	res := f.client.GET(t, "/api/kamishibai/boards/"+tokyo.Id+"/state", f.memberToken)
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d: %s", res.Status, res.Body)
	}
	var body struct {
		Periods map[string]struct {
			End string `json:"end"`
		} `json:"periods"`
	}
	res.JSON(t, &body)

	// The server clock is real here, so assert the property rather than a date:
	// every boundary is Tokyo local midnight, which carries a +09:00 offset.
	end, err := time.Parse(time.RFC3339, body.Periods["daily"].End)
	if err != nil {
		t.Fatalf("parse end: %v", err)
	}
	if _, offset := end.Zone(); offset != 9*60*60 {
		t.Errorf("daily period end offset = %ds, want +09:00", offset)
	}
	if end.Hour() != 0 || end.Minute() != 0 {
		t.Errorf("daily period end = %v, want local midnight", end)
	}
}

func TestCurrentPeriodsCanBeAskedForATeam(t *testing.T) {
	f := newFixture(t)
	tokyoTeam, _ := f.tokyoBoard(t)

	res := f.client.GET(t, "/api/kamishibai/periods/current?team="+tokyoTeam.Id, f.memberToken)
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d: %s", res.Status, res.Body)
	}
	var body struct {
		Timezone string `json:"timezone"`
	}
	res.JSON(t, &body)
	if body.Timezone != "Asia/Tokyo" {
		t.Errorf("timezone = %q, want Asia/Tokyo", body.Timezone)
	}

	// Without a team it is still the instance default.
	res = f.client.GET(t, "/api/kamishibai/periods/current", f.memberToken)
	res.JSON(t, &body)
	if body.Timezone != domain.DefaultTimezone {
		t.Errorf("no team: timezone = %q, want %q", body.Timezone, domain.DefaultTimezone)
	}

	// A team the caller cannot read looks like one that does not exist.
	res = f.client.GET(t, "/api/kamishibai/periods/current?team="+f.otherTeam.Id, f.memberToken)
	if res.Status != http.StatusNotFound {
		t.Errorf("another team's periods = %d, want 404", res.Status)
	}
	res = f.client.GET(t, "/api/kamishibai/periods/current?team=nosuchteam0000", f.memberToken)
	if res.Status != http.StatusNotFound {
		t.Errorf("unknown team = %d, want 404", res.Status)
	}
}

// ---------------------------------------------------------------------------
// Heatmap buckets
// ---------------------------------------------------------------------------

// completeAt writes a completed daily occurrence directly, so the completion
// instant is fixed instead of coming from the real clock.
func (f *fixture) completeAt(t *testing.T, board, card *core.Record, at time.Time) {
	t.Helper()

	collection, err := f.app.FindCollectionByNameOrId(schema.Occurrences)
	if err != nil {
		t.Fatal(err)
	}
	occ := core.NewRecord(collection)
	occ.Set(schema.FieldCard, card.Id)
	occ.Set(schema.FieldBoard, board.Id)
	occ.Set(schema.FieldTeam, board.GetString(schema.FieldTeam))
	occ.Set(schema.FieldCadence, string(domain.Daily))
	occ.Set(schema.FieldPeriodKey, "2026-10-05")
	occ.Set(schema.FieldStatus, string(domain.StatusDone))
	occ.Set(schema.FieldStartedBy, f.member.Id)
	occ.Set(schema.FieldStartedAt, at)
	occ.Set(schema.FieldCompletedBy, f.member.Id)
	occ.Set(schema.FieldCompletedAt, at)
	if err := f.app.Save(occ); err != nil {
		t.Fatalf("save occurrence: %v", err)
	}
}

func (f *fixture) activityDates(t *testing.T, board *core.Record) []string {
	t.Helper()

	res := f.client.GET(t, "/api/kamishibai/boards/"+board.Id+"/activity", f.memberToken)
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d: %s", res.Status, res.Body)
	}
	var body struct {
		Days []struct {
			Date string `json:"date"`
		} `json:"days"`
	}
	res.JSON(t, &body)

	dates := make([]string, 0, len(body.Days))
	for _, d := range body.Days {
		dates = append(dates, d.Date)
	}
	return dates
}

// 23:30 in New York on 5 October is 12:30 on the 6th in Tokyo, so one instant is
// a different heatmap day for each team.
func TestActivityBucketsByTheTeamsZone(t *testing.T) {
	f := newFixture(t)
	_, tokyo := f.tokyoBoard(t)
	tokyoCard := testutil.NewCard(t, f.app, tokyo, "Tokyo card", domain.Daily)

	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	instant := time.Date(2026, 10, 5, 23, 30, 0, 0, ny)

	f.completeAt(t, f.board, f.card, instant)
	f.completeAt(t, tokyo, tokyoCard, instant)

	if got := f.activityDates(t, f.board); len(got) != 1 || got[0] != "2026-10-05" {
		t.Errorf("New York day = %v, want [2026-10-05]", got)
	}
	if got := f.activityDates(t, tokyo); len(got) != 1 || got[0] != "2026-10-06" {
		t.Errorf("Tokyo day = %v, want [2026-10-06]", got)
	}
}
