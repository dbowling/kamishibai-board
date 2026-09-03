package migrations_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/dbowling/kamishibai/backend/internal/domain"
	"github.com/dbowling/kamishibai/backend/internal/schema"
	"github.com/dbowling/kamishibai/backend/internal/testutil"
)

// The migrations are the only source of schema, so these tests assert the shape
// they produce rather than trusting that the code that reads it agrees.

func TestMigrationsCreateAllCollections(t *testing.T) {
	app := testutil.NewApp(t)

	for _, name := range []string{
		schema.Users,
		schema.Teams,
		schema.Boards,
		schema.Cards,
		schema.Occurrences,
		schema.Rollups,
	} {
		if _, err := app.FindCollectionByNameOrId(name); err != nil {
			t.Errorf("collection %q missing after migrations: %v", name, err)
		}
	}
}

func TestUsersCollectionExtended(t *testing.T) {
	app := testutil.NewApp(t)
	users := testutil.Collection(t, app, schema.Users)

	role, ok := users.Fields.GetByName(schema.FieldRole).(*core.SelectField)
	if !ok {
		t.Fatalf("users.%s is not a select field", schema.FieldRole)
	}
	if !slices.Equal(role.Values, schema.RoleValues()) {
		t.Errorf("role values = %v, want %v", role.Values, schema.RoleValues())
	}
	if role.MaxSelect != 1 {
		t.Errorf("role MaxSelect = %d, want 1 (a user holds exactly one role)", role.MaxSelect)
	}
	// Left non-required on purpose: an empty role must read as a regular user so
	// that every admin check fails closed.
	if role.Required {
		t.Error("role should not be required; an empty role must fail closed as a regular user")
	}

	// Teammate names have to be resolvable to render attribution on the board.
	assertRule(t, "users.List", users.ListRule, schema.Authenticated)
	assertRule(t, "users.View", users.ViewRule, schema.Authenticated)

	// No open signup, and no self-deletion (occurrences reference users for
	// attribution).
	assertRuleNil(t, "users.Create", users.CreateRule)
	assertRuleNil(t, "users.Delete", users.DeleteRule)
}

func TestArchiveNotDeleteIsEnforcedBySchema(t *testing.T) {
	app := testutil.NewApp(t)

	// A nil delete rule means "superusers only" in PocketBase, which is how the
	// archive-never-delete requirement is enforced at the API layer rather than
	// relying on the UI to avoid calling DELETE.
	for _, name := range []string{
		schema.Teams,
		schema.Boards,
		schema.Cards,
		schema.Occurrences,
		schema.Rollups,
	} {
		c := testutil.Collection(t, app, name)
		assertRuleNil(t, name+".Delete", c.DeleteRule)

		if name == schema.Occurrences || name == schema.Rollups {
			continue
		}
		// Everything archivable needs somewhere to record the archival.
		for _, field := range []string{schema.FieldArchivedAt, schema.FieldArchivedBy} {
			if c.Fields.GetByName(field) == nil {
				t.Errorf("%s is missing the %q field", name, field)
			}
		}
	}
}

func TestTeamScopedReadRules(t *testing.T) {
	app := testutil.NewApp(t)

	// teams matches on its own members list; the rest hop through their team
	// relation.
	teams := testutil.Collection(t, app, schema.Teams)
	assertRule(t, "teams.List", teams.ListRule, schema.OwnTeam)
	assertRule(t, "teams.View", teams.ViewRule, schema.OwnTeam)
	// Creating and renaming teams is an admin action.
	assertRule(t, "teams.Create", teams.CreateRule, schema.AdminOnly)
	assertRule(t, "teams.Update", teams.UpdateRule, schema.AdminOnly)

	for _, name := range []string{schema.Boards, schema.Cards, schema.Occurrences, schema.Rollups} {
		c := testutil.Collection(t, app, name)
		assertRule(t, name+".List", c.ListRule, schema.TeamMember)
		assertRule(t, name+".View", c.ViewRule, schema.TeamMember)
	}
}

