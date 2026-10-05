# Scheduling and time

Everything in this application is a question about time: which period is a card in,
has that period closed, was this card expected to be done back then. This document
covers how those questions are answered.

## Periods

A **period** is one occurrence window for a cadence: the day `2026-09-03`, or the
week `2026-W36`. It has a **key**, a **start** (inclusive) and an **end**
(exclusive).

| Cadence | Key format | Example | Starts |
| --- | --- | --- | --- |
| `daily` | ISO date | `2026-09-03` | Midnight |
| `weekly` | ISO week | `2026-W36` | Monday |
| `monthly` | year + month | `2026-M09` | 1st of the month |
| `quarterly` | year + quarter | `2026-Q3` | 1 Jan, 1 Apr, 1 Jul, 1 Oct |
| `annual` | year | `2026-Y` | 1 January |

The formats are chosen so that keys:

- **sort chronologically as strings** within a cadence, so `ORDER BY period_key`
  needs no date parsing;
- are **readable in a database client**, which matters more than it sounds when you
  are looking at an occurrences table trying to work out what happened;
- are **mutually exclusive**, so a key alone identifies its cadence. That is what
  lets `CadenceFromKey` reject a weekly key filed against a daily card.

All of this lives in `internal/domain`, which has no database dependency at all. It
is the most heavily tested package in the codebase, because period arithmetic is
where off-by-one and daylight-saving bugs live.

## How cards flip

**Nothing flips them.** This is the part that surprises people.

A card has no status column. Its state for a period is the occurrence row for that
`(card, period)` pair, and a row exists only if somebody touched it. So:

```
Friday 2026-09-04, 10:00
  weekly card "Review Failed Pipelines"
  current period = 2026-W36
  occurrence for (card, 2026-W36) exists, status = done
  → the board shows: done

Monday 2026-09-07, 09:00
  same card, nothing has run in between
  current period = 2026-W37        ← the key changed because time passed
  occurrence for (card, 2026-W37)? none
  → the board shows: not started
```

The flip is a **consequence of the clock**, not an action. No row was written, no
job ran, and last week's record is still on disk with its attribution intact.

The practical benefit is that there is no scheduled task in the path of
correctness. A reset job could fail, run late, or half-complete and leave the board
silently lying about what still needs doing. There is no such job to fail.

In code, the whole mechanism is `Calendar.At`:

```go
// internal/domain/period.go
func (cal *Calendar) At(cadence Cadence, t time.Time) (Period, error) {
	start := cal.startOf(cadence, t)
	return cal.periodFromStart(cadence, start), nil
}
```

and then a lookup keyed on the result.

## Timezone

There are two time zones in play, and they do different jobs.

| | Whose | Decides | Empty means |
| --- | --- | --- | --- |
| **Team zone** | Each team | Period keys, rollover, which period is writable, rollups, the activity heatmap's day buckets | Inherit the instance default |
| **Display zone** | Each user | How instants (started, completed, "closes at") are *rendered* for that person | Use the browser's zone |

### The team zone is the operational truth

Every boundary for a card is evaluated in the zone of the **team** that owns its
board, default `America/New_York` (the instance default, set by
`KAMISHIBAI_TIMEZONE`). A team can set its own, so a team in Tokyo flips its
daily cards at Tokyo midnight rather than New York's.

The reasoning is the same as when there was one zone for the whole instance, just
applied per team: a card that flips "on Monday" has to flip at the same instant for
everybody working that board, or two people disagree about whether today's work is
done. One board, one clock. What changed is that the clock belongs to the team
rather than the deployment.

The same instant can therefore be different days for different teams:

```
2026-10-05 20:00 in New York  =  2026-10-06 09:00 in Tokyo

NY team's daily card     → 2026-10-05
Tokyo team's daily card  → 2026-10-06
```

