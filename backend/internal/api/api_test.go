package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/dbowling/kamishibai/backend/internal/app"
	"github.com/dbowling/kamishibai/backend/internal/config"
	"github.com/dbowling/kamishibai/backend/internal/domain"
	"github.com/dbowling/kamishibai/backend/internal/schema"
	"github.com/dbowling/kamishibai/backend/internal/testutil"
)

// These tests drive the real HTTP surface: PocketBase's generated collection
// endpoints plus our custom routes, with the actual API rules and hooks in place.
// They are where the security claims are verified, rather than asserted.

type fixture struct {
	app    *tests.TestApp
	client *testutil.Client

	// Platform team
	member      *core.Record
	memberToken string
	team        *core.Record
	board       *core.Record
	card        *core.Record

	// A second, unrelated team
	otherUser      *core.Record
	otherUserToken string
	otherTeam      *core.Record
	otherBoard     *core.Record
	otherCard      *core.Record

	admin      *core.Record
	adminToken string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	testApp := testutil.NewApp(t)

	// Register the real hooks, routes and cron wiring.
	app.Setup(testApp, config.Default())

	member := testutil.NewUser(t, testApp, "member@example.test", "Member", schema.RoleUser)
	admin := testutil.NewUser(t, testApp, "admin@example.test", "Admin", schema.RoleAdmin)
	otherUser := testutil.NewUser(t, testApp, "other@example.test", "Other", schema.RoleUser)

	team := testutil.NewTeam(t, testApp, "Platform", member)
	board := testutil.NewBoard(t, testApp, team, "Triage")
	card := testutil.NewCard(t, testApp, board, "Verify Backups", domain.Daily)

	otherTeam := testutil.NewTeam(t, testApp, "Unrelated", otherUser)
	otherBoard := testutil.NewBoard(t, testApp, otherTeam, "Their Board")
	otherCard := testutil.NewCard(t, testApp, otherBoard, "Their Card", domain.Daily)

	return &fixture{
		app:            testApp,
		client:         testutil.NewClient(t, testApp),
		member:         member,
		memberToken:    testutil.Token(t, member),
		team:           team,
		board:          board,
		card:           card,
		otherUser:      otherUser,
		otherUserToken: testutil.Token(t, otherUser),
		otherTeam:      otherTeam,
		otherBoard:     otherBoard,
		otherCard:      otherCard,
		admin:          admin,
		adminToken:     testutil.Token(t, admin),
	}
}

// ---------------------------------------------------------------------------
// Authentication
// ---------------------------------------------------------------------------

func TestCustomEndpointsRequireAuth(t *testing.T) {
	f := newFixture(t)

	paths := []struct{ method, path string }{
		{http.MethodGet, "/api/kamishibai/periods/current"},
		{http.MethodGet, "/api/kamishibai/boards/" + f.board.Id + "/state"},
		{http.MethodGet, "/api/kamishibai/boards/" + f.board.Id + "/report?cadence=daily"},
		{http.MethodPost, "/api/kamishibai/cards/" + f.card.Id + "/start"},
		{http.MethodPost, "/api/kamishibai/cards/" + f.card.Id + "/complete"},
		{http.MethodPost, "/api/kamishibai/cards/" + f.card.Id + "/reopen"},
	}

	for _, tc := range paths {
		res := f.client.Do(t, tc.method, tc.path, "", nil)
		if res.Status != http.StatusUnauthorized {
			t.Errorf("%s %s as a guest = %d, want 401", tc.method, tc.path, res.Status)
		}
	}
}

// ---------------------------------------------------------------------------
// Periods
// ---------------------------------------------------------------------------