func TestAnyTeamMemberCanCreateBoardsAndCards(t *testing.T) {
	app := testutil.NewApp(t)

	boards := testutil.Collection(t, app, schema.Boards)
	assertRule(t, "boards.Create", boards.CreateRule, schema.TeamMember)
	assertRule(t, "boards.Update", boards.UpdateRule, schema.TeamMember)

	// Cards authorise creation from the board, because their team is derived
	// server-side and so is not in the body the create rule sees.
	cards := testutil.Collection(t, app, schema.Cards)
	assertRule(t, "cards.Create", cards.CreateRule, schema.BoardTeamMember)
	assertRule(t, "cards.Update", cards.UpdateRule, schema.TeamMember)
}

// Occurrences and rollups are written only by server-side code. If a client
// could write them directly it could forge attribution, backdate a period key,
// or invent a completion statistic.
func TestOccurrencesAndRollupsAreServerWriteOnly(t *testing.T) {
	app := testutil.NewApp(t)

	for _, name := range []string{schema.Occurrences, schema.Rollups} {
		c := testutil.Collection(t, app, name)
		assertRuleNil(t, name+".Create", c.CreateRule)
		assertRuleNil(t, name+".Update", c.UpdateRule)
		assertRuleNil(t, name+".Delete", c.DeleteRule)
	}
}

func TestCardsSchema(t *testing.T) {
	app := testutil.NewApp(t)
	cards := testutil.Collection(t, app, schema.Cards)

	cadence, ok := cards.Fields.GetByName(schema.FieldCadence).(*core.SelectField)
	if !ok {
		t.Fatalf("cards.%s is not a select field", schema.FieldCadence)
	}
	if !slices.Equal(cadence.Values, domain.CadenceValues()) {
		t.Errorf("cadence values = %v, want %v", cadence.Values, domain.CadenceValues())
	}
	if !cadence.Required {
		t.Error("cadence must be required; a card with no cadence could never flip")
	}

	// Instructions carry links and steps, per the "Verify Backups" example.
	for _, field := range []string{
		schema.FieldTitle,
		schema.FieldInstructions,
		schema.FieldLinks,
		schema.FieldChecklist,
		schema.FieldBoard,
		schema.FieldTeam,
	} {
		if cards.Fields.GetByName(field) == nil {
			t.Errorf("cards is missing the %q field", field)
		}
	}

	// A card must never carry a status of its own; status is per-period and
	// lives in occurrences.
	for _, forbidden := range []string{schema.FieldStatus, schema.FieldPeriodKey} {
		if cards.Fields.GetByName(forbidden) != nil {
			t.Errorf("cards must not have a %q field; per-period state belongs in occurrences", forbidden)
		}
	}
}

// The unique (card, period_key) index is what makes the lazy write safe: two
// people pressing Start at the same instant cannot produce two rows.
func TestOccurrencesUniquePerCardPerPeriod(t *testing.T) {
	app := testutil.NewApp(t)
	occ := testutil.Collection(t, app, schema.Occurrences)

	idx := occ.GetIndex("idx_occurrences_card_period")
	if idx == "" {
		t.Fatal("missing idx_occurrences_card_period index")
	}
	upper := strings.ToUpper(idx)
	if !strings.Contains(upper, "UNIQUE") {
		t.Errorf("the card/period index must be UNIQUE, got: %s", idx)
	}
	if !strings.Contains(idx, schema.FieldCard) || !strings.Contains(idx, schema.FieldPeriodKey) {
		t.Errorf("index should cover card and period_key, got: %s", idx)
	}
}

func TestOccurrencesReportingIndexes(t *testing.T) {
	app := testutil.NewApp(t)
	occ := testutil.Collection(t, app, schema.Occurrences)

	// Board rendering and reporting both need to avoid a full scan of the only
	// table that grows without bound.
	for _, name := range []string{
		"idx_occurrences_board_period",
		"idx_occurrences_team_cadence_period",
	} {
		if occ.GetIndex(name) == "" {
			t.Errorf("missing index %q", name)
		}
	}
}