`domain.Calendars` resolves a zone name to a `Calendar` (an empty name gives the
default, and each name is loaded once and cached), and `internal/teamcal` joins that
to a team record. Everything that asks "which period" goes through it:
`occurrence.Service` (`StatesFor` loads the cards' teams in one query,
`mutateOnce` already holds the team), `rollup.Service.Run` (each board is walked in
its own team's calendar), and the board, report and activity endpoints, which report
the team's **effective** zone as `timezone`.

A stored team zone that fails to load is an **error**, never a fallback to the
default or UTC. The hooks validate zones on the way in, so it can only happen
through a bug or a hand edit of the database, and quietly using another zone would
shift that team's boundaries without anyone noticing.

#### Inheritance

A team with an empty zone uses the instance default, so every team that existed
before zones did behaves exactly as it did. There is no backfill. A malformed
`KAMISHIBAI_TIMEZONE` is still a **startup error** rather than a fallback to UTC,
for the same reason.

### The display zone changes nothing the server computes

A user's zone only changes how an instant is written in their browser, using
`Intl.DateTimeFormat`'s `timeZone` option. It never changes a period key, a card's
status, a report bucket or who may do what. Anyone can set their own (the `users`
update rule already allows editing your own record); empty means the browser's own
zone.

Period keys are **not** instants, so no display zone is applied to them. `2026-10-05`
is labelled as 5 October by building the date in UTC and formatting it in UTC; a
viewer in a zone far to the east or west cannot shift it to the 4th or the 6th.

The board banner ties the two together: "Periods roll over at midnight,
Asia/Tokyo (11:00 your time)", as a viewer in New York would read it, names the
team's zone and, when yours differs, what that midnight is on your clock.

### The IANA database is compiled in

The IANA timezone database is **compiled into the binary**:

```go
// internal/domain/tzdata.go
import _ "time/tzdata"
```

Without that, `time.LoadLocation("America/New_York")` depends on the host having
system tzdata, which the distroless container image does not. The failure mode would
be silently computing UTC boundaries in production while working correctly on a
developer's Mac — a bug worth a few hundred KB of binary to eliminate.

`domain.ValidTimezone` is what the hooks use for team and user zones. It rejects
unknown names, the empty string (which `LoadLocation` would treat as UTC) and
`"Local"` (whatever zone the server process happens to run in).

### Changing a team's zone, or moving a board

Only admins change a team's zone. Doing so does not rewrite history: existing
occurrences keep the period keys they were filed under, and rollups keep the
boundaries they were computed with. Only future boundaries move. That is the honest
behaviour, but it does mean a change leaves a visible **seam** in that team's
reports, and the team dialog says so before saving.

Moving a board to a team in a different zone has the same effect for that board,
and the move confirmation carries the same warning. Moves between teams in the same
zone are seamless.

Changing `KAMISHIBAI_TIMEZONE` behaves the same way for every team that inherits it.

## Daylight saving

Boundaries are always **local midnight**, which means a day is not always 24 hours.

| Date (US Eastern, 2026) | Length |
| --- | --- |
| 8 March (spring forward) | 23 hours |
| 1 November (fall back) | 25 hours |
| Week of 2 March | 167 hours |

Getting this right comes down to never adding a fixed duration. Every boundary is
rebuilt through `time.Date`:

```go
// internal/domain/period.go — advance()
case Daily:
	return time.Date(y, mo, d+n, 0, 0, 0, 0, cal.loc)
case Monthly:
	return time.Date(y, mo+time.Month(n), 1, 0, 0, 0, 0, cal.loc)
```

`time.Date` normalises out-of-range values, so `d+1` on the 31st rolls into the next
month and the location handles the DST offset. Adding `24 * time.Hour` instead would
drift by an hour twice a year and eventually land a "daily" period on the wrong
calendar day.

`assertLocalMidnight` in the tests checks this invariant directly.

## ISO weeks

Weekly periods use ISO-8601 weeks, which start on Monday. The wrinkle is that the
**ISO week-numbering year is not always the calendar year**:

- 1 January 2026 is a Thursday, so it falls in ISO week 1 of 2026 — and that week
  starts on **29 December 2025**. Both dates share the key `2026-W01`.
- 1 January 2027 is a Friday, and belongs to `2026-W53`.
- 2026 has 53 ISO weeks. 2025 has 52, so `2025-W53` does not exist and `FromKey`
  rejects it.

A weekly card spanning New Year therefore stays on one key for the whole week, which
is what you want: it is one week's work, not two.

Reconstructing a week's Monday from its key uses the fact that 4 January is always in
ISO week 1:

```go
// internal/domain/period.go
func (cal *Calendar) isoWeekStart(isoYear, isoWeek int) time.Time {
	jan4 := time.Date(isoYear, time.January, 4, 0, 0, 0, 0, cal.loc)
	week1Monday := time.Date(isoYear, time.January, 4-mondayOffset(jan4.Weekday()), 0, 0, 0, 0, cal.loc)
	y, mo, d := week1Monday.Date()
	return time.Date(y, mo, d+7*(isoWeek-1), 0, 0, 0, 0, cal.loc)
}
```

`FromKey` then verifies the round trip, which is how `2025-W53` gets caught.

## The server owns period keys

The frontend never computes a period key. It asks:

```
GET /api/kamishibai/periods/current
GET /api/kamishibai/periods/current?team=<teamId>
```

Without `team` the answer is for the instance default zone; with it, for that team's
(the caller must be able to read the team, otherwise it is a 404). Board state,
report and activity responses already use their own team's zone.

```json
{
  "timezone": "America/New_York",
  "serverAt": "2026-09-03T14:22:07-04:00",
  "periods": {
    "daily":     { "key": "2026-09-03", "start": "...", "end": "2026-09-04T00:00:00-04:00" },
    "weekly":    { "key": "2026-W36",   "start": "...", "end": "2026-09-07T00:00:00-04:00" },
    "monthly":   { "key": "2026-M09",   "start": "...", "end": "2026-10-01T00:00:00-04:00" },
    "quarterly": { "key": "2026-Q3",    "start": "...", "end": "2026-10-01T00:00:00-04:00" },
    "annual":    { "key": "2026-Y",     "start": "...", "end": "2027-01-01T00:00:00-04:00" }
  }
}
```

A second implementation of ISO week numbering in the browser would eventually
disagree with the first, and the symptom would be two people looking at different
days' work. One implementation, one source of truth.

The board's own state response embeds the same period objects per card, in the
board's team zone, so drawing a board needs no extra call.

## The one scheduled job: rollups

There is exactly one cron job, and it is deliberately **not** load-bearing.

### Why it exists

The lazy model makes writes cheap and reporting awkward. You cannot count work that
was never done, because it has no rows. And every historical query would have to
re-derive its own denominator: how many cards of this cadence existed on this board
back then, and were they active?

So once a period closes — and its contents can no longer change, because mutations
are restricted to the current period — the numbers are computed once and frozen into
`report_rollups`: one row per board, cadence and period.

| | Written by | Read by | Grows with |
| --- | --- | --- | --- |
| `occurrences` | People, lazily | The board | Work actually recorded |
| `report_rollups` | The cron job | Reporting | Boards × cadences × periods |

### What it does

`internal/rollup`:

- **`Compute`** derives a snapshot for one board, cadence and period.
- **`Persist`** upserts it, keyed by `(board, cadence, period_key)`.
- **`Run`** walks every board and cadence over the last *N* closed periods and
  ensures each has a snapshot.

The interesting part of `Compute` is the denominator. It counts cards that **actually
applied during that period**, not cards that exist now:

```go
total, err := app.CountRecords(
	schema.Cards,
	dbx.HashExp{schema.FieldBoard: board.Id, schema.FieldCadence: string(cadence)},
	dbx.NewExp(schema.FieldCreated+" < {:end}", dbx.Params{"end": end.String()}),
	dbx.NewExp("("+schema.FieldArchivedAt+" = '' OR "+schema.FieldArchivedAt+" IS NULL OR "+
		schema.FieldArchivedAt+" > {:start})", dbx.Params{"start": start.String()}),
)
```

Without those two clauses, adding a card today would retroactively make last month
look worse, and archiving one would make last month look better. Both would quietly
falsify every historical percentage whenever the board changed shape.

`not_started` is then **derived**, because it has no rows to count:

```go
snap.NotStarted = snap.TotalCards - snap.Done - snap.InProgress
```

### Scheduling

```go
// internal/app/app.go
app.Cron().SetTimezone(cfg.Calendar.Location())
app.Cron().MustAdd(RollupJobID, cfg.RollupCron, func() { service.Run(app, cfg.RollupLookback) })
```

The default schedule is `10 * * * *`: ten minutes past **every hour**. Every cadence
rolls over at local midnight, but teams can have their own zones, so some team's
midnight falls in almost every hour of the day. Running hourly means each team's
closed periods are rolled up within about an hour of its own midnight, rather than
waiting for the instance default's. `Run` walks each board in its team's calendar,
so a period that has closed in Tokyo is rolled up for Tokyo's boards while New
York's, still open, is left until it closes there.

The extra runs are cheap: the job is an idempotent upsert over a bounded window (see
below), and most runs find nothing new. It is also safe if a run is skipped or
doubled.

`SetTimezone` still matters if you set `KAMISHIBAI_ROLLUP_CRON` to something in local
terms (say `10 0 * * *`). Without it the schedule would follow the container's
clock, which is usually UTC. Note that a once-a-day schedule only suits an instance
whose teams all share the default zone; the job would still be correct for the
others, because it only ever summarises periods that have already closed, but their
snapshots would arrive late.

### Self-healing

`Run` re-checks the last `RollupLookback` closed periods (default 14) on every run,
not just the most recent one. If the process was down over a weekend, or a period
closed during a deploy, the next run fills the gap instead of leaving a permanent
hole in the history.

This is safe because snapshots are idempotent: the unique index on
`(board, cadence, period_key)` means re-running updates in place rather than
duplicating. `TestRunBackfillsAfterDowntime` covers exactly this.

### Empty snapshots are skipped

A board with only daily cards should not accumulate weekly, monthly, quarterly and
annual rows forever. `Snapshot.Empty()` is true when the board had no cards of that
cadence and no work was recorded, and those are not stored.

### If the job never runs

Reporting still works. For a closed period with no snapshot, the API computes it live
and labels the result:

```json
{ "periodKey": "2026-09-02", "completionRate": 0.66, "source": "live" }
```

The reporting screen shows a `live` badge on those rows. `source: "rollup"` means the
figure is frozen and cannot change; `source: "live"` means it was computed on request.
The period currently in progress is always `live`, by definition.

So a failed cron job degrades to slower reports, not wrong or missing ones.

### Running it by hand

```bash
mise run rollup                               # local
go run . rollup --lookback 90                 # wider window, e.g. after an import

kubectl -n kamishibai exec deploy/kamishibai -- \
  /app/kamishibai rollup --lookback 30 --dir=/app/pb_data
```

Safe to run at any time.

## Closed periods are immutable

Mutations only ever apply to the period that is currently open. Completing a card you
forgot on Friday is refused with a clear error.

Two reasons. Frozen rollups are only trustworthy if their inputs cannot change after
the fact. And a kamishibai board is meant to record the ritual as it happened, not as
somebody later wished it had.

This is the constraint most likely to need revisiting for a given team. It is
enforced in one place — `internal/occurrence.mutateOnce` derives the period from the
server clock and never accepts one from the caller — so relaxing it means allowing a
caller-supplied period key there, and then deciding what should happen to any rollup
already covering it.

## Live updates in the browser

Two mechanisms, because they solve different problems.

**A teammate acting** is a database write, so it produces a realtime event and the
board refetches. See [Realtime](realtime.md).

**A period rolling over is not a write.** Nothing changes in the database, so there
is nothing to broadcast — that is the whole point of the lazy model. The client
therefore schedules its own refresh from the boundaries the server gave it:

```ts
// frontend/src/boards/useBoardState.ts
const periods = Object.values(state.periods).filter((p) => p !== undefined);
const ms = msUntilNextBoundary(periods);
if (ms === null) return;

// A second of slack so the server has definitely crossed the boundary.
const timer = window.setTimeout(refresh, ms + 1_000);
return () => window.clearTimeout(timer);
```

A board left open overnight flips by itself in the morning. The one second of slack
avoids a refetch landing microseconds early on a clock skew and getting the period
that is about to end.

## Testing time

Both services take an injectable clock, and tests should always use it. A test that
depends on the real clock passes all day and fails at midnight.

```go
cal := config.Default().Calendar
frozen := time.Date(2026, 9, 3, 14, 0, 0, 0, cal.Location())
svc := occurrence.NewService(cal).WithClock(func() time.Time { return frozen })

// later
tomorrow := svc.WithClock(func() time.Time { return frozen.AddDate(0, 0, 1) })
```

`NewService(cal)` gives every team the one calendar. To test teams in different zones,
build the service with `NewServiceWithCalendars(domain.NewCalendars(cal))` and give
the teams zones with `testutil.NewTeamInZone`.

See [Testing](testing.md#testing-time-dependent-behaviour) for the dates with useful
properties.

## Reference

| Function | Answers |
| --- | --- |
| `Calendars.For(zone)` | The calendar for a zone name; empty gives the instance default |
| `teamcal.For(cals, team)` | The calendar a team's periods are evaluated in |
| `Calendar.At(cadence, t)` | Which period contains `t` |
| `Calendar.Current(cadence)` | Which period we are in now |
| `Calendar.Next` / `Previous` / `Shift` | Neighbouring periods |
| `Calendar.Between(cadence, from, to)` | Every period overlapping a range |
| `Calendar.ClosedBefore(cadence, now, n)` | The last `n` fully elapsed periods, newest first |
| `Calendar.FromKey(key)` | Rebuild a full period from a key, rejecting impossible ones |
| `Calendar.ValidateKey(cadence, key)` | Is this key well-formed *for this cadence* |
| `Period.Contains(t)` | Is `t` inside |
| `Period.IsClosed(now)` | Has it fully elapsed |

`Between` refuses ranges covering more than 10,000 periods, so a bad reporting
request fails loudly instead of allocating without bound.
