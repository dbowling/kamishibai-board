package api_test

import (
	"testing"

	"github.com/pocketbase/dbx"

	"github.com/dbowling/kamishibai/backend/internal/schema"
	"github.com/dbowling/kamishibai/backend/internal/testutil"
)

// The access rules are filter expressions evaluated by the database, so a typo in
// one is not a compile error and not a runtime error either: it just quietly
// matches nothing and looks identical to a legitimate denial.
//
// These tests assert the rules actually resolve, which is the thing that broke
// when `team.members ?= x` was used instead of `team.members.id ?= x`.

// resolvesTo runs a rule-shaped filter and reports how many records matched.
func resolvesTo(t *testing.T, f *fixture, collection, filter string, params dbx.Params) int {
	t.Helper()

	records, err := f.app.FindRecordsByFilter(collection, filter, "", 0, 0, params)
	if err != nil {
		t.Fatalf("filter %q on %s: %v", filter, collection, err)
	}
	return len(records)
}

// membershipFilter strips the auth placeholder out of a rule so it can be run as
// a plain filter with a bound parameter.
const (
	teamMemberFilter = `team.members.id ?= {:user}`
	ownTeamFilter    = `members.id ?= {:user}`
)

func TestMembershipFilterMatchesMembers(t *testing.T) {
	f := newFixture(t)
	params := dbx.Params{"user": f.member.Id}

	if got := resolvesTo(t, f, schema.Teams, ownTeamFilter, params); got != 1 {
		t.Errorf("teams matching the member = %d, want 1", got)
	}
	if got := resolvesTo(t, f, schema.Boards, teamMemberFilter, params); got != 1 {
		t.Errorf("boards visible to the member = %d, want 1", got)
	}
	if got := resolvesTo(t, f, schema.Cards, teamMemberFilter, params); got != 1 {
		t.Errorf("cards visible to the member = %d, want 1", got)
	}
}

func TestMembershipFilterExcludesNonMembers(t *testing.T) {
	f := newFixture(t)

	// The other user is on exactly one team, and it is not the Platform team.
	params := dbx.Params{"user": f.otherUser.Id}

	if got := resolvesTo(t, f, schema.Teams, ownTeamFilter, params); got != 1 {
		t.Errorf("teams matching the other user = %d, want 1 (their own)", got)
	}

	// Their single visible card must be their own, not the Platform one.
	records, err := f.app.FindRecordsByFilter(schema.Cards, teamMemberFilter, "", 0, 0, params)
	if err != nil {
		t.Fatalf("filter cards: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("cards visible to the other user = %d, want 1", len(records))
	}
	if records[0].Id != f.otherCard.Id {
		t.Errorf("visible card = %q, want their own %q", records[0].Id, f.otherCard.Id)
	}
}

// A user on no team at all should see nothing, confirming the rules are not
// accidentally permissive.
func TestMembershipFilterMatchesNothingForUnaffiliatedUser(t *testing.T) {
	f := newFixture(t)

	loner := testutil.NewUser(t, f.app, "loner@example.test", "Loner", schema.RoleUser)
	params := dbx.Params{"user": loner.Id}

	for collection, filter := range map[string]string{
		schema.Teams:  ownTeamFilter,
		schema.Boards: teamMemberFilter,
		schema.Cards:  teamMemberFilter,
	} {
		if got := resolvesTo(t, f, collection, filter, params); got != 0 {
			t.Errorf("%s visible to an unaffiliated user = %d, want 0", collection, got)
		}
	}
}

// The multi-relation trap, pinned so it cannot be reintroduced: the field must be
// traversed to .id.
func TestBareMultiRelationComparisonMatchesNothing(t *testing.T) {
	f := newFixture(t)
	params := dbx.Params{"user": f.member.Id}

	if got := resolvesTo(t, f, schema.Teams, `members ?= {:user}`, params); got != 0 {
		t.Skipf("PocketBase now supports bare multi-relation comparison (matched %d); the rules could be simplified", got)
	}

	// Given the bare form matches nothing, every rule must use the .id form.
	for _, rule := range []string{schema.TeamMember, schema.OwnTeam, schema.CardTeamMember} {
		if !containsSubstring(rule, "members.id ?=") {
			t.Errorf("rule does not traverse members to .id, so it will match nobody:\n  %s", rule)
		}
	}
}

func containsSubstring(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