func TestCurrentPeriodsCoversEveryCadence(t *testing.T) {
	f := newFixture(t)

	res := f.client.GET(t, "/api/kamishibai/periods/current", f.memberToken)
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", res.Status, res.Body)
	}

	var body struct {
		Timezone string `json:"timezone"`
		Periods  map[string]struct {
			Key   string `json:"key"`
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"periods"`
	}
	res.JSON(t, &body)

	if body.Timezone != domain.DefaultTimezone {
		t.Errorf("timezone = %q, want %q", body.Timezone, domain.DefaultTimezone)
	}
	for _, cadence := range domain.Cadences() {
		p, ok := body.Periods[string(cadence)]
		if !ok {
			t.Errorf("no current period reported for %s", cadence)
			continue
		}
		if p.Key == "" || p.Start == "" || p.End == "" {
			t.Errorf("%s period is incomplete: %+v", cadence, p)
		}
		// The server is the source of truth, so the key must parse as that cadence.
		if err := config.Default().Calendar.ValidateKey(cadence, p.Key); err != nil {
			t.Errorf("%s period key %q is invalid: %v", cadence, p.Key, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Board state and tenant isolation
// ---------------------------------------------------------------------------

func TestBoardStateReturnsCardsAndLiveStatus(t *testing.T) {
	f := newFixture(t)

	res := f.client.GET(t, "/api/kamishibai/boards/"+f.board.Id+"/state", f.memberToken)
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", res.Status, res.Body)
	}

	var body struct {
		Board struct {
			ID string `json:"id"`
		} `json:"board"`
		Cards []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			State struct {
				Status    string `json:"status"`
				PeriodKey string `json:"periodKey"`
			} `json:"state"`
		} `json:"cards"`
		Summary struct {
			Total      int `json:"total"`
			NotStarted int `json:"notStarted"`
		} `json:"summary"`
	}
	res.JSON(t, &body)

	if body.Board.ID != f.board.Id {
		t.Errorf("board id = %q, want %q", body.Board.ID, f.board.Id)
	}
	if len(body.Cards) != 1 {
		t.Fatalf("got %d cards, want 1", len(body.Cards))
	}
	if body.Cards[0].State.Status != string(domain.StatusNotStarted) {
		t.Errorf("status = %q, want not_started", body.Cards[0].State.Status)
	}
	if body.Summary.Total != 1 || body.Summary.NotStarted != 1 {
		t.Errorf("summary = %+v, want 1 total and 1 not started", body.Summary)
	}
}

// Cross-tenant reads must fail, and must not confirm the board exists.
func TestBoardStateHidesOtherTeamsBoards(t *testing.T) {
	f := newFixture(t)

	res := f.client.GET(t, "/api/kamishibai/boards/"+f.otherBoard.Id+"/state", f.memberToken)
	if res.Status != http.StatusNotFound {
		t.Errorf("reading another team's board = %d, want 404: %s", res.Status, res.Body)
	}
}

func TestAdminCanReadAnyBoard(t *testing.T) {
	f := newFixture(t)

	// The admin is on neither member list.
	for _, board := range []*core.Record{f.board, f.otherBoard} {
		res := f.client.GET(t, "/api/kamishibai/boards/"+board.Id+"/state", f.adminToken)
		if res.Status != http.StatusOK {
			t.Errorf("admin reading board %q = %d, want 200: %s", board.Id, res.Status, res.Body)
		}
	}
}

// ---------------------------------------------------------------------------
// Recording work
// ---------------------------------------------------------------------------

func TestStartAndCompleteRecordAttribution(t *testing.T) {
	f := newFixture(t)

	res := f.client.POST(t, "/api/kamishibai/cards/"+f.card.Id+"/start", f.memberToken, nil)
	if res.Status != http.StatusOK {
		t.Fatalf("start: status = %d, want 200: %s", res.Status, res.Body)
	}

	var started struct {
		State struct {
			Status    string `json:"status"`
			StartedBy struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"startedBy"`
		} `json:"state"`
	}
	res.JSON(t, &started)

	if started.State.Status != string(domain.StatusInProgress) {
		t.Errorf("status = %q, want in_progress", started.State.Status)
	}
	// Attribution comes from the token, not from the request body.
	if started.State.StartedBy.ID != f.member.Id {
		t.Errorf("startedBy = %q, want %q", started.State.StartedBy.ID, f.member.Id)
	}
	if started.State.StartedBy.Name != "Member" {
		t.Errorf("startedBy name = %q, want %q", started.State.StartedBy.Name, "Member")
	}

	res = f.client.POST(t, "/api/kamishibai/cards/"+f.card.Id+"/complete", f.adminToken,
		map[string]string{"notes": "all green"})
	if res.Status != http.StatusOK {
		t.Fatalf("complete: status = %d, want 200: %s", res.Status, res.Body)
	}

	var completed struct {
		State struct {
			Status      string              `json:"status"`
			Notes       string              `json:"notes"`
			StartedBy   struct{ ID string } `json:"startedBy"`
			CompletedBy struct {
				ID string `json:"id"`
			} `json:"completedBy"`
		} `json:"state"`
	}
	res.JSON(t, &completed)

	if completed.State.Status != string(domain.StatusDone) {
		t.Errorf("status = %q, want done", completed.State.Status)
	}
	if completed.State.CompletedBy.ID != f.admin.Id {
		t.Errorf("completedBy = %q, want %q", completed.State.CompletedBy.ID, f.admin.Id)
	}
	if completed.State.Notes != "all green" {
		t.Errorf("notes = %q, want %q", completed.State.Notes, "all green")
	}
}

