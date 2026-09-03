package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/dbowling/kamishibai/backend/internal/schema"
)

// Baseline instance settings, so a fresh deployment comes up recognisable and
// with sane log retention rather than the stock defaults.
func init() {
	m.Register(func(app core.App) error {
		s := app.Settings()

		s.Meta.AppName = "Kamishibai Triage Board"

		// Keep a fortnight of request logs. Long enough to debug an incident,
		// short enough that the auxiliary database does not grow unbounded.
		s.Logs.MaxDays = 14
		s.Logs.LogAuthId = true
		s.Logs.LogIP = false

		return app.Save(s)
	}, func(app core.App) error {
		s := app.Settings()
		s.Meta.AppName = "Acme"
		s.Logs.MaxDays = 7
		s.Logs.LogAuthId = false
		s.Logs.LogIP = true
		return app.Save(s)
	})
}

// Guard against the schema package drifting out of sync with the collections
// these migrations create. If a collection constant is renamed without a
// matching migration, this fails to compile rather than failing at runtime.
var _ = []string{
	schema.Users,
	schema.Teams,
	schema.Boards,
	schema.Cards,
	schema.Occurrences,
	schema.Rollups,
}
