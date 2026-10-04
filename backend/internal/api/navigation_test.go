package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/dbowling/kamishibai/backend/internal/domain"
	"github.com/dbowling/kamishibai/backend/internal/schema"
	"github.com/dbowling/kamishibai/backend/internal/testutil"
)

// These tests cover the admin-only sidebar management: the collection rules that
// now gate boards and teams, and the two custom endpoints for ordering and moving.

const (
	orderPath = "/api/kamishibai/navigation/order"
)

func movePath(boardID string) string {
	return "/api/kamishibai/boards/" + boardID + "/move"
}

func isSuccess(status int) bool { return status >= 200 && status < 300 }

func sortOrderOf(t *testing.T, f *fixture, collection, id string) int {
	t.Helper()
	rec, err := f.app.FindRecordById(collection, id)
	if err != nil {
		t.Fatalf("load %s/%s: %v", collection, id, err)
	}
	return rec.GetInt(schema.FieldSortOrder)
}

// ---------------------------------------------------------------------------
// Collection rules
// ---------------------------------------------------------------------------

func TestOnlyAdminsCanUpdateAndArchiveBoards(t *testing.T) {
	f := newFixture(t)
	path := "/api/collections/boards/records/" + f.board.Id

	// A member can still read it, but cannot rename or archive it.
	if res := f.client.GET(t, path, f.memberToken); res.Status != http.StatusOK {
		t.Fatalf("member viewing their board = %d, want 200", res.Status)
	}
	res := f.client.PATCH(t, path, f.memberToken, map[string]any{schema.FieldName: "Renamed"})
	if isSuccess(res.Status) {
		t.Errorf("member renamed a board (%d)", res.Status)
	}
	res = f.client.PATCH(t, path, f.memberToken, map[string]any{schema.FieldArchivedAt: "2026-09-03 12:00:00.000Z"})
	if isSuccess(res.Status) {
		t.Errorf("member archived a board (%d)", res.Status)
	}
	stored, _ := f.app.FindRecordById(schema.Boards, f.board.Id)
	if stored.GetString(schema.FieldName) != "Triage" || stored.GetDateTime(schema.FieldArchivedAt).Time().Unix() > 0 {
		t.Errorf("a refused member write changed the board: %v", stored.PublicExport())
	}

	// An admin can rename, archive, and restore.
	res = f.client.PATCH(t, path, f.adminToken, map[string]any{schema.FieldName: "Renamed"})
	if res.Status != http.StatusOK {
		t.Fatalf("admin renaming a board = %d: %s", res.Status, res.Body)
	}
	res = f.client.PATCH(t, path, f.adminToken, map[string]any{schema.FieldArchivedAt: "2026-09-03 12:00:00.000Z"})
	if res.Status != http.StatusOK {
		t.Fatalf("admin archiving a board = %d: %s", res.Status, res.Body)
	}
	if got, _ := res.Map(t)[schema.FieldArchivedBy].(string); got != f.admin.Id {
		t.Errorf("archived_by = %q, want %q", got, f.admin.Id)
	}
	res = f.client.PATCH(t, path, f.adminToken, map[string]any{schema.FieldArchivedAt: ""})
	if res.Status != http.StatusOK {
		t.Fatalf("admin restoring a board = %d: %s", res.Status, res.Body)
	}
}

