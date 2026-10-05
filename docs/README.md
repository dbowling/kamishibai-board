# Kamishibai documentation

A shared board for recurring triage work. Cards repeat on a daily, weekly,
monthly, quarterly or annual rhythm, reset when their period rolls over, and
record who picked each one up and who finished it.

## Reading order

If you are new to the project, read these in order. The first two get you running;
the rest can be read as you need them.

| Doc | What it covers |
| --- | --- |
| [Overview](overview.md) | What the application is for, what it does, and the one design decision everything else follows from |
| [Development](development.md) | Running it locally, the layout of the code, day-to-day workflow |
| [Schema and migrations](schema-and-migrations.md) | Changing the database: writing, running and reverting migrations |
| [Testing](testing.md) | Running tests, the fixtures and helpers available, how to test time-dependent behaviour |
| [Permissions](permissions.md) | Who can do what, and the three layers that enforce it |
| [Managing the sidebar](sidebar.md) | Admin guide to teams, boards, ordering and moves, and how the editor is built |
| [Scheduling and time](scheduling.md) | Periods, how cards flip, the rollup job, team and display timezones, daylight saving |
| [Realtime](realtime.md) | How a teammate's click reaches your screen. Written for readers with no realtime experience |
| [CI](ci.md) | The GitHub Actions workflow, and running it locally with act |
| [Production](production.md) | Deploying, operating, backing up, and the constraints that come with SQLite |

## Quick reference

```bash
mise install          # install the pinned Go and Node toolchains
mise run setup        # install dependencies
mise run db:reset     # fresh database: migrate, then seed demo data
mise run dev          # backend on :8090, frontend on :5173
mise run test         # backend + frontend tests
mise run check        # what CI runs: lint, test, build
mise tasks            # everything else
```

Sign in at http://localhost:5173 as `admin@example.test` or `dana@example.test`
with the password `kamishibai-dev-1234`.

## The short version of the architecture

One Go binary does everything: it serves the REST API, the realtime stream, the
static frontend, the scheduled rollup job, and the operator CLI. The database is a
single SQLite file. The frontend is React with no UI framework.

```
                    ┌─────────────────────────────────┐
   browser ────────▶│  kamishibai (one Go binary)     │
   React SPA        │                                 │
                    │  /                static assets │
   ◀── SSE stream ──│  /api/collections/*  PocketBase │
                    │  /api/kamishibai/*   custom      │
                    │  /api/realtime       SSE         │
                    │  cron                rollups     │
                    └──────────────┬──────────────────┘
                                   │
                            pb_data/data.db
```

The pieces worth knowing the names of:

- **Cards** are task definitions: a title, a cadence, instructions, links and
  steps. They hold no status.
- **Occurrences** are what happened to one card in one period. A row exists only
  if somebody touched it.
- **Rollups** are frozen completion statistics for periods that have closed.
- **Boards** belong to a team and hold cards. **Teams** are the tenant boundary.
