// Package testutil provides the shared test harness for backend tests.
//
// Tests run against a real SQLite database in a throwaway directory with the
// full migration set applied, so they exercise the same schema, indexes and API
// rules as production. There are no committed database fixtures to drift out of
// date.
package testutil

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	// Registers the application migrations so NewApp brings up the real schema.
	_ "github.com/dbowling/kamishibai/backend/migrations"
)

// NewApp returns a fully migrated test application backed by a fresh temporary
// data directory. Cleanup is registered automatically.
func NewApp(t testing.TB) *tests.TestApp {
	t.Helper()

	// t.TempDir() is empty, so PocketBase bootstraps a brand new database and
	// then RunAllMigrations (called inside NewTestApp) applies the system
	// migrations followed by ours.
	app, err := tests.NewTestApp(t.TempDir())
	if err != nil {
		t.Fatalf("create test app: %v", err)
	}
	t.Cleanup(app.Cleanup)

	return app
}

// Collection fetches a collection or fails the test.
func Collection(t testing.TB, app core.App, name string) *core.Collection {
	t.Helper()
	c, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		t.Fatalf("find collection %q: %v", name, err)
	}
	return c
}
