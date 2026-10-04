# Overview

## What this is for

Every team has work that has to happen again and again: check the backups
completed, triage overnight alerts, review access, run a disaster recovery drill.
The work itself is usually straightforward. The hard part is that it is easy to
skip, easy to skip quietly, and hard to prove afterwards whether it happened.

Kamishibai (紙芝居, "paper theatre") boards come from factory floors, where routine
checks were tracked with a wall of two-sided cards. Turned one way a card showed
the check was outstanding; turned the other, done. Anyone walking past could see
the state of the whole routine in a second, and at the end of the shift every card
was turned back over.

This is that board, for a software team, with the turning-back-over automated and
the history kept.

## What it does

**Recurring cards on five cadences.** Daily, weekly, monthly, quarterly and
annual. A card resets to not-started when its period rolls over: daily at
midnight, weekly on Monday, monthly on the 1st, quarterly on the 1st of January,
April, July and October, annually on 1 January.

**A board everyone shares.** There is one board state, not one per person. When a
teammate marks a card done, it changes on your screen within a second, without a
reload. See [Realtime](realtime.md).

**Attribution.** Every occurrence records who started it and who completed it,
separately, because those are often different people. Attribution comes from the
authenticated request and cannot be supplied by the client, so it is trustworthy
rather than merely reported.

**Cards carry the work, not just its name.** A card holds rich-text instructions,
a list of quick links, and a checklist of steps. The "Verify Backups" card links
straight to the Grafana backup dashboard and lists the three things to try when a
backup has failed. Somebody picking it up at 8am should not have to go hunting
through a wiki.

**Reporting per task and per period.** What percentage of daily cards were
completed last month? Which specific card is being skipped most? Both are answered
from the same reporting screen, over a window you choose. A calendar heatmap of
completions per day, filterable by cadence, sits above them.

**Multiple teams.** Users can belong to several. Each team owns its boards, and a
board's cards are only visible to that team. Admins can see everything.

**Anyone on a team can add a card.** This is deliberate. A board that requires an
administrator to add a check is a board that stops reflecting what the team
actually does.

**Nothing is ever deleted.** Teams, boards and cards are archived, and an admin can
restore them. Archived items stay readable so history and reporting survive.

## The one design decision that shapes everything

**A card has no status column.**

That sounds like a detail. It is the reason the rest of the system looks the way it
does, so it is worth understanding early.

The obvious way to build this is a `status` field on each card and a nightly job
that resets them. That works, and it has three problems. The job can fail, run
late, or half-complete, and when it does the board silently lies. History needs a
separate audit table. And a card that nobody ever touches still gets written to
every single day.

Instead, a card's state for a period is the **occurrence row** for that
`(card, period)` pair — and a row exists **only if somebody touched it**.

The consequences:

- **"Not started" is the absence of a row.** Nothing is stored for work that has
  not been picked up.
- **A daily card nobody starts for a year costs zero rows**, not 365.
- **The flip requires no write at all.** On Monday, the weekly period key changes
  from `2026-W36` to `2026-W37`. No row exists for the new key, so every weekly
  card reads as not-started. Nothing was updated. No job ran. The board flipped
  because time passed.
- **History is automatic.** Last week's occurrence row is still there, with its
  attribution intact. There is no separate audit log to keep in sync.

That last point about the flip is the one that matters operationally. There is no
scheduled task in the path of correctness, so there is nothing to page anyone about
at midnight.

### What it costs

Sparse data is cheap to write and awkward to report on. You cannot count work that
was never done, because it has no rows, and every historical query would have to
re-derive its own denominator ("how many cards *should* have been done that week?").

So there is a second table. Once a period closes, and its contents can no longer
change, the counts are computed once and frozen into `report_rollups`: one row per
board, cadence and period. Historical reporting reads that small dense table. The
still-open period is computed live and labelled as such, so you can tell a settled
number from a moving one.

This is the only scheduled job in the system, and it is deliberately not
load-bearing: if it is late, reports fall back to computing the window on request.
See [Scheduling and time](scheduling.md).

## Data model

```
teams ──┬── boards ──┬── cards ──── occurrences
        │            │                  │
        │            └── report_rollups ┘
        └── members (many-to-many with users)
```

| Collection | Holds | Grows with |
| --- | --- | --- |
| `users` | People, plus a `role` of `user` or `admin` | Team size |
| `teams` | The tenant boundary, with a `members` list | Number of teams |
| `boards` | Belong to a team, hold cards | Number of boards |
| `cards` | Task definitions: title, cadence, instructions, links, checklist | Number of checks |
| `occurrences` | What happened to one card in one period | **Work actually recorded** |
| `report_rollups` | Frozen stats for closed periods | Boards × cadences × periods |

`occurrences` is the only table that grows with time, which is why it is the one
with the careful design. `board`, `team` and `cadence` are denormalised onto it so
reporting aggregates never have to join back up through `cards` and `boards`.

## Deliberate constraints

These will look like missing features until you know why they are there.

**Only the current period can be changed.** Completing a card you forgot on Friday
is refused. Two reasons: closed periods feed the frozen rollups, and a kamishibai
board is meant to reflect the ritual as it actually happened. This is the
constraint most likely to need revisiting for your team; it is a small change in
`internal/occurrence`.

**Occurrences and rollups reject client writes outright.** All recording goes
through purpose-built endpoints that accept nothing but an optional note. The
server decides which period is open and stamps who did it. If clients could POST
to the collection directly, attribution and period could both be forged.

**Nothing moves between teams.** A card's team is derived from its board and cannot
be changed afterwards, because its occurrence history is attributed to the original
team.

**No public signup.** Accounts are provisioned by an admin. This is internal team
tooling.

**One replica.** The database is SQLite, which allows a single writer. See
[Production](production.md).

## Where to go next

- To run it: [Development](development.md)
- To understand the flip and the rollup job in detail: [Scheduling and time](scheduling.md)
- To understand who can do what: [Permissions](permissions.md)
