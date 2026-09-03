# Kamishibai Triage Board

A shared board for recurring triage work. Cards represent tasks that repeat on a
daily, weekly, monthly, quarterly or annual rhythm. They reset when their period
rolls over, they record who picked each one up and who finished it, and they carry
the instructions needed to actually do the work.

Named after 紙芝居 (*kamishibai*), the paper-theatre boards used on factory floors
to make routine checks visible at a glance.

```
mise install       # install the pinned Go and Node toolchains
mise run setup     # install dependencies
mise run db:reset  # create the database, migrate it, load demo data
mise run dev       # backend on :8090, frontend on :5173
```

Then sign in at http://localhost:5173 as `admin@example.test` or
`dana@example.test` with the password `kamishibai-dev-1234`.

## How it works

### Cards flip without anything writing to the database

This is the central design decision, and everything else follows from it.

A card has no status column. Its state for a given period is the occurrence row
for that `(card, period)` pair, and a row exists **only if somebody touched it**.
So:

- A daily card nobody starts for a year costs **zero rows**, not 365.
- "Not started" is the *absence* of a row.
- When Monday arrives, the weekly period key changes from `2026-W36` to
  `2026-W37`. No row exists for the new key, so every weekly card reads as
  not-started. **Nothing was updated, and no job had to run.** The board flipped
  because time passed.

That last point is what makes the flip reliable. There is no scheduled task that
could fail, run late, or half-complete and leave the board in a state nobody
intended.

### Reporting reads a different table

The sparseness that makes writes cheap makes reporting awkward: you cannot count
work that was never done, because it has no rows. And every historical query would
have to re-derive its own denominator.

So once a period closes and its contents can no longer change, the counts are
computed once and frozen into `report_rollups`, one row per board, cadence and
period. Historical reporting reads a small dense table; the still-open period is
computed live and labelled as such, so you can tell a settled number from a moving
one.

The rollup job re-checks the last few closed periods on every run rather than only
the most recent one. If the process was down over a weekend, the next run fills the
gap instead of leaving a permanent hole.

### Period keys

Canonical, sortable, and readable in a database client:

| Cadence   | Key          | Resets                                 |
| --------- | ------------ | -------------------------------------- |
| Daily     | `2026-09-03` | Every midnight                         |
| Weekly    | `2026-W36`   | Monday (ISO-8601 week)                 |
| Monthly   | `2026-M09`   | 1st of the month                       |
| Quarterly | `2026-Q3`    | 1 Jan, 1 Apr, 1 Jul, 1 Oct             |
| Annual    | `2026-Y`     | 1 January                              |

All boundaries are evaluated in one timezone (`America/New_York` by default) so
the board flips at the same moment for everyone. The timezone database is compiled
into the binary, so this holds in a container with no system tzdata.

The fiddly parts are tested: days that are 23 or 25 hours long across daylight
saving, weeks whose ISO year differs from their calendar year (1 Jan 2026 belongs
to `2026-W01`, which starts in December 2025), and years with 53 ISO weeks.

The server is the only thing that computes period keys. The frontend asks it, so a
second implementation in the browser cannot disagree.

## Layout

```
backend/            PocketBase used as a Go framework
  main.go           serve | migrate | seed | rollup
  migrations/       every schema change, applied automatically on serve
  internal/
    domain/         period and cadence arithmetic. No database, heavily tested
    occurrence/     the lazy read/write engine
    rollup/         freezing closed periods into snapshots
    api/            custom endpoints
    hooks/          record-level rules the API rules cannot express
    access/         one definition of "may this person touch this team"
    seed/           repeatable demo data
frontend/           React, Vite, no UI framework
deploy/k8s/         Kubernetes manifests
```

## Access model

Two roles: **user** and **admin**. Admins are implicitly members of every team, by
role rather than by being listed on each one.

| Action                        | Who                                     |
| ----------------------------- | --------------------------------------- |
| Read a team's boards and cards | Members of that team, and admins        |
| Create a board or a card      | Any member of the team                  |
| Start / complete / reopen     | Any member of the team                  |
| Archive a board or card       | Any member of the team                  |
| **Restore** an archived record | Admins only                             |
| Create or rename a team       | Admins only                             |
| Delete anything               | Nobody                                  |

Nothing is ever deleted. Delete rules are `nil` on every collection, which in
PocketBase means superusers only, so archive-instead-of-delete is enforced by the
API rather than by the UI remembering not to call `DELETE`.

Some deliberate constraints worth knowing about:

- **Occurrences and rollups reject client writes entirely.** All recording goes
  through `/api/kamishibai/cards/{id}/start|complete|reopen`, which take nothing
  but an optional note. The server decides which period is open and stamps who did
  it from the authenticated request. Attribution is therefore not forgeable.
- **Only the current period can be changed.** Completing a card you forgot on
  Friday is refused. The board reflects the ritual as it actually happened, and
  closed periods stay immutable so the snapshots taken from them can be trusted.
- **A card's team is derived from its board**, never taken from the request, and
  nothing can move between teams afterwards, because its history is attributed to
  the original team.
- **No public signup.** Accounts are provisioned by an admin.
- Card instructions are rich text written by teammates, so they are sanitised
  before rendering and link URLs are restricted to `http`, `https` and `mailto`.

## Common tasks

```
mise run test              # backend and frontend tests
mise run check             # what CI runs: lint, test, build
mise run seed              # reload demo data (idempotent)
mise run migrate:create -- add_widget_field
mise run rollup            # recompute reporting snapshots now
mise tasks                 # everything else
```

Seeding is genuinely repeatable: records are matched by natural key and the
invented history is driven by a seed derived from each card and period, so running
it twice leaves the database exactly as running it once.

## Testing

The backend tests run against a real SQLite database with the full migration set
applied to a throwaway directory, so they exercise the actual schema, indexes and
API rules. There are no committed database fixtures to drift out of date.

The API tests drive the real HTTP router in-process. That matters more than it
sounds: PocketBase access rules are filter expressions evaluated by the database,
so a mistake in one is not a compile error or a runtime error. It simply matches
nothing, and looks exactly like a legitimate denial. Two such bugs were caught here
only because these tests assert HTTP outcomes rather than comparing rule strings.

## Deploying

See [deploy/README.md](deploy/README.md).

One thing to carry over: the deployment runs **one replica with the `Recreate`
strategy**, and both halves matter. SQLite allows a single writer, and a
`RollingUpdate` would briefly run two pods against the same file. That is a
reasonable trade for a small team; outgrowing it means moving to Postgres, not
raising the replica count.
