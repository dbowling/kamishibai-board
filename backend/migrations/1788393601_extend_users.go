// Package migrations contains the versioned database schema.
//
// Every structural change lives here as a Go migration so that a fresh checkout
// and a long-running production database converge on the same schema. Nothing
// creates collections at runtime.
//
// Migrations are applied automatically on `serve`, and can be driven manually
// with `go run . migrate up` / `migrate down`.
package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/dbowling/kamishibai/backend/internal/schema"
)

// Extend the built-in users collection with a role, and open up read access so
// teammates can see who started and completed a card.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId(schema.Users)
		if err != nil {
			return err
		}

		// Role is intentionally not Required. An empty role reads as a regular
		// user, so every access rule fails closed if the field is ever missing.
		// A create hook normalises it to "user".
		users.Fields.Add(&core.SelectField{
			Name:      schema.FieldRole,
			Values:    schema.RoleValues(),
			MaxSelect: 1,
			Help:      "Admins are implicitly members of every team and can archive or restore anything.",
		})

		// Any signed-in user can read the directory. The board has to render
		// "started by Dana", which means resolving user records the viewer does
		// not own. For a single small team this is the right trade.
		users.ListRule = types.Pointer(schema.Authenticated)
		users.ViewRule = types.Pointer(schema.Authenticated)

		// Users may edit their own profile; admins may edit anyone. Role changes
		// are additionally gated in a hook so a user cannot promote themselves.
		users.UpdateRule = types.Pointer(`id = @request.auth.id || @request.auth.role = "admin"`)

		// No public signup: this is internal team tooling, so accounts are
		// provisioned by an admin or by the seed command. nil means superusers
		// only.
		users.CreateRule = nil

		// Users are never deleted, because occurrences reference them for
		// attribution and that history has to stay readable.
		users.DeleteRule = nil

		return app.Save(users)
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId(schema.Users)
		if err != nil {
			return err
		}

		users.Fields.RemoveByName(schema.FieldRole)

		// Restore PocketBase's stock owner-only rules.
		const ownerRule = "id = @request.auth.id"
		users.ListRule = types.Pointer(ownerRule)
		users.ViewRule = types.Pointer(ownerRule)
		users.UpdateRule = types.Pointer(ownerRule)
		users.DeleteRule = types.Pointer(ownerRule)
		users.CreateRule = types.Pointer("")

		return app.Save(users)
	})
}
