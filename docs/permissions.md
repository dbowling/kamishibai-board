# Permissions

## The model in one table

There are two roles, `user` and `admin`. Teams are the tenant boundary.

| Action | Who can do it |
| --- | --- |
| Read a team's boards, cards, occurrences, reports | Members of that team, and admins |
| Create, rename or reorder a board | Admins only |
| Move a board to another team (history moves with it) | Admins only |
| Reorder teams | Admins only |
| Create a card | Any member of the team |
| Start / complete / reopen a card | Any member of the team |
| Edit a card | Any member of the team |
| Archive a card | Any member of the team |
| Archive a team or board | Admins only |
| **Restore** an archived record | **Admins only** |
| Create or rename a team, change its members | Admins only |
| Change anybody's role | Admins only |
| Edit your own name and password | Yourself |
| Create a user account | Admins and superusers only |
| **Delete anything** | **Nobody** |

Three asymmetries are deliberate and worth explaining.

**Admins own the sidebar.** Teams and boards are the structure everybody navigates
by, and they can now be reordered and moved, so one member rearranging them would
rearrange them for everyone. Creating, renaming, archiving, restoring, reordering
and moving teams and boards is therefore admin-only, enforced by the collection
rules and the custom endpoints, not just hidden in the UI. Members keep read access
to their own teams' boards and full day-to-day access to cards.

**Anyone can archive a card, only admins can restore.** Tidying your own board's
checks should not need a ticket, but undoing someone else's tidying should involve
someone with a wider view. Nothing is lost either way, since archiving is
reversible and archived records stay readable. Archiving a board or team is an
update, and those update rules are `AdminOnly`, so for them archiving is admin-only
too; the restore check in the hook is then a second line of defence.

**Anyone on a team can create a card.** A board that needs an administrator to add
a check is a board that gradually stops reflecting what the team actually does.

## Admins are not superusers

Three distinct things, easily confused:

| | What it is | How you get one |
| --- | --- | --- |
| **user** | An ordinary account, scoped to the teams it belongs to | Created by an admin |
| **admin** | A `users` record with `role = "admin"`. Implicitly a member of every team; can restore and manage teams | An admin sets the role |
| **superuser** | A PocketBase operator account. Bypasses every application rule | `kamishibai superuser upsert EMAIL PASS` |

Admins are inside the model: their access is expressed *in* the rules, as
`@request.auth.role = "admin"`. Superusers are outside it — they are the recovery
hatch, and the account the CLI and the cron job effectively act as. Day-to-day work,
including administrative work, should happen as an `admin` user.

An admin is a member of every team **by role**, not by being added to member lists.
Nothing enumerates teams for them, and there is no membership to keep in sync when a
team is created.

## Three layers

Authorisation is enforced in three places, each doing what the others cannot.

```
1. Collection API rules   filter expressions in the migrations
        │                 "may this person touch this row at all?"
        ▼
2. Record hooks           Go, in internal/hooks
        │                 "is this specific change allowed, and what should
        │                  the server override?"
        ▼
3. Custom endpoints       Go, in internal/api + internal/occurrence
                          + internal/navigation
                          "the client does not get to decide this at all"
```

### Layer 1: collection API rules

Defined in the migrations, evaluated by the database. The predicates live in
`internal/schema/schema.go` so every collection uses the same wording:

```go
// Any signed-in user.
Authenticated = `@request.auth.id != ""`

// Holds the admin role.
AdminOnly = `@request.auth.id != "" && @request.auth.role = "admin"`

// A member of the team this record belongs to, via its `team` relation.
TeamMember = `@request.auth.id != "" && (@request.auth.role = "admin" || team.members.id ?= @request.auth.id)`

// A member of a `teams` record itself, where the member list is on the record.
OwnTeam = `@request.auth.id != "" && (@request.auth.role = "admin" || members.id ?= @request.auth.id)`

// A member of the team owning the record's *board*, ignoring the record's own team.
BoardTeamMember = `@request.auth.id != "" && (@request.auth.role = "admin" || board.team.members.id ?= @request.auth.id)`
```

Applied like this:

