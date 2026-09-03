# Running in production

The whole application is one container: the Go binary serves the REST API, the
realtime stream, the static frontend, the scheduled rollup job and the operator CLI.
The database is a single SQLite file on a mounted volume.

## Deploying

```bash
mise run docker:build                  # or: docker build -t kamishibai:local .
kubectl apply -k deploy/k8s
```

Set the image before deploying for real. Override the tag in
`deploy/k8s/kustomization.yaml` rather than editing the Deployment:

```yaml
images:
  - name: ghcr.io/dbowling/kamishibai
    newTag: 2026.09.03-a1b2c3d
```

Check what a change would do first:

```bash
mise run k8s:render      # kubectl kustomize deploy/k8s
mise run k8s:diff        # kubectl diff -k deploy/k8s
```

There is **no separate migration step**. `serve` applies pending migrations before it
starts listening, so deploying a new image is the migration. The `startupProbe`
allows up to two minutes for that before liveness starts counting.

## First run

The database is created and migrated automatically. What is not automatic is the
first account, because there is no public signup.

Create a PocketBase superuser:

```bash
kubectl -n kamishibai exec deploy/kamishibai -- \
  /app/kamishibai superuser upsert you@example.com 'a-long-password' --dir=/app/pb_data
```

Then open `/_/`, and from that dashboard create:

1. A `users` record with `role = "admin"` — your day-to-day account.
2. A team, with that user in `members`.
3. A board for the team.