// A caller must not be able to nominate someone else, backdate the period, or set
// the status directly. All three are ignored because the endpoint takes only notes.
func TestMutationIgnoresClientSuppliedAttributionAndPeriod(t *testing.T) {
	f := newFixture(t)

	res := f.client.POST(t, "/api/kamishibai/cards/"+f.card.Id+"/complete", f.memberToken,
		map[string]any{
			"notes":        "legit",
			"completed_by": f.otherUser.Id,
			"completedBy":  f.otherUser.Id,
			"period_key":   "1999-01-01",
			"periodKey":    "1999-01-01",
			"status":       "not_started",
			"card":         f.otherCard.Id,
		})
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", res.Status, res.Body)
	}

	occurrences, err := f.app.FindAllRecords(schema.Occurrences)
	if err != nil {
		t.Fatalf("list occurrences: %v", err)
	}
	if len(occurrences) != 1 {
		t.Fatalf("got %d occurrences, want 1", len(occurrences))
	}

	occ := occurrences[0]
	if got := occ.GetString(schema.FieldCompletedBy); got != f.member.Id {
		t.Errorf("completed_by = %q, want the authenticated user %q", got, f.member.Id)
	}
	if got := occ.GetString(schema.FieldCard); got != f.card.Id {
		t.Errorf("card = %q, want %q; the path parameter must win", got, f.card.Id)
	}
	if got := occ.GetString(schema.FieldStatus); got != string(domain.StatusDone) {
		t.Errorf("status = %q, want done", got)
	}

	// The period must be today's, derived from the server clock.
	current, err := config.Default().Calendar.Current(domain.Daily)
	if err != nil {
		t.Fatalf("current period: %v", err)
	}
	if got := occ.GetString(schema.FieldPeriodKey); got != current.Key {
		t.Errorf("period_key = %q, want the server's current period %q", got, current.Key)
	}
}

func TestNonMemberCannotRecordWork(t *testing.T) {
	f := newFixture(t)

	res := f.client.POST(t, "/api/kamishibai/cards/"+f.card.Id+"/start", f.otherUserToken, nil)
	if res.Status != http.StatusForbidden {
		t.Errorf("start as non-member = %d, want 403: %s", res.Status, res.Body)
	}

	if n := testutil.CountRecords(t, f.app, schema.Occurrences); n != 0 {
		t.Errorf("a forbidden request wrote %d occurrence rows, want 0", n)
	}
}

