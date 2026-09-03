# Deploying the Kamishibai board

The whole application is one container: the Go binary serves the API, the static
frontend, the scheduled rollup job and the CLI, with SQLite on a mounted volume.

```
kubectl apply -k deploy/k8s
```

## First run

The database is created and migrated automatically when the pod starts. What is
not automatic is the first account, because there is no public signup.

Create a PocketBase superuser, then use its dashboard to create the first admin
user and team:

```
kubectl -n kamishibai exec deploy/kamishibai -- \
  /app/kamishibai superuser create you@example.com 'a-long-password' --dir=/app/pb_data
```

The superuser dashboard is at `/_/`. A superuser is an operator account: it
bypasses the application's access rules entirely, so use it to bootstrap and to
recover, not day to day. Ordinary work happens as a `users` record with the
`admin` role.

To load demo data into a non-production environment:

```
kubectl -n kamishibai exec deploy/kamishibai -- \
  /app/kamishibai seed --dir=/app/pb_data
```

Seeding is idempotent and only ever touches records matching its own dataset
(`*.example.test` accounts and its two named teams), but it is still a
development tool. Do not point it at production.

## Why one replica

`replicas: 1` and `strategy: Recreate` are both deliberate, and neither is safe
to change on its own.

SQLite allows a single writer. Two pods with the same database file open will
corrupt it, and a `RollingUpdate` briefly runs exactly that overlap. `Recreate`
trades a few seconds of downtime during a deploy for the guarantee that only one
process ever holds the file.

This is a reasonable trade for a small team: one binary, one file, a backup that
is a file copy. If you outgrow it, the change is to move the data to Postgres,
not to raise the replica count.

## Backups

The database is a single file on the PVC, so a backup is a copy:

```
kubectl -n kamishibai exec deploy/kamishibai -- \
  /app/kamishibai backup --dir=/app/pb_data
```

PocketBase writes the archive inside `pb_data/backups`, which is on the volume, so
copy it somewhere else to be useful. Prefer volume snapshots if your storage class
supports them.

Worth knowing what is and is not recoverable: the schema lives in the migrations
compiled into the binary, so a lost database can be rebuilt structurally from a
fresh deploy. What cannot be reconstructed is the history of who did what and
when. That is the data worth protecting.

## Configuration

Everything is in `config.yaml` as a ConfigMap.

| Key              | Meaning                                                        |
| ---------------- | -------------------------------------------------------------- |
| `timezone`       | The single timezone every period boundary is evaluated in       |
| `rollupCron`     | When the reporting snapshot job runs, in that timezone          |
| `rollupLookback` | How many closed periods it re-checks, so downtime self-heals    |

Changing `timezone` after data exists does not rewrite history. Past occurrences
keep the period keys they were filed under and rollups keep the boundaries they
were computed with, so a change shifts only future boundaries. That is the honest
behaviour, but it does mean a mid-year change leaves a visible seam in reports.

## Security notes

- The application authenticates every request itself, and the collection API
  rules scope reads and writes to the caller's teams. The Ingress deliberately
  adds no second auth layer.
- TLS is not optional. Auth tokens travel in an `Authorization` header on every
  request, including the realtime stream.
- There is no public signup: the `users` collection cannot be created through the
  API. Accounts are provisioned by an admin.
- The container runs as a non-root user with a read-only root filesystem and all
  capabilities dropped. The only writable path is the data volume.
- Occurrences and report rollups reject client writes outright. They are written
  only by server-side code, which is what makes "who completed this" trustworthy
  rather than merely reported.