From then on, use the admin account and the application itself. A superuser bypasses
every application access rule; it is an operator account for bootstrapping and
recovery, not for daily work. See [Permissions](permissions.md#admins-are-not-superusers).

## The one-replica constraint

`deploy/k8s/deployment.yaml` sets:

```yaml
replicas: 1
strategy:
  type: Recreate
```

**Both are load-bearing, and neither is safe to change on its own.**

SQLite permits a single writer. Two processes with the same database file open will
corrupt it. `replicas: 1` prevents the obvious case; `strategy: Recreate` prevents the
subtle one, because a `RollingUpdate` deliberately starts the new pod before stopping
the old one, which is exactly the overlap that must not happen.

`Recreate` trades a few seconds of downtime during a deploy for the guarantee that
only one process ever holds the file. For a small team's triage board that is the
right trade: one binary, one file, a backup that is a file copy.

The PVC is `ReadWriteOnce` for the same reason. Do not widen it to `ReadWriteMany`
hoping to scale out — SQLite over a shared network filesystem is a well-known way to
corrupt a database, because its locking relies on POSIX semantics that most shared
volumes do not honour.

**If you outgrow this**, the change is to move the data to Postgres, not to raise the
replica count. That is a real project, not a config edit.

## Configuration

All in the ConfigMap in `deploy/k8s/config.yaml`.

| Key | Env var | Default | Meaning |
| --- | --- | --- | --- |
| `timezone` | `KAMISHIBAI_TIMEZONE` | `America/New_York` | The single timezone all period boundaries are evaluated in |
| `rollupCron` | `KAMISHIBAI_ROLLUP_CRON` | `10 0 * * *` | When the reporting snapshot job runs, in that timezone |
| `rollupLookback` | `KAMISHIBAI_ROLLUP_LOOKBACK` | `14` | How many closed periods it re-checks, so downtime self-heals |

A malformed timezone is a **startup failure**, not a silent fallback to UTC. That is
intentional: a typo would otherwise shift every boundary in the system without anyone
noticing.

**Changing the timezone after data exists does not rewrite history.** Existing
occurrences keep the period keys they were filed under, and rollups keep the
boundaries they were computed with. Only future boundaries move, which means a
mid-year change leaves a visible seam in reports. Decide this before you start
recording work if you can.

## The container

Three stages, in `Dockerfile`:

1. **Node** builds the frontend into `backend/pb_public`.
2. **Go** runs `go vet ./... && go test ./...`, then builds a static binary. A
   container that cannot pass its own tests never gets tagged.
3. **`gcr.io/distroless/static-debian12:nonroot`** carries the binary and the assets.

The final image is about 28 MB. It has no shell, no package manager and no libc, so
there is very little to exploit and very little to patch.

Two details that make that possible:

- **`CGO_ENABLED=0`.** PocketBase uses a pure Go SQLite port, so no libc is needed.
- **The timezone database is compiled in** via `import _ "time/tzdata"`. Without it,
  `LoadLocation("America/New_York")` would fail on an image with no
  `/usr/share/zoneinfo`, and period boundaries would silently be computed in UTC.

### Security posture

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 65532
  fsGroup: 65532
  seccompProfile: { type: RuntimeDefault }
containers:
  - securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities: { drop: ["ALL"] }
```

The only writable path is the data volume.

`fsGroup: 65532` is required, not decorative. Without it the mounted volume is
root-owned and a non-root process cannot create the database file — the container
exits with `unable to open database file`. The image also ships `/app/pb_data` owned
by 65532 so plain `docker run` works too, since Docker seeds a fresh volume's
ownership from whatever is at the mount point in the image.

### Running the image locally

Useful for verifying a build before deploying:

```bash
mise run docker:run        # port 8090, named volume kamishibai-local-data
```

## Ingress and TLS

`deploy/k8s/ingress.yaml` assumes nginx and cert-manager. Adjust for your controller,
but keep these properties.

**TLS is not optional.** Auth tokens are sent in an `Authorization` header on every
request, including the realtime stream. Over plain HTTP they are on the wire in clear
text.

**The realtime stream needs proxy buffering off and a long read timeout:**

```yaml
nginx.ingress.kubernetes.io/proxy-buffering: "off"
nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
nginx.ingress.kubernetes.io/proxy-send-timeout: "3600"
```

A buffering proxy holds SSE messages waiting for a response that never finishes, so
the stream looks connected and stays silent. A short read timeout cuts it every
minute. Both are the usual cause of "realtime works locally but not in the cluster".
See [Realtime](realtime.md#proxies-that-buffer).

**No auth layer is added at the ingress**, deliberately. The application
authenticates every request itself and scopes reads and writes to the caller's teams.

## Health and probes

PocketBase serves `/api/health`.

| Probe | Purpose |
| --- | --- |
| `startupProbe` | Up to 2 minutes for migrations to finish on boot |
| `readinessProbe` | Gates traffic |
| `livenessProbe` | Restarts a wedged process; deliberately slack, since a restart mid-write is worse than a slow response |

## Backups

The database is one file, so a backup is a copy. The binary has a `backup` command
that produces a consistent archive while the application is running:

```bash
kubectl -n kamishibai exec deploy/kamishibai -- \
  /app/kamishibai backup --dir=/app/pb_data
```

(This is our command, not a PocketBase built-in. PocketBase exposes backups through
its dashboard and its HTTP API, both of which need a superuser session; the image has
no shell and no browser, so a CLI command is what makes `kubectl exec` viable.)

That writes into `pb_data/backups`, **which is on the same volume**. It protects
against an application-level mistake, not against losing the volume. Copy it
elsewhere or it is not really a backup:

```bash
kubectl -n kamishibai exec deploy/kamishibai -- ls /app/pb_data/backups
kubectl -n kamishibai cp kamishibai/<pod>:/app/pb_data/backups/<file>.zip ./<file>.zip
```

Volume snapshots are better if your storage class supports them.

### What is and is not recoverable

Worth being clear about, because it tells you what to protect.

- **The schema** lives in the migrations, compiled into the binary. A lost database
  can be rebuilt structurally by deploying a fresh one.
- **The card definitions** are re-creatable by hand, tediously.
- **The occurrence history** — who did what, and when — cannot be reconstructed from
  anything. That is the data worth protecting.
- **Rollups are derived.** If they are lost, run `rollup --lookback N` to rebuild
  them from the occurrences.

## Operations

### Recompute reporting snapshots

Safe at any time; snapshots are keyed by board, cadence and period, so re-running
converges rather than duplicating.

```bash
kubectl -n kamishibai exec deploy/kamishibai -- \
  /app/kamishibai rollup --lookback 30 --dir=/app/pb_data
```

Useful after an import, or if the pod was down when a period closed. The scheduled job
already re-checks the last 14 closed periods on every run, so routine gaps heal
themselves.

### Checking the cron job

The job logs a summary on each run:

```bash
kubectl -n kamishibai logs deploy/kamishibai | grep rollup
# rollup job finished job=kamishibai-rollups boards=3 periodsChecked=210 created=5 updated=139 skipped=66
```

It can also be inspected and triggered from **Dashboard → Settings → Crons**.

If it fails, reporting degrades to computing windows on request and labelling them
`live`. Nothing breaks, and cards still flip, because the flip does not depend on any
job. See [Scheduling and time](scheduling.md#if-the-job-never-runs).

### Adding a user

Via the app as an admin, or from the dashboard. There is no self-service signup: the
`users` collection has a `nil` create rule, so only superusers and the CLI can create
accounts.

### Inspecting the database

No shell in the image, so copy the file out:

```bash
kubectl -n kamishibai cp kamishibai/<pod>:/app/pb_data/data.db ./data.db
sqlite3 ./data.db 'select cadence, count(*) from occurrences group by cadence;'
```

Take a copy rather than opening the live file; a second writer is exactly what the
single-replica constraint exists to prevent.

### Do not seed production

`seed` is a development tool. It only touches its own dataset — two named teams and
`*.example.test` accounts — but it exists to make demo data, not to manage real data.

## Capacity

The lazy occurrence model means storage tracks **work recorded**, not time elapsed.

For a team of ten with three boards and forty cards, mostly daily:

| Table | Rows per year | Roughly |
| --- | --- | --- |
| `occurrences` | ~8,000 (one per card actually worked) | a few MB |
| `report_rollups` | ~1,500 (boards × cadences × periods) | under a MB |
| everything else | tens of rows | negligible |

The 2 Gi PVC is generous by a wide margin. A card nobody ever touches contributes
nothing at all.

The queries that matter are all index-backed: the board view is one query using
`(board, period_key)`, and reporting aggregates use `(team, cadence, period_key)` or
read the rollup table directly.

## If something goes wrong

**Pod will not start, `unable to open database file`** — volume ownership. Confirm
`fsGroup: 65532` is present in the pod securityContext.

**Pod restarts in a loop during a deploy** — check for a failing migration in the
logs. Migrations run before the listener starts, so a broken one prevents startup.
Roll back to the previous image; the schema change will not have been committed
because migrations are transactional.

**Realtime silent in the cluster but fine locally** — proxy buffering. See above.

**Reports show `live` on every row** — the rollup job is not running. Check its logs
and run it by hand.

**Board is empty for a user** — they are probably not on a team. Check `members` on
the relevant team.

**Everything is slow** — check whether the memory limit is being hit. There is no CPU
limit by design, because throttling SQLite writes causes latency spikes and the pod is
alone on its volume anyway.