func TestReopenFlow(t *testing.T) {
	f := newFixture(t)

	// Nothing recorded yet.
	res := f.client.POST(t, "/api/kamishibai/cards/"+f.card.Id+"/reopen", f.memberToken, nil)
	if res.Status != http.StatusBadRequest {
		t.Errorf("reopen with nothing recorded = %d, want 400", res.Status)
	}

	f.client.POST(t, "/api/kamishibai/cards/"+f.card.Id+"/complete", f.memberToken, nil)

	// Start on a completed card should be refused rather than silently discarding
	// the completion.
	res = f.client.POST(t, "/api/kamishibai/cards/"+f.card.Id+"/start", f.memberToken, nil)
	if res.Status != http.StatusBadRequest {
		t.Errorf("start on a completed card = %d, want 400: %s", res.Status, res.Body)
	}

	res = f.client.POST(t, "/api/kamishibai/cards/"+f.card.Id+"/reopen", f.memberToken, nil)
	if res.Status != http.StatusOK {
		t.Fatalf("reopen = %d, want 200: %s", res.Status, res.Body)
	}

	var body struct {
		State struct {
			Status      string    `json:"status"`
			CompletedBy *struct{} `json:"completedBy"`
		} `json:"state"`
	}
	res.JSON(t, &body)
	if body.State.Status != string(domain.StatusInProgress) {
		t.Errorf("status = %q, want in_progress", body.State.Status)
	}
	if body.State.CompletedBy != nil {
		t.Error("completedBy should be cleared after reopening")
	}
}

func TestMutationOnUnknownCard(t *testing.T) {
	f := newFixture(t)

	res := f.client.POST(t, "/api/kamishibai/cards/nonexistent1234/start", f.memberToken, nil)
	if res.Status != http.StatusNotFound {
		t.Errorf("status = %d, want 404: %s", res.Status, res.Body)
	}
}

// ---------------------------------------------------------------------------
// Occurrences are closed to direct client writes
// ---------------------------------------------------------------------------

// If this ever starts passing, attribution becomes forgeable.
func TestClientsCannotWriteOccurrencesDirectly(t *testing.T) {
	f := newFixture(t)

	payload := map[string]any{
		schema.FieldCard:      f.card.Id,
		schema.FieldBoard:     f.board.Id,
		schema.FieldTeam:      f.team.Id,
		schema.FieldCadence:   string(domain.Daily),
		schema.FieldPeriodKey: "2020-01-01",
		schema.FieldStatus:    string(domain.StatusDone),
		// Forged attribution.
		schema.FieldCompletedBy: f.otherUser.Id,
	}

	for _, token := range []string{f.memberToken, f.adminToken} {
		res := f.client.POST(t, "/api/collections/occurrences/records", token, payload)
		if res.Status == http.StatusOK || res.Status == http.StatusCreated {
			t.Errorf("direct occurrence write succeeded (%d); it must be server-only", res.Status)
		}
	}

	if n := testutil.CountRecords(t, f.app, schema.Occurrences); n != 0 {
		t.Errorf("occurrences table holds %d rows, want 0", n)
	}
}

func TestClientsCannotWriteRollupsDirectly(t *testing.T) {
	f := newFixture(t)

	res := f.client.POST(t, "/api/collections/report_rollups/records", f.adminToken, map[string]any{
		schema.FieldTeam:           f.team.Id,
		schema.FieldBoard:          f.board.Id,
		schema.FieldCadence:        string(domain.Daily),
		schema.FieldPeriodKey:      "2026-01-01",
		schema.FieldCompletionRate: 1.0,
	})
	if res.Status == http.StatusOK || res.Status == http.StatusCreated {
		t.Errorf("a client fabricated a completion statistic (%d)", res.Status)
	}
}

// ---------------------------------------------------------------------------
// Archive, never delete
// ---------------------------------------------------------------------------

func TestRecordsCannotBeDeleted(t *testing.T) {
	f := newFixture(t)

	targets := map[string]string{
		"cards":  f.card.Id,
		"boards": f.board.Id,
		"teams":  f.team.Id,
	}

	for collection, id := range targets {
		for _, token := range []string{f.memberToken, f.adminToken} {
			res := f.client.DELETE(t, fmt.Sprintf("/api/collections/%s/records/%s", collection, id), token)
			if res.Status == http.StatusNoContent || res.Status == http.StatusOK {
				t.Errorf("DELETE %s succeeded (%d); records must only ever be archived", collection, res.Status)
			}
		}
	}

	// Everything is still there.
	for collection, id := range targets {
		if _, err := f.app.FindRecordById(collection, id); err != nil {
			t.Errorf("%s record %s was deleted: %v", collection, id, err)
		}
	}
}