func TestOnlyAdminsCanManageTeams(t *testing.T) {
	f := newFixture(t)
	path := "/api/collections/teams/records/" + f.team.Id

	// Member: cannot rename, archive or reorder.
	for name, body := range map[string]map[string]any{
		"rename":  {schema.FieldName: "Renamed"},
		"archive": {schema.FieldArchivedAt: "2026-09-03 12:00:00.000Z"},
		"reorder": {schema.FieldSortOrder: 7},
	} {
		if res := f.client.PATCH(t, path, f.memberToken, body); isSuccess(res.Status) {
			t.Errorf("member could %s a team (%d)", name, res.Status)
		}
	}

	// Admin: create, rename, archive, restore.
	res := f.client.POST(t, "/api/collections/teams/records", f.adminToken, map[string]any{
		schema.FieldName: "Fresh Team", schema.FieldSortOrder: 3,
	})
	if !isSuccess(res.Status) {
		t.Fatalf("admin creating a team = %d: %s", res.Status, res.Body)
	}
	id, _ := res.Map(t)["id"].(string)
	if got := sortOrderOf(t, f, schema.Teams, id); got != 3 {
		t.Errorf("sort_order = %d, want 3", got)
	}

	path = "/api/collections/teams/records/" + id
	res = f.client.PATCH(t, path, f.adminToken, map[string]any{schema.FieldName: "Renamed Team"})
	if res.Status != http.StatusOK {
		t.Fatalf("admin renaming a team = %d: %s", res.Status, res.Body)
	}
	res = f.client.PATCH(t, path, f.adminToken, map[string]any{schema.FieldArchivedAt: "2026-09-03 12:00:00.000Z"})
	if res.Status != http.StatusOK {
		t.Fatalf("admin archiving a team = %d: %s", res.Status, res.Body)
	}
	res = f.client.PATCH(t, path, f.adminToken, map[string]any{schema.FieldArchivedAt: ""})
	if res.Status != http.StatusOK {
		t.Fatalf("admin restoring a team = %d: %s", res.Status, res.Body)
	}
	team, _ := f.app.FindRecordById(schema.Teams, id)
	if !team.GetDateTime(schema.FieldArchivedAt).IsZero() {
		t.Error("team is still archived after restore")
	}
}

// ---------------------------------------------------------------------------
// POST /navigation/order
// ---------------------------------------------------------------------------

func TestOrderEndpointSetsSortOrders(t *testing.T) {
	f := newFixture(t)
	second := testutil.NewBoard(t, f.app, f.team, "Second")

	res := f.client.POST(t, orderPath, f.adminToken, map[string]any{
		"teams": []string{f.otherTeam.Id, f.team.Id},
		"boards": map[string][]string{
			f.team.Id: {second.Id, f.board.Id},
		},
	})
	if res.Status != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", res.Status, res.Body)
	}

	for id, want := range map[string]int{f.otherTeam.Id: 0, f.team.Id: 1} {
		if got := sortOrderOf(t, f, schema.Teams, id); got != want {
			t.Errorf("team %s sort_order = %d, want %d", id, got, want)
		}
	}
	for id, want := range map[string]int{second.Id: 0, f.board.Id: 1} {
		if got := sortOrderOf(t, f, schema.Boards, id); got != want {
			t.Errorf("board %s sort_order = %d, want %d", id, got, want)
		}
	}
}

func TestOrderEndpointKeysAreOptional(t *testing.T) {
	f := newFixture(t)

	for name, body := range map[string]map[string]any{
		"empty":      {},
		"teams only": {"teams": []string{f.team.Id}},
	} {
		if res := f.client.POST(t, orderPath, f.adminToken, body); res.Status != http.StatusNoContent {
			t.Errorf("%s: status = %d, want 204: %s", name, res.Status, res.Body)
		}
	}
}

// A board listed under a team it does not belong to is rejected, and because the
// request is transactional nothing earlier in it may have been written either.
func TestOrderEndpointRejectsABoardUnderTheWrongTeamAndWritesNothing(t *testing.T) {
	f := newFixture(t)
	second := testutil.NewBoard(t, f.app, f.team, "Second")

	res := f.client.POST(t, orderPath, f.adminToken, map[string]any{
		"teams": []string{f.otherTeam.Id, f.team.Id},
		"boards": map[string][]string{
			f.team.Id: {second.Id, f.otherBoard.Id},
		},
	})
	if res.Status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", res.Status, res.Body)
	}

	for _, check := range []struct{ collection, id string }{
		{schema.Teams, f.otherTeam.Id}, {schema.Teams, f.team.Id},
		{schema.Boards, second.Id}, {schema.Boards, f.otherBoard.Id},
	} {
		if got := sortOrderOf(t, f, check.collection, check.id); got != 0 {
			t.Errorf("%s/%s sort_order = %d after a rejected request, want 0", check.collection, check.id, got)
		}
	}
	other, _ := f.app.FindRecordById(schema.Boards, f.otherBoard.Id)
	if other.GetString(schema.FieldTeam) != f.otherTeam.Id {
		t.Error("the order endpoint moved a board between teams")
	}
}

