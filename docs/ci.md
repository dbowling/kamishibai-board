# Continuous integration

The workflow is `.github/workflows/ci.yml`. It runs on every pull request and on
every pushed commit, and it can be executed on your own machine with
[act](https://github.com/nektos/act) so a workflow change does not need a
push-and-wait cycle to test.

## What runs

Five jobs in parallel, then a gate.

| Job | Steps |
| --- | --- |
| `backend` | `go vet`, `go test -race`, build the binary |
| `frontend` | `npm ci`, typecheck, Vitest, build the bundle |
| `storybook` | When frontend files changed: `npm ci`, install headless Chromium, render every story as a browser test, build Storybook |
| `manifests` | Render the Kubernetes manifests with kustomize |
| `container` | Build the production image (which itself runs the Go tests) |
| `ci` | Depends on all five; fails unless every one succeeded |

`ci` exists so branch protection can require **one** check. Adding a sixth job later
does not mean reconfiguring the rule.

Note that `ci` treats anything other than `success` as a failure, including
`cancelled` and `skipped`. A gate job that only checks for `failure` reports green
when a dependency was skipped, which is exactly when you least want a green tick.

### Why Storybook is its own job

The story tests need a real browser, which the `frontend` job (typecheck, jsdom
tests, build) does not otherwise pay for. A separate job runs the two in parallel
and shows up as its own failing check when a story breaks. It also builds
Storybook, so a story that renders in the test runner but breaks the static build
is still caught.

Two things keep it cheap:

- **It only does work when it matters.** Its first step diffs the commit against
  its base and skips the remaining steps unless something under `frontend/` (other
  than a `*.test.ts(x)` file), `.mise.toml` or this workflow changed. Skipped steps
  still leave the job green, so the `ci` gate needs no special case. A
  workflow-level `paths:` filter was deliberately not used: on a backend-only pull
  request it would never report the check at all, and a required check that never
  reports blocks the merge. Events it cannot diff reliably (a manual run, a new
  branch, a force push, act) run the full job.
- **It installs only the headless shell.** `npx playwright install --only-shell
  chromium` fetches about 100 MB in a few seconds. The hosted runner image already
  carries Chromium's system libraries, so `--with-deps` (an apt update and install)
  is only added under act.

### Toolchains come from mise

Every job uses `jdx/mise-action`, which reads `.mise.toml` and installs the pinned
Go and Node versions. The steps then call the same mise tasks you run locally:

```yaml
- name: Test
  run: mise run backend:test:race
```

The point is that CI and a developer's machine cannot drift onto different Go
versions, and a test command cannot be defined twice with two different sets of
flags. If `mise run backend:test:race` passes locally, it is the same command CI
runs.

`kubectl` is deliberately **not** in `[tools]`. The `k8s:render` task pulls it on
demand:

```toml
run = "mise exec kubectl@1.36 -- kubectl kustomize deploy/k8s"
```

That way a frontend-only contributor running `mise install` does not download
kubectl, and the CI runner needs nothing preinstalled.

### Why `-race` in CI

`backend:test:race` rather than `backend:test`. The occurrence service resolves
concurrent writers with a retry loop around a unique index, and there is a test that
fires eight simultaneous completions at one card. That is precisely the code where a
data race would hide, and the race detector is cheap enough here to leave on.

## Triggers

```yaml
on:
  pull_request:
  push:
    branches: ['**']
    tags-ignore: ['**']
  workflow_dispatch:
```

A `concurrency` group supersedes in-flight runs for the same ref, so pushing three
times in a minute leaves one run instead of three. The event name is part of the
group key, so a push run and a pull request run never cancel each other.

**One tradeoff to be aware of.** Pushing to a branch that already has an open pull
request produces **two runs**, one per event. That is the cost of literally testing
every pushed commit, as asked for. If you would rather not pay it:

```yaml
push:
  branches: [main]
```

Same-branch pushes are then covered by their pull request instead, and you still get
a run on every commit that lands on `main`.

## Running it locally with act

`act` reads the workflow and executes each job in a Docker container, so what runs
locally is the workflow itself rather than an approximation of it.

### One-time setup

```bash
brew install act jq
```

Plus a running Docker daemon (Docker Desktop, OrbStack, Colima). `mise run ci:local`
checks for all of these and tells you what is missing.

### Usage

```bash
mise run ci:list                        # list the jobs and their ids
mise run ci:local                       # the whole workflow, as a pull request
mise run ci:local -- --job backend      # one job
mise run ci:local -- --job frontend
mise run ci:local -- --job storybook
mise run ci:push                        # as a push event instead
mise run ci:local -- --base develop     # compare against a different base branch
mise run ci:local -- --dryrun           # show what would run, without running it
mise run ci:local -- --verbose          # unrecognised flags pass through to act
```

Expect the first run to be slow: act pulls a runner image of about a gigabyte, and
then `mise-action` installs Go and Node inside it. Later runs reuse both.

### Why a synthesised event payload

`act push` works out of the box, because a push is just a ref and a commit and both
exist locally. A pull request does not: there is no PR on your machine, so there is
nothing for act to read. Without a payload, `github.event.pull_request.*` is empty
and anything keyed on it behaves differently than it would on GitHub.

So `ci:local` builds one from the current branch, which is the same approach as the
snippet this was modelled on:

```bash
jq -n \
  --arg head_ref "$(git rev-parse --abbrev-ref HEAD)" \
  --arg head_sha "$(git rev-parse HEAD)" \
  --arg base_ref "main" \
  --arg base_sha "$(git merge-base main HEAD)" \
  '{
    "action": "synchronize",
    "number": 1,
    "pull_request": {
      "number": 1,
      "state": "open",
      "draft": false,
      "title": "Local CI run",
      "head": {"ref": $head_ref, "sha": $head_sha},
      "base": {"ref": $base_ref, "sha": $base_sha}
    }
  }'
```

Two deliberate differences from that original snippet:

**`git merge-base main HEAD` instead of `git rev-parse main`.** The base of a pull
request is the commit the branch diverged from, not the current tip of `main`. If
`main` has moved on since you branched, `rev-parse` describes a comparison GitHub
would never make. Using the merge base keeps the payload honest, and matters as soon
as a workflow starts diffing against `base.sha` to decide what changed.

**Written to a temporary file rather than passed through `<(...)`.** Process
substitution gives act a `/dev/fd` entry that can only be read once, and act opens
the event path more than one time. A `mktemp` file with a `trap` to clean it up works
regardless. The script does not depend on the shell supporting process substitution
either.

### The guard against running inside CI

Every `ci:*` task starts with:

```bash
if [ "${GITHUB_ACTIONS:-}" = "true" ]; then
  echo "Skipping: already running inside GitHub Actions"
  exit 0
fi
```

Without it, a workflow that ever invoked one of these tasks would try to start
Docker containers on the runner to re-execute the same workflow — recursively. It
exits `0` rather than failing, so a composite task that happens to include a `ci:*`
task still succeeds inside CI.

`${GITHUB_ACTIONS:-}` rather than `${GITHUB_ACTIONS}` because the script runs under
`set -u`, where an unset variable is an error.

### act configuration

`.actrc` holds the flags, so they apply however act is invoked:

```
-P ubuntu-latest=catthehacker/ubuntu:act-latest
--container-daemon-socket -
--rm
```

**The runner image mapping** is what stops act prompting interactively the first
time. `catthehacker/ubuntu:act-latest` carries the tooling actions expect; the
default `node:*-slim` image does not.

**`--container-daemon-socket -` does not bind the host Docker socket** into workflow
containers. A container with access to that socket can do essentially anything to the
host, including to containers it does not own, and handing that to third-party
actions by default is not a trade worth making for convenience. This is why the
`container` job is skipped under act.

### The container job is skipped locally

It needs the Docker daemon that `.actrc` withholds, so it prints an explanation
instead:

```yaml
- name: Build image
  if: ${{ !env.ACT }}
  run: mise run docker:build
```

act sets `ACT=true` inside workflow containers, which is what that condition reads.

To build the image locally, do it directly, which is faster and does not need the
socket exposed at all:

```bash
mise run docker:build
```

If you specifically want the job to run under act, override the socket for that
invocation:

```bash
act pull_request --container-daemon-socket /var/run/docker.sock --job container
```

Understand what you are allowing before you do.

### Caching

`actions/cache` and `mise-action`'s own cache are both disabled under act:

```yaml
- uses: jdx/mise-action@v4.3.0
  with:
    cache: ${{ !env.ACT }}
```

act's cache server implementation is partial, and the failures are noisy without
being informative. On GitHub, the Go module and build caches and the npm download
cache are all restored, which is most of the difference between a warm and a cold
run.

### On Apple Silicon

act warns that no container architecture was specified. The `catthehacker` images
have arm64 builds, so runs generally work natively, which is much faster than
emulation. If an action ships amd64-only binaries and fails oddly:

```bash
mise run ci:local -- --container-architecture linux/amd64
```

That is deliberately not in `.actrc`, because forcing emulation for everyone to
accommodate a hypothetical action would make every local run several times slower.

## What act does not reproduce

Worth knowing before trusting a local pass completely.

- **Concurrency and cancellation.** Ignored locally.
- **Caching.** Disabled, so timings are not comparable.
- **Secrets** are not present unless you pass `--secret` or `-s`.
- **`GITHUB_TOKEN`** is absent, so anything calling the GitHub API will not work.
- **Runner hardware.** A container on your laptop is not a GitHub runner; a test
  that only fails under a slow runner's timing may pass locally.
- **The exact runner image.** `catthehacker/ubuntu:act-latest` is close to
  `ubuntu-latest` but not identical.

For this workflow none of these matter much, because the jobs are self-contained:
install toolchains, run tests, build. The main use of `act` here is checking that a
workflow edit is syntactically valid and that the steps do what you meant, without
burning a push.

## Adding a job

1. Add the job to `.github/workflows/ci.yml`.
2. Add it to the `needs` list of the `ci` gate job **and** to its result check, or
   the gate will pass while your job fails.
3. Put the actual commands in a mise task rather than inline shell, so the same
   thing is runnable locally.
4. Verify it: `mise run ci:local -- --job <id>`.

If a step needs a tool that is not Go or Node, prefer `mise exec tool@version --`
over a setup action. It keeps the version pinned in one place and works identically
locally.

## Troubleshooting

**`act` prompts to choose an image** — `.actrc` is not being read. Run from the
repository root.

**`Cannot connect to the Docker daemon`** — start Docker. `mise run ci:local` checks
this and says so.

**A job passes locally and fails on GitHub** — usually caching or a missing secret.
Compare the step logs; the mise-action step reports which tool versions it installed.

**`base ref 'main' does not exist locally`** — you have no local `main`. Either fetch
it or pass `--base <ref>`.

**Everything is very slow the first time** — expected. Image pull plus toolchain
install. Subsequent runs reuse both.

**`mise: command not found` inside the container** — the `mise-action` step failed
earlier in the job. Read its output; usually a network problem reaching the mise
release.
