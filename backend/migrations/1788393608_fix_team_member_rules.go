package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/dbowling/kamishibai/backend/internal/schema"
)

// Correct the team-membership access rules.
//
// The original rules tested `team.members ?= @request.auth.id`. On a
// multi-relation field that comparison silently matches nothing: the path has to
// be traversed to the related record's id, as `team.members.id ?= ...`.
//
// The failure mode was the dangerous kind. Both spellings are valid filter
// syntax, neither raises an error, and the wrong one simply denies everything, so
// it presents as "you are not on this team" rather than as a broken rule. Any
// database created before this migration has rules that lock members out of their
// own boards.
//
// Re-applying the rules is idempotent, so this is a no-op on a database created
// after the fix.
func init() {
	m.Register(func(app core.App) error {
		// teams matches against its own member list; everything else hops through
		// its team relation.
		if err := setRules(app, schema.Teams, schema.OwnTeam, schema.OwnTeam); err != nil {
			return err
		}

		for _, name := range []string{schema.Boards, schema.Cards, schema.Occurrences, schema.Rollups} {
			if err := setRules(app, name, schema.TeamMember, schema.TeamMember); err != nil {
				return err
			}
		}

		// Boards allow any team member to create and edit. A board's team is
		// supplied by the client (there is nothing to derive it from), so the rule
		// can read it straight from the body.
		boards, err := app.FindCollectionByNameOrId(schema.Boards)
		if err != nil {
			return err
		}
		boards.CreateRule = types.Pointer(schema.TeamMember)
		boards.UpdateRule = types.Pointer(schema.TeamMember)
		if err := app.Save(boards); err != nil {
			return err
		}

		// Cards are different: their team is derived from the board by a hook, so
		// it is absent from the body the create rule is evaluated against. Author-
		// ising from the board is what makes "any team member can add a card"
		// actually work without trusting a client-supplied team.
		cards, err := app.FindCollectionByNameOrId(schema.Cards)
		if err != nil {
			return err
		}
		cards.CreateRule = types.Pointer(schema.BoardTeamMember)
		cards.UpdateRule = types.Pointer(schema.TeamMember)
		if err := app.Save(cards); err != nil {
			return err
		}

		return nil
	}, func(app core.App) error {
		// The down direction deliberately restores the broken rules, because a
		// migration's job is to reverse its own change rather than to decide the
		// previous state was wrong.
		const brokenTeamMember = `@request.auth.id != "" && (@request.auth.role = "admin" || team.members ?= @request.auth.id)`
		const brokenOwnTeam = `@request.auth.id != "" && (@request.auth.role = "admin" || members ?= @request.auth.id)`

		if err := setRules(app, schema.Teams, brokenOwnTeam, brokenOwnTeam); err != nil {
			return err
		}
		for _, name := range []string{schema.Boards, schema.Cards, schema.Occurrences, schema.Rollups} {
			if err := setRules(app, name, brokenTeamMember, brokenTeamMember); err != nil {
				return err
			}
		}
		for _, name := range []string{schema.Boards, schema.Cards} {
			collection, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			collection.CreateRule = types.Pointer(brokenTeamMember)
			collection.UpdateRule = types.Pointer(brokenTeamMember)
			if err := app.Save(collection); err != nil {
				return err
			}
		}
		return nil
	})
}

// setRules updates the list and view rules of a collection.
func setRules(app core.App, name, listRule, viewRule string) error {
	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return err
	}
	collection.ListRule = types.Pointer(listRule)
	collection.ViewRule = types.Pointer(viewRule)
	return app.Save(collection)
}