func TestOrderEndpointRejectsUnknownIDs(t *testing.T) {
	f := newFixture(t)

	for name, body := range map[string]map[string]any{
		"unknown team in teams":     {"teams": []string{"nope"}},
		"unknown team key":          {"boards": map[string][]string{"nope": {f.board.Id}}},
		"unknown board":             {"boards": map[string][]string{f.team.Id: {"nope"}}},
		"duplicate team":            {"teams": []string{f.team.Id, f.team.Id}},
		"duplicate board in a list": {"boards": map[string][]string{f.team.Id: {f.board.Id, f.board.Id}}},
	} {
		if res := f.client.POST(t, orderPath, f.adminToken, body); res.Status != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400: %s", name, res.Status, res.Body)
		}
	}
}

func TestOrderEndpointIsAdminOnly(t *testing.T) {
	f := newFixture(t)
	body := map[string]any{"teams": []string{f.otherTeam.Id, f.team.Id}}

	if res := f.client.POST(t, orderPath, "", body); res.Status != http.StatusUnauthorized {
		t.Errorf("guest status = %d, want 401", res.Status)
	}
	if res := f.client.POST(t, orderPath, f.memberToken, body); res.Status != http.StatusForbidden {
		t.Errorf("member status = %d, want 403", res.Status)
	}
	if got := sortOrderOf(t, f, schema.Teams, f.team.Id); got != 0 {
		t.Errorf("a forbidden request wrote sort_order %d", got)
	}
}

// ---------------------------------------------------------------------------
// POST /boards/{id}/move
// ---------------------------------------------------------------------------

// moveFixture adds history to the Platform board so a move has something to carry.
type moveFixture struct {
	*fixture
	card2 *core.Record
}

func newMoveFixture(t *testing.T) *moveFixture {
	t.Helper()
	f := newFixture(t)
	card2 := testutil.NewCard(t, f.app, f.board, "Second Card", domain.Weekly)

	occurrences := testutil.Collection(t, f.app, schema.Occurrences)
	for i, key := range []string{"2026-01-01", "2026-01-02"} {
		rec := core.NewRecord(occurrences)
		card := f.card
		if i == 1 {
			card = card2
		}
		rec.Set(schema.FieldCard, card.Id)
		rec.Set(schema.FieldBoard, f.board.Id)
		rec.Set(schema.FieldTeam, f.team.Id)
		rec.Set(schema.FieldCadence, card.GetString(schema.FieldCadence))
		rec.Set(schema.FieldPeriodKey, key)
		rec.Set(schema.FieldStatus, string(domain.StatusDone))
		if err := f.app.Save(rec); err != nil {
			t.Fatalf("create occurrence: %v", err)
		}
	}

	rollups := testutil.Collection(t, f.app, schema.Rollups)
	rec := core.NewRecord(rollups)
	rec.Set(schema.FieldTeam, f.team.Id)
	rec.Set(schema.FieldBoard, f.board.Id)
	rec.Set(schema.FieldCadence, string(domain.Daily))
	rec.Set(schema.FieldPeriodKey, "2026-01-01")
	rec.Set(schema.FieldPeriodStart, types.NowDateTime())
	rec.Set(schema.FieldPeriodEnd, types.NowDateTime())
	rec.Set(schema.FieldClosedAt, types.NowDateTime())
	if err := f.app.Save(rec); err != nil {
		t.Fatalf("create rollup: %v", err)
	}

	return &moveFixture{fixture: f, card2: card2}
}

func countWhere(t *testing.T, f *fixture, collection, field, value string) int {
	t.Helper()
	records, err := f.app.FindAllRecords(collection)
	if err != nil {
		t.Fatalf("list %s: %v", collection, err)
	}
	n := 0
	for _, r := range records {
		if r.GetString(field) == value {
			n++
		}
	}
	return n
}

