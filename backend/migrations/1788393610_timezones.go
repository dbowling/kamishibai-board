package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/dbowling/kamishibai/backend/internal/schema"
)

// Add time zones to teams and users.
//
// They mean different things, which is why the two Help strings are not the same:
//
//   - A team's zone is operational. Period keys, rollover, which period is
//     writable, rollups and the activity heatmap are all evaluated in it.
//   - A user's zone is display-only. It changes how instants are rendered in that
//     person's browser and nothing the server computes.
//
// Both are optional and neither is backfilled. An empty team zone inherits
// KAMISHIBAI_TIMEZONE, so every existing team behaves exactly as it did, and an
// empty user zone means "use the browser's". Values are validated in hooks, since
// a text field cannot check that a name is a real IANA zone.
func init() {
	m.Register(func(app core.App) error {
		teams, err := app.FindCollectionByNameOrId(schema.Teams)
		if err != nil {
			return err
		}
		teams.Fields.Add(&core.TextField{
			Name: schema.FieldTimezone,
			Max:  64,
			Help: "IANA zone this team's periods roll over in, e.g. Asia/Tokyo. Empty uses the instance default.",
		})
		if err := app.Save(teams); err != nil {
			return err
		}

		users, err := app.FindCollectionByNameOrId(schema.Users)
		if err != nil {
			return err
		}
		users.Fields.Add(&core.TextField{
			Name: schema.FieldTimezone,
			Max:  64,
			Help: "IANA zone used to display times to this person. Display only; empty uses the browser's zone.",
		})
		return app.Save(users)
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId(schema.Users)
		if err != nil {
			return err
		}
		users.Fields.RemoveByName(schema.FieldTimezone)
		if err := app.Save(users); err != nil {
			return err
		}

		teams, err := app.FindCollectionByNameOrId(schema.Teams)
		if err != nil {
			return err
		}
		teams.Fields.RemoveByName(schema.FieldTimezone)
		return app.Save(teams)
	})
}
