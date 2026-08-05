# Development: Run Without Docker

Runnarr can be developed and tested without running the Docker app image.

## Prerequisites

- Go 1.22+
- Node.js 22+
- PostgreSQL 16 with PostGIS 3.5+ (running locally)

You still need a PostGIS-enabled PostgreSQL database for the app, but you can run the server and UI directly:

1. Configure environment variables in `.env` (copy from `.env.example` first).
   `scripts/dev.sh` now performs this bootstrap automatically when `.env` does not yet exist.
2. Set `RUNNARR_STATIC_DIR=web/dist` (default) for the backend config.
3. Set `RUNNARR_HTTP_ADDR` for the backend (if unset, `scripts/dev.sh` now auto-chooses a random high port and binds it to loopback).

## Make targets

The repository Makefile provides the standard checks in one place:

```bash
make check       # Go format, vet, backend tests, frontend tests, and build
make test-race   # Backend race-enabled tests
make e2e         # Isolated Docker Compose Playwright suite
make visual-review # Selected before/after Playwright recordings
make deployment-check # Deployment script and rendered Compose invariants
```

`make` runs `make check`. The E2E suite is separate because it starts and
removes an isolated Docker Compose project. Set `GOCACHE` when the default Go
cache location is not writable; it defaults to `/tmp/runnarr-go-cache`.

## Start backend + frontend (with Vite hot-reload)

```bash
scripts/dev.sh
```

On first run, the script creates `.env` from `.env.example`, fills missing defaults,
generates a `RUNNARR_SECRET_KEY` when missing, and leaves `RUNNARR_ADMIN_PASSWORD` untouched unless missing.

Then visit:

- Backend API: uses `RUNNARR_HTTP_ADDR` (or the auto-selected random high port when unset).
- Frontend (Vite): whatever is available, e.g. `http://localhost:5173`
  (set `RUNNARR_FRONTEND_PORT` in your environment to pin it).
The backend and Vite server bind to loopback by default. If you still see `/api` calls sent to `5173`, stop the old frontend process on that port and rerun:

```bash
pkill -f "vite --host 127.0.0.1 --port 5173"
```

Set `RUNNARR_KEEP_LEGACY_FRONTEND=1` to keep manually managed legacy frontend instances running.

The frontend uses Vite proxy rules for `/api` and `/healthz`, routed from `scripts/dev.sh` via `VITE_API_TARGET`.

## Browser end-to-end tests

The Playwright suite exercises the full local web app in desktop Chromium and
Google Pixel 8 Pro Chrome emulation. It starts an isolated Docker Compose
project with a fresh PostgreSQL database, seeds deterministic health and gear
records, runs the tests, and removes only that test project when finished.

Install the web dependencies and browser once:

```bash
cd web
npm ci
npx playwright install chromium
```

Then run the suite from the `web` directory:

```bash
npm run e2e
```

The runner chooses free host ports automatically. Override
`RUNNARR_E2E_PORT`, `RUNNARR_E2E_DB_PORT`, `RUNNARR_E2E_USERNAME`, or
`RUNNARR_E2E_PASSWORD` when needed. Pass normal Playwright arguments after
the npm command, for example `npm run e2e -- --project=mobile-chromium`.
Videos are written for every executed test under `web/test-results/`. Set
`PLAYWRIGHT_SLOW_MO` to add a delay between browser actions when reviewing
them, for example `PLAYWRIGHT_SLOW_MO=250 npm run e2e`.

### Before/after visual review

Material user-facing pull requests can select up to two recording profiles
from `.github/visual-review-profiles.json`. Each profile combines one tagged
E2E journey with either the desktop or Pixel 8 Pro viewport. The PR labels use
the form `visual:<viewport>:<scenario>`, for example:

```text
visual:desktop:activity-inspection
visual:mobile:mobile-navigation
```