// Anyone on the team can archive; only an admin can bring it back.
func TestArchiveIsOpenButRestoreIsAdminOnly(t *testing.T) {
	f := newFixture(t)
	path := "/api/collections/cards/records/" + f.card.Id

	res := f.client.PATCH(t, path, f.memberToken, map[string]any{
		schema.FieldArchivedAt: "2026-09-03 12:00:00.000Z",
	})
	if res.Status != http.StatusOK {
		t.Fatalf("member archiving a card = %d, want 200: %s", res.Status, res.Body)
	}

	// archived_by is stamped by the server.
	card, err := f.app.FindRecordById(schema.Cards, f.card.Id)
	if err != nil {
		t.Fatalf("reload card: %v", err)
	}
	if got := card.GetString(schema.FieldArchivedBy); got != f.member.Id {
		t.Errorf("archived_by = %q, want %q", got, f.member.Id)
	}

	// A regular member cannot restore.
	res = f.client.PATCH(t, path, f.memberToken, map[string]any{schema.FieldArchivedAt: ""})
	if res.Status != http.StatusForbidden {
		t.Errorf("member restoring = %d, want 403: %s", res.Status, res.Body)
	}

	// An admin can.
	res = f.client.PATCH(t, path, f.adminToken, map[string]any{schema.FieldArchivedAt: ""})
	if res.Status != http.StatusOK {
		t.Fatalf("admin restoring = %d, want 200: %s", res.Status, res.Body)
	}

	card, err = f.app.FindRecordById(schema.Cards, f.card.Id)
	if err != nil {
		t.Fatalf("reload card: %v", err)
	}
	if !card.GetDateTime(schema.FieldArchivedAt).IsZero() {
		t.Error("the card should be restored")
	}
	if got := card.GetString(schema.FieldArchivedBy); got != "" {
		t.Errorf("archived_by = %q, want cleared after restore", got)
	}
}

// ---------------------------------------------------------------------------
// Hooks
// ---------------------------------------------------------------------------

// Users may edit their own profile, which must not become a route to admin.
func TestUsersCannotPromoteThemselves(t *testing.T) {
	f := newFixture(t)

	res := f.client.PATCH(t, "/api/collections/users/records/"+f.member.Id, f.memberToken,
		map[string]any{schema.FieldRole: schema.RoleAdmin})
	if res.Status != http.StatusForbidden {
		t.Errorf("self-promotion = %d, want 403: %s", res.Status, res.Body)
	}

	user, err := f.app.FindRecordById(schema.Users, f.member.Id)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if got := user.GetString(schema.FieldRole); got != schema.RoleUser {
		t.Errorf("role = %q, want %q", got, schema.RoleUser)
	}
}

func TestAdminCanChangeRoles(t *testing.T) {
	f := newFixture(t)

	res := f.client.PATCH(t, "/api/collections/users/records/"+f.member.Id, f.adminToken,
		map[string]any{schema.FieldRole: schema.RoleAdmin})
	if res.Status != http.StatusOK {
		t.Fatalf("admin changing a role = %d, want 200: %s", res.Status, res.Body)
	}
}

func TestUsersCanStillEditTheirOwnProfile(t *testing.T) {
	f := newFixture(t)

	res := f.client.PATCH(t, "/api/collections/users/records/"+f.member.Id, f.memberToken,
		map[string]any{schema.FieldName: "Renamed"})
	if res.Status != http.StatusOK {
		t.Fatalf("editing own name = %d, want 200: %s", res.Status, res.Body)
	}
}