// not_started is synthetic: it is the absence of a row, so it must not be a
// storable value.
func TestOccurrenceStatusExcludesNotStarted(t *testing.T) {
	app := testutil.NewApp(t)
	occ := testutil.Collection(t, app, schema.Occurrences)

	status, ok := occ.Fields.GetByName(schema.FieldStatus).(*core.SelectField)
	if !ok {
		t.Fatalf("occurrences.%s is not a select field", schema.FieldStatus)
	}
	if !slices.Equal(status.Values, domain.PersistedStatusValues()) {
		t.Errorf("status values = %v, want %v", status.Values, domain.PersistedStatusValues())
	}
	if slices.Contains(status.Values, string(domain.StatusNotStarted)) {
		t.Error("not_started must not be a storable status; it is represented by the absence of a row")
	}
}

func TestOccurrencesTrackAttribution(t *testing.T) {
	app := testutil.NewApp(t)
	occ := testutil.Collection(t, app, schema.Occurrences)

	// "It tracks who started a task, and who completed it."
	for _, field := range []string{
		schema.FieldStartedBy,
		schema.FieldStartedAt,
		schema.FieldCompletedBy,
		schema.FieldCompletedAt,
	} {
		if occ.Fields.GetByName(field) == nil {
			t.Errorf("occurrences is missing the %q field", field)
		}
	}
}

func TestRollupsUniquePerBoardCadencePeriod(t *testing.T) {
	app := testutil.NewApp(t)
	rollups := testutil.Collection(t, app, schema.Rollups)

	idx := rollups.GetIndex("idx_rollups_board_cadence_period")
	if idx == "" {
		t.Fatal("missing idx_rollups_board_cadence_period index")
	}
	// Uniqueness is what lets the rollup job re-run over the same window and
	// update in place, which is how it backfills after downtime.
	if !strings.Contains(strings.ToUpper(idx), "UNIQUE") {
		t.Errorf("the rollup index must be UNIQUE, got: %s", idx)
	}

	for _, field := range []string{
		schema.FieldTotalCards,
		schema.FieldDoneCount,
		schema.FieldInProgressCount,
		schema.FieldNotStartedCount,
		schema.FieldCompletionRate,
		schema.FieldPeriodStart,
		schema.FieldPeriodEnd,
	} {
		if rollups.Fields.GetByName(field) == nil {
			t.Errorf("report_rollups is missing the %q field", field)
		}
	}
}

func TestUniqueNamesOnlyApplyToActiveRecords(t *testing.T) {
	app := testutil.NewApp(t)

	// Archiving a team or board should free its name for reuse, so the unique
	// indexes are partial.
	cases := map[string]string{
		schema.Teams:  "idx_teams_name_active",
		schema.Boards: "idx_boards_team_name_active",
	}
	for collection, indexName := range cases {
		c := testutil.Collection(t, app, collection)
		idx := c.GetIndex(indexName)
		if idx == "" {
			t.Errorf("%s: missing index %q", collection, indexName)
			continue
		}
		if !strings.Contains(strings.ToUpper(idx), "WHERE") {
			t.Errorf("%s: index %q should be partial so archived records do not hold their name: %s",
				collection, indexName, idx)
		}
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func assertRule(t *testing.T, label string, got *string, want string) {
	t.Helper()
	if got == nil {
		t.Errorf("%s rule is nil, want %q", label, want)
		return
	}
	if *got != want {
		t.Errorf("%s rule =\n  %q\nwant\n  %q", label, *got, want)
	}
}

func assertRuleNil(t *testing.T, label string, got *string) {
	t.Helper()
	if got != nil {
		t.Errorf("%s rule = %q, want nil (superusers only)", label, *got)
	}
}