func TestMoveBoardMovesItsHistoryAndVisibility(t *testing.T) {
	f := newMoveFixture(t)
	existing := testutil.NewBoard(t, f.app, f.otherTeam, "Already There")
	existing.Set(schema.FieldSortOrder, 4)
	if err := f.app.Save(existing); err != nil {
		t.Fatal(err)
	}

	// Before: only the Platform member can see the board; the other user cannot.
	if res := f.client.GET(t, "/api/kamishibai/boards/"+f.board.Id+"/state", f.otherUserToken); res.Status != http.StatusNotFound {
		t.Fatalf("other user before the move = %d, want 404", res.Status)
	}

	res := f.client.POST(t, movePath(f.board.Id), f.adminToken, map[string]any{"team": f.otherTeam.Id})
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", res.Status, res.Body)
	}

	var body struct {
		BoardID string `json:"boardId"`
		TeamID  string `json:"teamId"`
		Moved   struct {
			Cards, Occurrences, Rollups int
		} `json:"moved"`
	}
	res.JSON(t, &body)
	if body.BoardID != f.board.Id || body.TeamID != f.otherTeam.Id {
		t.Errorf("response ids = %+v", body)
	}
	if body.Moved.Cards != 2 || body.Moved.Occurrences != 2 || body.Moved.Rollups != 1 {
		t.Errorf("moved counts = %+v, want 2 cards, 2 occurrences, 1 rollup", body.Moved)
	}

	board, _ := f.app.FindRecordById(schema.Boards, f.board.Id)
	if got := board.GetString(schema.FieldTeam); got != f.otherTeam.Id {
		t.Errorf("board team = %q, want %q", got, f.otherTeam.Id)
	}
	if got := board.GetInt(schema.FieldSortOrder); got != 5 {
		t.Errorf("board sort_order = %d, want max(4)+1 = 5", got)
	}
	for _, c := range []string{schema.Cards, schema.Occurrences, schema.Rollups} {
		if n := countWhere(t, f.fixture, c, schema.FieldTeam, f.team.Id); n != 0 && c != schema.Cards {
			t.Errorf("%d %s rows still point at the old team", n, c)
		}
	}
	// The old team keeps nothing of this board's, but its other records stay put.
	for _, card := range []*core.Record{f.card, f.card2} {
		got, _ := f.app.FindRecordById(schema.Cards, card.Id)
		if got.GetString(schema.FieldTeam) != f.otherTeam.Id {
			t.Errorf("card %s team = %q, want the new team", card.Id, got.GetString(schema.FieldTeam))
		}
	}
	if n := countWhere(t, f.fixture, schema.Boards, schema.FieldTeam, f.team.Id); n != 0 {
		t.Errorf("old team still owns %d boards", n)
	}

	// Visibility flipped, through the real rules.
	if res := f.client.GET(t, "/api/kamishibai/boards/"+f.board.Id+"/state", f.memberToken); res.Status != http.StatusNotFound {
		t.Errorf("old team's member after the move = %d, want 404", res.Status)
	}
	if res := f.client.GET(t, "/api/collections/boards/records/"+f.board.Id, f.memberToken); res.Status != http.StatusNotFound {
		t.Errorf("old team's member via the collection API = %d, want 404", res.Status)
	}
	if res := f.client.GET(t, "/api/kamishibai/boards/"+f.board.Id+"/state", f.otherUserToken); res.Status != http.StatusOK {
		t.Errorf("new team's member after the move = %d, want 200: %s", res.Status, res.Body)
	}
	res = f.client.GET(t, "/api/collections/occurrences/records?perPage=100", f.otherUserToken)
	if res.Status != http.StatusOK || strings.Count(string(res.Body), f.board.Id) != 2 {
		t.Errorf("new team's member should see the 2 moved occurrences: %d %s", res.Status, res.Body)
	}
}

func TestMoveBoardToAnEmptyTeamStartsAtOne(t *testing.T) {
	f := newFixture(t)
	empty := testutil.NewTeam(t, f.app, "Empty")

	res := f.client.POST(t, movePath(f.board.Id), f.adminToken, map[string]any{"team": empty.Id})
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d: %s", res.Status, res.Body)
	}
	if got := sortOrderOf(t, f, schema.Boards, f.board.Id); got != 1 {
		t.Errorf("sort_order = %d, want 1", got)
	}
}