// Any team member can create a card, and its team is derived from the board
// rather than trusted from the request.
func TestCardCreationDerivesTeamFromBoard(t *testing.T) {
	f := newFixture(t)

	res := f.client.POST(t, "/api/collections/cards/records", f.memberToken, map[string]any{
		schema.FieldBoard:   f.board.Id,
		schema.FieldTitle:   "Check certificates",
		schema.FieldCadence: string(domain.Weekly),
		// A forged team, pointing at a team the caller is not on.
		schema.FieldTeam: f.otherTeam.Id,
	})
	if res.Status != http.StatusOK && res.Status != http.StatusCreated {
		t.Fatalf("creating a card = %d, want success: %s", res.Status, res.Body)
	}

	created := res.Map(t)
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("no id in response: %s", res.Body)
	}

	card, err := f.app.FindRecordById(schema.Cards, id)
	if err != nil {
		t.Fatalf("reload card: %v", err)
	}
	if got := card.GetString(schema.FieldTeam); got != f.team.Id {
		t.Errorf("team = %q, want the board's team %q; the client value must be discarded", got, f.team.Id)
	}
	if got := card.GetString(schema.FieldCreatedBy); got != f.member.Id {
		t.Errorf("created_by = %q, want %q", got, f.member.Id)
	}
}

// A card created without a team in the body must still be authorised, because the
// team is the server's business to derive.
func TestCardCanBeCreatedWithoutSupplyingTeam(t *testing.T) {
	f := newFixture(t)

	res := f.client.POST(t, "/api/collections/cards/records", f.memberToken, map[string]any{
		schema.FieldBoard:   f.board.Id,
		schema.FieldTitle:   "No team supplied",
		schema.FieldCadence: string(domain.Daily),
	})
	if res.Status != http.StatusOK && res.Status != http.StatusCreated {
		t.Fatalf("creating a card without a team = %d, want success: %s", res.Status, res.Body)
	}

	created := res.Map(t)
	if got, _ := created[schema.FieldTeam].(string); got != f.team.Id {
		t.Errorf("team = %q, want the board's team %q", got, f.team.Id)
	}
}

// Members can create boards for their own team, but not for anybody else's.
func TestBoardCreationIsScopedToOwnTeams(t *testing.T) {
	f := newFixture(t)

	res := f.client.POST(t, "/api/collections/boards/records", f.memberToken, map[string]any{
		schema.FieldTeam: f.team.Id,
		schema.FieldName: "My New Board",
	})
	if res.Status != http.StatusOK && res.Status != http.StatusCreated {
		t.Fatalf("member creating a board on their own team = %d, want success: %s", res.Status, res.Body)
	}
	if got, _ := res.Map(t)[schema.FieldCreatedBy].(string); got != f.member.Id {
		t.Errorf("created_by = %q, want %q", got, f.member.Id)
	}

	res = f.client.POST(t, "/api/collections/boards/records", f.memberToken, map[string]any{
		schema.FieldTeam: f.otherTeam.Id,
		schema.FieldName: "Trespassing Board",
	})
	if res.Status == http.StatusOK || res.Status == http.StatusCreated {
		t.Errorf("member created a board on another team (%d)", res.Status)
	}
}

func TestCardCannotBeCreatedOnAnotherTeamsBoard(t *testing.T) {
	f := newFixture(t)

	res := f.client.POST(t, "/api/collections/cards/records", f.memberToken, map[string]any{
		schema.FieldBoard:   f.otherBoard.Id,
		schema.FieldTitle:   "Sneaky card",
		schema.FieldCadence: string(domain.Daily),
	})
	if res.Status == http.StatusOK || res.Status == http.StatusCreated {
		t.Errorf("created a card on another team's board (%d)", res.Status)
	}
}

func TestCardCannotBeMovedBetweenTeams(t *testing.T) {
	f := newFixture(t)

	// The admin can see both teams, so this is not blocked by visibility.
	res := f.client.PATCH(t, "/api/collections/cards/records/"+f.card.Id, f.adminToken,
		map[string]any{schema.FieldBoard: f.otherBoard.Id})
	if res.Status == http.StatusOK {
		t.Errorf("moved a card to another team's board (%d); its history would be misattributed", res.Status)
	}
}

func TestOnlyAdminsCanCreateTeams(t *testing.T) {
	f := newFixture(t)

	res := f.client.POST(t, "/api/collections/teams/records", f.memberToken,
		map[string]any{schema.FieldName: "Rogue Team"})
	if res.Status == http.StatusOK || res.Status == http.StatusCreated {
		t.Errorf("a regular user created a team (%d)", res.Status)
	}

	res = f.client.POST(t, "/api/collections/teams/records", f.adminToken,
		map[string]any{schema.FieldName: "Legit Team"})
	if res.Status != http.StatusOK && res.Status != http.StatusCreated {
		t.Errorf("an admin could not create a team (%d): %s", res.Status, res.Body)
	}
}

