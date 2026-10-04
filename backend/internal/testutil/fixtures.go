package testutil

import (
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/dbowling/kamishibai/backend/internal/domain"
	"github.com/dbowling/kamishibai/backend/internal/schema"
)

// Fixture builders. These write through app.Save, which bypasses the collection
// API rules on purpose: tests need to arrange state directly, and the rules
// themselves are covered by the schema and API tests.

// NewUser creates a user with the given role (schema.RoleUser or RoleAdmin).
func NewUser(t testing.TB, app core.App, email, name, role string) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(schema.Users)
	if err != nil {
		t.Fatalf("find users collection: %v", err)
	}

	rec := core.NewRecord(collection)
	rec.Set("email", email)
	rec.Set(schema.FieldName, name)
	rec.Set(schema.FieldRole, role)
	rec.Set("verified", true)
	rec.SetPassword("test-password-1234")

	if err := app.Save(rec); err != nil {
		t.Fatalf("create user %q: %v", email, err)
	}
	return rec
}

// NewTeam creates an active team with the given members.
func NewTeam(t testing.TB, app core.App, name string, members ...*core.Record) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(schema.Teams)
	if err != nil {
		t.Fatalf("find teams collection: %v", err)
	}

	ids := make([]string, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.Id)
	}

	rec := core.NewRecord(collection)
	rec.Set(schema.FieldName, name)
	rec.Set(schema.FieldMembers, ids)

	if err := app.Save(rec); err != nil {
		t.Fatalf("create team %q: %v", name, err)
	}
	return rec
}

// NewBoard creates an active board belonging to team.
func NewBoard(t testing.TB, app core.App, team *core.Record, name string) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(schema.Boards)
	if err != nil {
		t.Fatalf("find boards collection: %v", err)
	}

	rec := core.NewRecord(collection)
	rec.Set(schema.FieldTeam, team.Id)
	rec.Set(schema.FieldName, name)

	if err := app.Save(rec); err != nil {
		t.Fatalf("create board %q: %v", name, err)
	}
	return rec
}

// NewCard creates an active card on board with the given cadence.
func NewCard(t testing.TB, app core.App, board *core.Record, title string, cadence domain.Cadence) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(schema.Cards)
	if err != nil {
		t.Fatalf("find cards collection: %v", err)
	}

	rec := core.NewRecord(collection)
	rec.Set(schema.FieldBoard, board.Id)
	// Mirrors what the card hook does in production.
	rec.Set(schema.FieldTeam, board.GetString(schema.FieldTeam))
	rec.Set(schema.FieldTitle, title)
	rec.Set(schema.FieldCadence, string(cadence))

	if err := app.Save(rec); err != nil {
		t.Fatalf("create card %q: %v", title, err)
	}
	return rec
}

// Archive marks a record archived, as the archive endpoint would.
func Archive(t testing.TB, app core.App, record *core.Record, by *core.Record) {
	t.Helper()

	record.Set(schema.FieldArchivedAt, types.NowDateTime())
	if by != nil {
		record.Set(schema.FieldArchivedBy, by.Id)
	}
	if err := app.Save(record); err != nil {
		t.Fatalf("archive %s/%s: %v", record.Collection().Name, record.Id, err)
	}
}

// Backdate sets a record's created timestamp. PocketBase's autodate field
// overwrites created on every Save, so the column is written directly. Use it
// to arrange records that existed before a pinned period instead of relying on
// the real wall clock.
func Backdate(t testing.TB, app core.App, record *core.Record, created time.Time) {
	t.Helper()
	setDateColumn(t, app, record, schema.FieldCreated, created)
}

// BackdateArchived sets a record's archived_at timestamp, for the same reason
// as Backdate: Archive stamps the real wall clock.
func BackdateArchived(t testing.TB, app core.App, record *core.Record, archivedAt time.Time) {
	t.Helper()
	setDateColumn(t, app, record, schema.FieldArchivedAt, archivedAt)
}

func setDateColumn(t testing.TB, app core.App, record *core.Record, field string, ts time.Time) {
	t.Helper()

	value, err := types.ParseDateTime(ts)
	if err != nil {
		t.Fatalf("parse %s time: %v", field, err)
	}

	name := record.Collection().Name
	_, err = app.DB().
		Update(name, dbx.Params{field: value.String()}, dbx.HashExp{"id": record.Id}).
		Execute()
	if err != nil {
		t.Fatalf("set %s on %s/%s: %v", field, name, record.Id, err)
	}
	record.Set(field, value)
}

// CountRecords returns the number of rows in a collection, failing the test on
// error. Used to assert the lazy-write behaviour.
func CountRecords(t testing.TB, app core.App, collection string) int {
	t.Helper()

	n, err := app.CountRecords(collection)
	if err != nil {
		t.Fatalf("count %s: %v", collection, err)
	}
	return int(n)
}