A collaborator with write access can add a profile label to record the
selected journey against both the PR's `main` merge base and its head. An
untrusted push, reopen, or ready-for-review event removes all visual labels;
the collaborator must inspect the new head and reapply the affected profiles.
This prevents outside contributors from authorizing paid recording or media
work and makes the approval specific to one commit.

The two revisions use separate Docker Compose projects, the same synthetic
seed and fixture clock, and no provider credentials. Recordings are at most 60
seconds and 25 MiB each. A default-branch-controlled publisher accepts at most
two profiles (four videos), converts the WebM recordings to H.264 MP4 with
small poster images, and embeds direct seven-day links in one bot comment.
Separately named before/after GitHub artifacts retain the original video,
Playwright failure diagnostics, and Compose logs as a ZIP fallback. R2 or
transcoding failure leaves those ZIPs available, adds a warning to the comment,
and fails the visual publishing workflow visibly.

The recorder checks the triggering actor before checking out or executing PR
code. The R2 credentials are available only to the later trusted publisher,
which independently rechecks the original actor, current PR head, latest run,
default-branch profile catalog, artifact names and sizes, and sanitized file
types. The guard and publisher are `pull_request_target`/`workflow_run`
workflows, so a pull request that first introduces or changes them cannot use
the new trusted path until those definitions have merged to the default
branch.

Generate the same comparison locally from a clean, committed branch with:

```bash
VISUAL_PROFILES="visual:desktop:activity-inspection" make visual-review
```

Select two profiles with a comma-separated value. Set `VISUAL_BASE_REF` when
the comparison base is not `origin/main`:

```bash
VISUAL_BASE_REF=main \
VISUAL_PROFILES="visual:desktop:auth,visual:mobile:auth" \
make visual-review
```

The command creates a temporary detached worktree for the merge base, runs
only isolated E2E stacks, and writes both revisions under
`web/test-results/visual-review/`. A failed before journey is retained as
comparison evidence; a failed after journey makes the command fail. The
generated `.webm` files can be attached manually to a PR when needed.

For free-form product exploration against disposable data, start the permanent
testbed workflow without Playwright driving the browser:

```bash
make testbed
```

The command prints the selected URL and local login credentials, then keeps the
stack running until Ctrl-C. It contains a synthetic multi-sport activity
history with samples, laps, structured workout prescriptions, health history,
gear, and training plans. Garmin workout operations use a file-backed offline
bridge with one deliberately foreign workout and calendar entry; the bridge
rejects any attempt to mutate those foreign fixtures. Activity/health provider
downloads and Google integration remain disabled. Stopping the command removes
its isolated containers, network, and volumes.

The testbed also supports notification UI and browser-push testing. Use the
per-device Test action for a deterministic push without changing fixture data.
Startup intentionally does not create historical notifications for seeded
workouts. The direct device test sends push only; inbox timeline events appear
after a real state transition performed during that testbed session.

The browser tests intentionally use local password authentication and local
fixtures. Garmin Connect, Google OAuth, MFA, and real provider syncs are not
part of the deterministic CI suite.

### What the script does

- Validates required runtime variables
- Installs frontend dependencies if missing
- Starts `go run ./cmd/runnarr` and `npm run dev` together
- Keeps both processes on a single command

Process logs are written to `tmp/runnarr-backend.log` and `tmp/runnarr-frontend.log`.

## Quick DB alternative with Docker (optional)

If you want the quickest local database start, run only the bundled
PostGIS-enabled PostgreSQL service in Docker:

```bash
docker compose up -d db
```

`scripts/dev.sh` now auto-starts `docker compose up -d db` when `DATABASE_URL` points at localhost and PostgreSQL is not reachable.
Set `RUNNARR_SKIP_DB_START=1` to prevent this behavior if you want to keep startup fully manual.
`RUNNARR_DB_HOST_PORT` controls which host port Docker publishes for postgres (default `5432`) and is used by both `docker compose up -d db` and `scripts/dev.sh`.
