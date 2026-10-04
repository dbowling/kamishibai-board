# Development

## Prerequisites

Only [mise](https://mise.jdx.dev) is needed. It installs the pinned Go and Node
versions declared in `.mise.toml`, so nothing has to be installed globally and
everyone gets the same toolchain.

```bash
brew install mise          # or see mise.jdx.dev for other installers
mise trust                 # approve this repo's .mise.toml, once
mise install               # install Go 1.27 and Node 26
```

`mise trust` is required because `.mise.toml` sets environment variables and
defines tasks. mise will not run an untrusted config, which is a sensible default.

## First run

```bash
mise run setup       # go mod download + npm install
mise run db:reset    # delete any local database, migrate, seed demo data
mise run dev         # start both servers
```

Then open http://localhost:5173 and sign in:

| Account | Role | On teams |
| --- | --- | --- |
| `admin@example.test` | admin | all of them, implicitly |
| `dana@example.test` | user | Platform Engineering, Security |
| `mei@example.test` | user | Platform Engineering |
| `sam@example.test` | user | Security |

Password for all of them: `kamishibai-dev-1234`.

Signing in as `dana` and `mei` in two browser profiles side by side is the quickest
way to see the shared board and realtime updates working.

## What is running

`mise run dev` starts two processes:

| Port | What | Notes |
| --- | --- | --- |
| 8090 | The Go backend | Applies pending migrations on boot |
| 5173 | The Vite dev server | Proxies `/api` and `/_` to 8090 |

Always use **http://localhost:5173** during development. The Vite proxy means the
browser sees a single origin, so cookies, the realtime stream and relative URLs
behave exactly as they do in production. Hitting :8090 directly serves the last
production bundle from `backend/pb_public`, which is usually stale.

The PocketBase admin dashboard is at http://localhost:5173/_/ and is genuinely
useful for poking at data. You will need a superuser:

```bash
cd backend && go run . superuser upsert you@example.com 'a-long-password'
```

A superuser bypasses all application access rules. It is an operator account for
inspection and recovery, not a way to use the app.

## Code layout

```
backend/
  main.go              wires everything together; defines the CLI
  migrations/          every schema change, in order
  internal/
    domain/            period and cadence arithmetic. No database. Start here
    schema/            collection names, field names, API rule strings
    config/            environment configuration
    access/            "may this person touch this team?" in one place
    occurrence/        the lazy read/write engine for card state
    rollup/            freezing closed periods into statistics
    hooks/             record rules the API rules cannot express
    api/               custom HTTP endpoints
    app/               Setup(): binds hooks, routes and the cron job
    seed/              repeatable demo data
    cli/               the seed and rollup commands
    testutil/          test harness: app, fixtures, HTTP client
frontend/
  src/
    lib/               pocketbase client, types, api wrapper, pure helpers
    auth/              auth context and the sign-in screen
    boards/            the board, cards, card detail, new-card form
    reports/           completion history tables and the Heat.js activity heatmap
```

### Reading it for the first time

`internal/domain` is the best entry point. It is pure Go with no database, it is
where the period logic lives, and its tests read as a specification of how flipping
works. From there, `internal/occurrence` shows how that logic becomes rows, and
`internal/api` shows how it becomes HTTP.

## Everyday tasks

```bash
mise run test              # backend + frontend
mise run backend:test      # Go only
mise run backend:test:race # with the race detector, as CI runs it
mise run frontend:test     # Vitest only, single run
mise run lint              # go vet + tsc --noEmit
mise run check             # lint + test + build, i.e. what CI does
mise run ci:local          # run the actual GitHub Actions workflow via act
mise run backend:fmt       # gofmt
mise run seed              # reload demo data (idempotent, safe to repeat)
mise run seed:reset        # remove seeded records first, then reseed
mise run db:reset          # nuclear option: delete the database and start over
mise run rollup            # recompute reporting snapshots now
mise tasks                 # the full list
```

For a watch loop while working on the frontend:

```bash
cd frontend && npm run test:watch
```

### Passing arguments to a task

mise forwards anything after `--`:

```bash
mise run migrate:create -- add_priority_to_cards
mise run migrate:down -- 2
```

## The CLI

The binary is the operator interface as well as the server. Run these from
`backend/`:

```bash
go run . serve                    # start the server
go run . migrate up               # apply pending migrations
go run . migrate down [n]         # revert the last n migrations (default 1)
go run . migrate create NAME      # generate a blank migration file
go run . migrate collections      # snapshot current collections as a migration
go run . migrate history-sync     # drop history entries with no matching file
go run . seed                     # load demo data
go run . rollup                   # recompute reporting snapshots
go run . backup                   # archive pb_data into pb_data/backups
go run . superuser upsert EMAIL PASS
go run . --help
```

`serve`, `migrate` and `superuser` come from PocketBase. `seed`, `rollup` and
`backup` are ours, in `internal/cli`.

`serve` applies pending migrations before it starts listening, so in development
you rarely need `migrate up` explicitly.

### Seeding

`seed` is idempotent. Records are matched by natural key (user email, team name,
board name within a team, card title within a board) and updated in place, and the
invented completion history is generated from a seed derived from each card and
period. Running it twice leaves the database exactly as running it once. There is a
test that enforces this.

```bash
go run . seed                      # 45 closed periods of history, plus rollups
go run . seed --history 0          # cards but no invented history
go run . seed --history 90         # a longer window for reporting screens
go run . seed --reset              # remove seeded records first
go run . seed --password hunter2   # different password for seeded accounts
```

`--reset` only deletes records matching the seed's own dataset: its two named teams
and its `*.example.test` accounts. It will not touch a team you created by hand,
and there is a test for that too. It is still a development tool; do not point it
at production.

## Configuration

Defaults are set for local development in the `[env]` block of `.mise.toml`, and
every value has a working fallback in code.

| Variable | Default | Meaning |
| --- | --- | --- |
| `KAMISHIBAI_TIMEZONE` | `America/New_York` | The timezone all period boundaries are evaluated in |
| `KAMISHIBAI_ROLLUP_CRON` | `10 0 * * *` | When the rollup job runs, in that timezone |
| `KAMISHIBAI_ROLLUP_LOOKBACK` | `14` | How many closed periods it re-checks per run |
| `KAMISHIBAI_PUBLIC_DIR` | `./pb_public` | Where the built frontend is served from |
| `KAMISHIBAI_HTTP_ADDR` | `127.0.0.1:8090` | Local backend address (used by mise tasks and the Vite proxy) |

A bad timezone is a startup error rather than a silent fallback, because a typo
there would quietly shift every boundary in the system.

## The API surface

Two kinds of endpoint.

**PocketBase's generated collection API**, used for ordinary reads and writes of
teams, boards and cards, guarded by the API rules in the migrations:

```
GET    /api/collections/cards/records?filter=...
POST   /api/collections/cards/records
PATCH  /api/collections/cards/records/:id
```

**Custom endpoints** for the things that API cannot do safely or efficiently:

```
GET  /api/kamishibai/periods/current          which period each cadence is in
GET  /api/kamishibai/boards/:id/state         a board, its cards, and each status
GET  /api/kamishibai/boards/:id/report        completion history
POST /api/kamishibai/cards/:id/start
POST /api/kamishibai/cards/:id/complete
POST /api/kamishibai/cards/:id/reopen
```

The three mutation endpoints accept only `{"notes": "..."}`, and nothing else. No
period key, no status, no user id. All three are the server's to decide, which is
what makes the audit trail worth having.

Trying it with curl:

```bash
TOKEN=$(curl -s -X POST http://localhost:8090/api/collections/users/auth-with-password \
  -H 'content-type: application/json' \
  -d '{"identity":"dana@example.test","password":"kamishibai-dev-1234"}' \
  | jq -r .token)

curl -s -H "Authorization: $TOKEN" \
  http://localhost:8090/api/kamishibai/periods/current | jq

BOARD=$(curl -s -H "Authorization: $TOKEN" \
  'http://localhost:8090/api/collections/boards/records?perPage=1' | jq -r '.items[0].id')

curl -s -H "Authorization: $TOKEN" \
  "http://localhost:8090/api/kamishibai/boards/$BOARD/state" | jq '.summary'
```

## Frontend conventions

**No UI framework and no CSS library.** `src/index.css` is plain CSS with custom
properties, a light and dark theme, and a `prefers-reduced-motion` block. For a UI
this size that is less code than configuring a framework.

**No routing library.** `src/lib/router.ts` is about sixty lines covering three
destinations. It uses real URLs and `popstate`, so deep links and the back button
work.

**No data-fetching library.** `useBoardState` and `useBoards` are plain
`useState`/`useEffect` hooks.

**Period keys come from the server.** The frontend never computes them. ISO week
numbering and daylight-saving boundaries are fiddly enough that a second
implementation would eventually disagree with the first, and then two people would
be looking at different days' work.

**Card instructions are untrusted.** They are rich text written by teammates and
rendered in everyone else's browser, so they go through `sanitizeInstructions`
before being inserted as HTML, and link URLs are restricted to `http`, `https` and
`mailto` by `safeUrl`. If you add a place that renders card content, use those.

## Troubleshooting

**"mise: config file is not trusted"** — run `mise trust`.

**Frontend loads but every request 404s** — you are on :8090 instead of :5173.

**Signed in but the board is empty** — the account may not be on a team. Check the
sidebar; if it says you are not on any team, add yourself via the admin dashboard
or run `mise run seed`.

**Schema changes not appearing** — restart the backend. Migrations run at startup.
If you edited a migration that has already been applied, it will not re-run; see
[Schema and migrations](schema-and-migrations.md).

**Realtime updates not arriving** — check the browser network tab for a pending
`GET /api/realtime` request. It should stay open indefinitely. See
[Realtime](realtime.md#debugging).

**Tests pass locally but fail in CI near midnight** — a test may be using the real
clock across a period boundary. Use `WithClock`; see [Testing](testing.md#testing-time-dependent-behaviour).
