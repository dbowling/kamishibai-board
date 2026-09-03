# Schema and migrations

Every table, field, index and access rule is defined by a Go migration in
`backend/migrations/`. Nothing creates or alters a collection at runtime, and the
PocketBase dashboard is not the source of truth. A fresh checkout and a
long-running production database converge on the same schema because they run the
same ordered list of migrations.

## How migrations work here

Each file registers an up function and a down function:

```go
func init() {
	m.Register(func(app core.App) error {
		// apply the change
		return nil
	}, func(app core.App) error {
		// undo it
		return nil
	})
}
```

`m.Register` appends to a global list. `backend/main.go` imports the package for
its side effects only, which is what makes the migrations part of the binary:

```go
import _ "github.com/dbowling/kamishibai/backend/migrations"
```

Files are applied in filename order, which is why they are prefixed with a Unix
timestamp. Applied filenames are recorded in the internal `_migrations` table, so
each runs exactly once per database.

Both callbacks receive a **transactional** `core.App`. If the function returns an
error, the whole migration rolls back.

### When they run

`serve` applies pending migrations before it starts listening. That means deploying
a new image is the whole migration step; there is no separate job to remember. The
`seed` and `rollup` commands also call `RunAllMigrations()` first, so they cannot
operate against a stale schema.

## Adding a field

Say cards need a `priority` number.

**1. Generate the file.** Run from `backend/`, because the generator writes relative
to the working directory:

```bash
mise run migrate:create -- add_card_priority
# creates backend/migrations/1788470000_add_card_priority.go
```

**2. Add the field name to `internal/schema/schema.go`.** Every field is referenced
through a constant so a rename is a compile error rather than a silent runtime miss:

```go
const FieldPriority = "priority"
```

**3. Write the migration.**

```go
package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/dbowling/kamishibai/backend/internal/schema"
)

func init() {
	m.Register(func(app core.App) error {
		cards, err := app.FindCollectionByNameOrId(schema.Cards)
		if err != nil {
			return err
		}

		cards.Fields.Add(&core.NumberField{
			Name:    schema.FieldPriority,
			OnlyInt: true,
			Min:     types.Pointer(1.0),
			Max:     types.Pointer(5.0),
			Help:    "1 is most urgent. Used only for display ordering.",
		})

		cards.AddIndex("idx_cards_board_priority", false, "board, priority", "")

		return app.Save(cards)
	}, func(app core.App) error {
		cards, err := app.FindCollectionByNameOrId(schema.Cards)
		if err != nil {
			return err
		}
		cards.RemoveIndex("idx_cards_board_priority")
		cards.Fields.RemoveByName(schema.FieldPriority)
		return app.Save(cards)
	})
}
```

**4. Apply it.** Restarting the backend is enough, or explicitly:

```bash
mise run migrate:up
```

**5. Extend the tests.** `backend/migrations/schema_test.go` asserts the shape the
migrations produce. Add an assertion there, and see the warning about behaviour
versus strings below.

**6. Surface it.** Add it to the API DTO in `internal/api/dto.go`, the TypeScript
type in `frontend/src/lib/types.ts`, and wherever it should appear in the UI.

## Creating a collection

Follow the pattern in `1788393603_create_boards.go`. The essentials:

```go
users, err := app.FindCollectionByNameOrId(schema.Users)
if err != nil {
	return err
}

c := core.NewBaseCollection("widgets")   // or NewAuthCollection / NewViewCollection

c.ListRule = types.Pointer(schema.TeamMember)
c.ViewRule = types.Pointer(schema.TeamMember)
c.CreateRule = types.Pointer(schema.TeamMember)
c.UpdateRule = types.Pointer(schema.TeamMember)
c.DeleteRule = nil            // nil means superusers only

c.Fields.Add(
	&core.TextField{Name: "name", Required: true, Max: 100, Presentable: true},
	&core.RelationField{
		Name:          schema.FieldTeam,
		CollectionId:  teams.Id,
		Required:      true,
		MaxSelect:     1,
		CascadeDelete: true,
	},
	&core.AutodateField{Name: schema.FieldCreated, OnCreate: true},
	&core.AutodateField{Name: schema.FieldUpdated, OnCreate: true, OnUpdate: true},
)

c.AddIndex("idx_widgets_team", false, "team", "")

return app.Save(c)
```

Down migration:

```go
c, err := app.FindCollectionByNameOrId("widgets")
if err != nil {
	return err
}
return app.Delete(c)
```

### Field types

`core.TextField`, `NumberField`, `BoolField`, `EmailField`, `URLField`,
`DateField`, `AutodateField`, `SelectField`, `RelationField`, `FileField`,
`JSONField`, `EditorField`, `GeoPointField`.

Options worth knowing:

- `Required` — non-empty. For `NumberField` this means non-zero, which is usually
  not what you want for a count.
- `Presentable` — hints to the dashboard to use this field as the record's label.
- `Hidden` — omitted from API responses.
- `MaxSelect` — for relations and selects; must be `> 1` for multi-value.
- `CascadeDelete` — on a relation, deletes this record when the target goes.
- `Min` / `Max` on `NumberField` are `*float64`; use `types.Pointer(1.0)`.