func TestMoveBoardRejectsInvalidRequests(t *testing.T) {
	f := newMoveFixture(t)

	archivedTeam := testutil.NewTeam(t, f.app, "Archived Team")
	testutil.Archive(t, f.app, archivedTeam, f.admin)

	archivedBoard := testutil.NewBoard(t, f.app, f.team, "Archived Board")
	testutil.Archive(t, f.app, archivedBoard, f.admin)

	archivedSourceTeam := testutil.NewTeam(t, f.app, "Archived Source")
	sourceBoard := testutil.NewBoard(t, f.app, archivedSourceTeam, "Stuck Board")
	testutil.Archive(t, f.app, archivedSourceTeam, f.admin)

	// A same-named active board on the destination.
	testutil.NewBoard(t, f.app, f.otherTeam, f.board.GetString(schema.FieldName))

	cases := []struct {
		name    string
		board   string
		body    any
		message string
	}{
		{"missing team", f.board.Id, map[string]any{}, ""},
		{"empty team", f.board.Id, map[string]any{"team": ""}, ""},
		{"unknown team", f.board.Id, map[string]any{"team": "nope"}, ""},
		{"same team", f.board.Id, map[string]any{"team": f.team.Id}, ""},
		{"archived board", archivedBoard.Id, map[string]any{"team": f.otherTeam.Id}, ""},
		{"archived target team", f.board.Id, map[string]any{"team": archivedTeam.Id}, ""},
		{"archived source team", sourceBoard.Id, map[string]any{"team": f.otherTeam.Id}, ""},
		{"name clash", f.board.Id, map[string]any{"team": f.otherTeam.Id}, "A board with that name already exists on the target team."},
	}
	for _, tc := range cases {
		res := f.client.POST(t, movePath(tc.board), f.adminToken, tc.body)
		if res.Status != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400: %s", tc.name, res.Status, res.Body)
			continue
		}
		if tc.message != "" {
			if got, _ := res.Map(t)["message"].(string); got != tc.message {
				t.Errorf("%s: message = %q, want %q", tc.name, got, tc.message)
			}
		}
	}

	// Nothing moved.
	board, _ := f.app.FindRecordById(schema.Boards, f.board.Id)
	if board.GetString(schema.FieldTeam) != f.team.Id {
		t.Error("a rejected move changed the board's team")
	}
}

func TestMoveBoardNotFoundAndAuth(t *testing.T) {
	f := newMoveFixture(t)
	body := map[string]any{"team": f.otherTeam.Id}

	if res := f.client.POST(t, movePath("nope"), f.adminToken, body); res.Status != http.StatusNotFound {
		t.Errorf("unknown board = %d, want 404", res.Status)
	}
	if res := f.client.POST(t, movePath(f.board.Id), "", body); res.Status != http.StatusUnauthorized {
		t.Errorf("guest = %d, want 401", res.Status)
	}
	// Forbidden is decided before anything about the board is revealed.
	if res := f.client.POST(t, movePath(f.board.Id), f.memberToken, body); res.Status != http.StatusForbidden {
		t.Errorf("member = %d, want 403", res.Status)
	}
	if res := f.client.POST(t, movePath("nope"), f.memberToken, body); res.Status != http.StatusForbidden {
		t.Errorf("member with an unknown board = %d, want 403", res.Status)
	}

	board, _ := f.app.FindRecordById(schema.Boards, f.board.Id)
	if board.GetString(schema.FieldTeam) != f.team.Id {
		t.Error("a forbidden move changed the board's team")
	}
	if n := countWhere(t, f.fixture, schema.Occurrences, schema.FieldTeam, f.team.Id); n != 2 {
		t.Errorf("occurrences on the original team = %d, want 2", n)
	}
}

// The endpoint is the only door. A plain PATCH is refused for admins too, because
// it would move the board without its history.
func TestGenericPatchStillCannotMoveABoard(t *testing.T) {
	f := newMoveFixture(t)

	res := f.client.PATCH(t, "/api/collections/boards/records/"+f.board.Id, f.adminToken,
		map[string]any{schema.FieldTeam: f.otherTeam.Id})
	if res.Status != http.StatusBadRequest {
		t.Errorf("admin PATCH of board.team = %d, want 400: %s", res.Status, res.Body)
	}
	board, _ := f.app.FindRecordById(schema.Boards, f.board.Id)
	if board.GetString(schema.FieldTeam) != f.team.Id {
		t.Error("board team changed through the generic API")
	}
}