| Collection | List / View | Create | Update | Delete |
| --- | --- | --- | --- | --- |
| `users` | `Authenticated` | `nil` | own record, or admin | `nil` |
| `teams` | `OwnTeam` | `AdminOnly` | `AdminOnly` | `nil` |
| `boards` | `TeamMember` | `AdminOnly` | `AdminOnly` | `nil` |
| `cards` | `TeamMember` | `BoardTeamMember` | `TeamMember` | `nil` |
| `occurrences` | `TeamMember` | `nil` | `nil` | `nil` |
| `report_rollups` | `TeamMember` | `nil` | `nil` | `nil` |

**A team's time zone is admin-only** because it decides when that team's periods
roll over. It needs no hook of its own: it is a field on `teams`, whose update rule
is `AdminOnly`, so a member never reaches the hook that validates it. A **user's**
time zone is display-only and sits on their own record, which they may edit like the
rest of their profile. Both are checked by `domain.ValidTimezone` in the record
hooks (400 on an unknown name).

**`nil` means superusers only.** That is how "archive, never delete" is enforced:
no client, admin included, can issue a successful `DELETE`. It is a property of the
API, not a UI convention that a stray curl could bypass.

**Reads are permitted for archived records.** Archiving hides something from the
active board, but its history has to stay readable or reporting over past periods
would develop holes.

**`users` list and view are open to any signed-in user.** The board renders "started
by Dana", which means resolving user records the viewer does not own. For internal
team tooling that is the right trade; it does mean any authenticated user can
enumerate the user directory.

#### Why cards create differs

`cards.CreateRule` is `BoardTeamMember`, not `TeamMember`, and this is not a
stylistic choice.

A card's `team` is derived from its board by a hook. But **create rules are
evaluated against the submitted request body, before hooks run**. A rule referencing
`team` therefore rejects every well-behaved client that correctly omits it, and only
succeeds if the client supplies its own `team` — exactly what we do not want to
depend on.

Authorising from `board` is both safe and honest: the board is the field the client
is genuinely choosing.

#### The `.id` trap

`teams.members` is a multi-relation, and it must be traversed to `.id`:

```go
`team.members ?= @request.auth.id`      // matches NOTHING. Valid syntax, no error.
`team.members.id ?= @request.auth.id`   // correct
```

The wrong form fails closed, so it looks identical to a legitimate denial. It locked
every member out of their own boards on this project until a test that asserted real
HTTP behaviour caught it. `internal/api/rules_test.go` now pins it.

The general point: **a rule that matches nothing and a rule that correctly denies
are indistinguishable from the outside.** Test rules by making requests.

### Layer 2: record hooks

Some invariants cannot be a filter, because they depend on comparing the old and new
value of a field. Those are in `internal/hooks/hooks.go`.

**No self-promotion.** The users update rule has to allow people to edit their own
profile. Without this hook, "edit your own record" would include "set your own role
to admin", which would hand any user access to every team:

```go
app.OnRecordUpdateRequest(schema.Users).BindFunc(func(e *core.RecordRequestEvent) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}
	previous := e.Record.Original().GetString(schema.FieldRole)
	if e.Record.GetString(schema.FieldRole) != previous && !access.IsAdmin(e.Auth) {
		return e.ForbiddenError("Only an admin can change a user's role.", nil)
	}
	return e.Next()
})
```

**A card's team is always recomputed from its board**, discarding whatever the client
sent. Since the access rules depend on that column, a client that could set it freely
could make its card readable by a team that does not own the board.

**Nothing moves between teams through the collection API.** Occurrences carry a
denormalised team and rollups aggregate per team, so changing a board's or card's
team with a plain PATCH would leave that history attributed to the wrong tenant.
It is refused, admins included. The one exception is the admin-only
`POST /api/kamishibai/boards/{boardId}/move` endpoint (Layer 3), which re-points the
board, its cards, occurrences and rollups together in one transaction. Cards never
move on their own.

**`created_by` is stamped by the server** and preserved on update.

**Archive transitions are owned by the server.** Archiving stamps `archived_by` from
the authenticated user; restoring requires admin; any other update resets
`archived_by` to its stored value so it cannot be set arbitrarily.

### Layer 3: things the client never decides

`occurrences` and `report_rollups` reject client writes entirely. All recording goes
through:

```
POST /api/kamishibai/cards/:id/start
POST /api/kamishibai/cards/:id/complete
POST /api/kamishibai/cards/:id/reopen
```

The request body accepts exactly one field:

```json
{ "notes": "all green" }
```

Note what is absent: no period key, no status, no user id, no card id. The card comes
from the path, the period from the server clock, the status from the endpoint, and
the attribution from the authenticated token.

This is what makes "Dana completed this on Tuesday" worth anything. If clients could
POST to the occurrences collection, all four could be forged, and the audit trail
would be a record of what people claimed rather than what happened.

`internal/api/api_test.go` asserts this directly: it posts a completion with
`completed_by` set to a different user, a `period_key` of `1999-01-01`, a `status` of
`not_started` and a different `card`, then checks the stored row used the
authenticated user, the current period, `done`, and the card from the path.

The same tests confirm a direct write to the collection fails:

```go
res := f.client.POST(t, "/api/collections/occurrences/records", token, forgedPayload)
// must not be 200 or 201, and the table must still be empty
```

#### Sidebar structure

Two admin-only endpoints manage the sidebar:

```
POST /api/kamishibai/navigation/order        {"teams": [...], "boards": {"teamId": [...]}}
POST /api/kamishibai/boards/{boardId}/move   {"team": "teamId"}
```

`order` sets `sort_order` to the array index for each listed team and board, in one
transaction. It never moves anything: a board listed under a team it does not
belong to is a 400 and nothing is written. `move` re-homes a board and its history
(see above); it refuses archived boards or teams, a same-team move, and a name that
clashes with an active board on the target team. Both return 403 to non-admins,
decided before anything about the board is revealed.

The admin check and the transactions live in `internal/navigation`, not in the HTTP
handlers, so no future caller can skip them.

Authorisation for these lives in `internal/occurrence`, not in the HTTP handler, so
no future caller can skip it. `mutateOnce` checks team membership, then walks the
chain and refuses if the team, board **or** card is archived.

## Archived records are frozen

Archiving anything freezes the work beneath it. An archived team's boards are
read-only, an archived board's cards are read-only, and an archived card cannot be
started or completed — even by an admin, and even though the team is otherwise
active.

`access.CanWriteTeam` is `CanReadTeam && !IsArchived`, and the occurrence service
checks each level.

## Shared helpers

`internal/access` exists so "may this person touch this team" has one definition:

```go
access.IsAdmin(user)                      // role == "admin", fails closed otherwise
access.IsMember(team, user)               // literally on the member list
access.CanReadTeam(team, user)            // admin or member
access.CanWriteTeam(team, user)           // the above, and not archived
access.IsArchived(record)                 // archived_at is set
access.LoadTeam(app, record)              // follow the `team` relation
access.VisibleTeamIDs(app, user)          // every team the user can read
```

`IsAdmin` treats anything other than exactly `"admin"` as an ordinary user, so a
missing, empty or misspelled role denies rather than grants. The `role` field is
intentionally **not** required in the schema for the same reason: an empty value has
to be safe.

## Realtime respects the same rules

A realtime subscription is not a way around the API rules. PocketBase evaluates the
collection's **ViewRule for each subscriber** before delivering an event, so a user
subscribed to `occurrences` only receives events for occurrences on their own teams.

This is a genuine property of the framework rather than something layered on here,
but it is worth knowing because it is the kind of thing that is easy to assume and
expensive to be wrong about. See [Realtime](realtime.md#is-this-secure).

## Adding a permission-sensitive feature

1. Decide which layer it belongs to. Row-level visibility is a rule; "compare old
   and new" is a hook; "the client must not decide this" is a custom endpoint.
2. If it is a rule, reuse a predicate from `internal/schema`. If you need a new one,
   add it there rather than inlining a string in a migration.
3. Remember `.id` on multi-relations, and remember create rules only see the body.
4. **Write a test that makes a request**, asserting both the permitted and the
   forbidden case. A rule that matches nothing looks exactly like a rule that works.
5. If the feature writes data that must be trustworthy, keep the client out of it:
   nil collection write rules, plus an endpoint that derives the sensitive fields.