### Indexes

```go
c.AddIndex("idx_name", unique, "col_a, col_b", "optional WHERE clause")
```

The partial index clause is genuinely useful. Team and board names are unique only
among *active* records, so archiving a board frees its name:

```go
c.AddIndex("idx_boards_team_name_active", true, "team, name", "archived_at = ''")
```

The most important index in the schema is on `occurrences`:

```go
c.AddIndex("idx_occurrences_card_period", true, "card, period_key", "")
```

That uniqueness is load-bearing. It is what makes the lazy "create the row if
somebody touches it" write safe when two people click Start at the same instant:
the database lets exactly one insert win, and the loser retries and updates
instead. See `internal/occurrence`.

## Reverting

```bash
mise run migrate:down          # revert the most recent
mise run migrate:down -- 3     # revert the last three
```

Then restart the server, because it caches collection state at boot.

Down migrations are worth writing properly. They are how you get out of a bad
deploy, and the cost of writing one is a few minutes while the schema is fresh in
your mind.

Note that a down migration should reverse *its own* change, not correct history.
`1788393608_fix_team_member_rules.go` restores the previous, buggy rules on the way
down, which looks odd until you consider the alternative: if down migrations
"improved" things, running down then up would not return you to where you started.

## Rules for staying out of trouble

### Never edit an applied migration

Applied filenames are recorded, so an edited file does not re-run. You end up with a
schema that depends on when the database was created: fresh checkouts get the new
version, existing databases keep the old one, and the difference shows up as a
confusing bug much later.

Add a new migration instead, even for a one-character fix.

### Do not share helpers between migrations

Every migration here spells out its own field list rather than calling a shared
builder. That is deliberate. If a shared helper changes, already-applied migrations
do not re-run, but a fresh database gets the new behaviour — the same divergence as
editing a file, arrived at indirectly.

The one exception in this codebase is the `schema` constants package, and the
schema test exists specifically to catch it drifting.

### The dashboard is not the source of truth

Editing collections in the dashboard changes the database without changing the
migrations, so the next fresh deploy will not have your change.

During `go run`, `Automigrate` is enabled, so dashboard edits generate a migration
file for you. Treat that file as a draft: read it, rename it, and squash it into
something intentional before committing. `Automigrate` is off for real builds.

If you have been experimenting and the history is cluttered, delete the intermediate
files and then:

```bash
mise run migrate:status    # go run . migrate history-sync
```

which drops `_migrations` entries with no matching file.

## Two PocketBase behaviours that will bite you

Both of these cost real debugging time on this project. Neither produces an error.

### Multi-relation fields must be traversed to `.id`

`teams.members` is a multi-relation. This rule looks correct and matches **nothing**:

```go
// WRONG: silently matches no records at all
`team.members ?= @request.auth.id`
```

```go
// RIGHT
`team.members.id ?= @request.auth.id`
```

Both are valid filter syntax. Neither raises an error. The wrong one fails closed,
so it presents as "you are not a member of this team" — indistinguishable from a
legitimate denial. It locked every member out of their own boards until a test that
asserted real HTTP behaviour caught it.

There is now a regression test in `internal/api/rules_test.go` that pins this.

### Create rules are evaluated against the request body

For creates, the rule is evaluated against what the client submitted, **before**
`OnRecordCreateRequest` hooks have run. So a create rule cannot reference a field
that a hook derives.

A card's `team` is derived from its board by a hook, so the obvious rule fails for
any client that correctly omits `team`:

```go
// WRONG as a create rule: `team` is not in the submitted body
CreateRule = types.Pointer(schema.TeamMember)
```

```go
// RIGHT: authorise from the board, which the client really is choosing
CreateRule = types.Pointer(schema.BoardTeamMember)
// `@request.auth.id != "" && (@request.auth.role = "admin" || board.team.members.id ?= @request.auth.id)`
```

The general lesson: write create rules against fields the client actually sends.

## Rules versus hooks

Some invariants cannot be expressed as a filter, because they depend on comparing
the old and new value of a field. Those live in `internal/hooks` instead:

- a user cannot change their own `role` (the update rule has to allow profile
  edits, and without this that would include self-promotion to admin)
- a card's `team` is always recomputed from its board, discarding client input
- nothing moves between teams
- `created_by` is stamped by the server and then immutable
- archiving stamps `archived_by`; restoring is admin-only

Rules answer "may this person touch this row at all". Hooks answer "is this
specific change allowed, and what should the server override". See
[Permissions](permissions.md).

## Testing schema changes

`backend/migrations/schema_test.go` runs the full migration set against a throwaway
SQLite database and asserts the result: collections exist, rules match the
constants, delete rules are `nil`, the unique occurrence index exists and is
actually `UNIQUE`, `not_started` is not a storable status.

**A warning about what that test can and cannot catch.** It compares rule
*strings*. Both of the bugs above produce a correct-looking string that behaves
wrongly. So a schema change that touches access rules needs a behavioural test too
— an actual HTTP request, as a real user, asserting the status code. That is what
`internal/api/api_test.go` is for. See [Testing](testing.md).

```bash
mise run backend:test
```