// Listing must be scoped to the caller's teams.
func TestListingIsScopedToVisibleTeams(t *testing.T) {
	f := newFixture(t)

	res := f.client.GET(t, "/api/collections/cards/records?perPage=100", f.memberToken)
	if res.Status != http.StatusOK {
		t.Fatalf("listing cards = %d, want 200: %s", res.Status, res.Body)
	}

	var body struct {
		Items []struct {
			ID   string `json:"id"`
			Team string `json:"team"`
		} `json:"items"`
	}
	res.JSON(t, &body)

	for _, item := range body.Items {
		if item.Team != f.team.Id {
			t.Errorf("card %s from team %s leaked into the member's listing", item.ID, item.Team)
		}
	}
	if len(body.Items) != 1 {
		t.Errorf("got %d cards, want only the member's own 1", len(body.Items))
	}
}

// ---------------------------------------------------------------------------
// Reporting
// ---------------------------------------------------------------------------

func TestBoardReport(t *testing.T) {
	f := newFixture(t)

	// Record some work so the current period has numbers.
	if res := f.client.POST(t, "/api/kamishibai/cards/"+f.card.Id+"/complete", f.memberToken, nil); res.Status != http.StatusOK {
		t.Fatalf("complete: %s", res.Body)
	}

	res := f.client.GET(t, "/api/kamishibai/boards/"+f.board.Id+"/report?cadence=daily&periods=5", f.memberToken)
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", res.Status, res.Body)
	}

	var body struct {
		Cadence string `json:"cadence"`
		Current *struct {
			Total          int     `json:"total"`
			Done           int     `json:"done"`
			CompletionRate float64 `json:"completionRate"`
			Source         string  `json:"source"`
		} `json:"current"`
		Cards []struct {
			CardID         string  `json:"cardId"`
			Expected       int     `json:"expected"`
			Done           int     `json:"done"`
			CompletionRate float64 `json:"completionRate"`
		} `json:"cards"`
		Totals struct {
			Expected int `json:"expected"`
			Done     int `json:"done"`
		} `json:"totals"`
	}
	res.JSON(t, &body)

	if body.Cadence != string(domain.Daily) {
		t.Errorf("cadence = %q, want daily", body.Cadence)
	}
	if body.Current == nil {
		t.Fatal("expected the in-progress period to be reported")
	}
	if body.Current.Done != 1 || body.Current.Total != 1 {
		t.Errorf("current = %+v, want 1 done of 1", body.Current)
	}
	// The open period has no snapshot, so it must be labelled as computed live.
	if body.Current.Source != "live" {
		t.Errorf("current source = %q, want live", body.Current.Source)
	}
	if len(body.Cards) != 1 {
		t.Fatalf("got %d card reports, want 1", len(body.Cards))
	}
	if body.Cards[0].Done != 1 {
		t.Errorf("card done = %d, want 1", body.Cards[0].Done)
	}
	if body.Totals.Done != 1 {
		t.Errorf("totals.done = %d, want 1", body.Totals.Done)
	}
}

func TestBoardReportRejectsBadCadence(t *testing.T) {
	f := newFixture(t)

	for _, query := range []string{"", "?cadence=", "?cadence=fortnightly"} {
		res := f.client.GET(t, "/api/kamishibai/boards/"+f.board.Id+"/report"+query, f.memberToken)
		if res.Status != http.StatusBadRequest {
			t.Errorf("report%q = %d, want 400", query, res.Status)
		}
	}
}

func TestBoardReportHidesOtherTeams(t *testing.T) {
	f := newFixture(t)

	res := f.client.GET(t, "/api/kamishibai/boards/"+f.otherBoard.Id+"/report?cadence=daily", f.memberToken)
	if res.Status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.Status)
	}
}
