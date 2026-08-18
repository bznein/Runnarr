# Runnarr

Runnarr is a self-hosted, Dockerized activity hub. It imports activities from Garmin Connect and local activity files, presents a private dashboard with activity history, maps, and charts, and builds structured running workouts from training plans or manual prescriptions. Multiple local accounts can use one deployment while keeping activity, health, provider, gear, workout, and planning data private to each account.

The v1 scope covers the existing private activity, health, calendar, gear, tools,
planning, Garmin, manual-import, map, chart, multi-user, and PWA workflows.
Course support provides private per-account storage, a searchable/favorite
library, GPX review/import/export, course inspection, activity route snapshots,
and waypoint planning with optional self-hosted Valhalla routing. Printable
pace bands, basic/expert mode, Garmin write-back, and encrypted support mode
remain post-v1 work. Maintainers can optionally
provision isolated PR previews, persistent staging, and manually approved
production promotion using the [deployment pipeline](docs/deployment-pipeline.md);
the normal self-hosted Compose workflow remains unchanged.

## Quick Start

1. Copy `.env.example` to `.env`.
2. Change `RUNNARR_ADMIN_USERNAME`, `RUNNARR_ADMIN_PASSWORD`, and `RUNNARR_SECRET_KEY`.
3. Start the stack:

```sh
docker compose up --build
```

The app listens on `http://localhost:37617` by default.

The configured admin account is created automatically on first startup. Additional accounts are created from Settings by an administrator. Administrators can temporarily enter a read-only support view for another account; account data remains private and provider credentials are stored per user.

## Mobile web and PWA

The responsive web client is the mobile client. It can be installed as a PWA
from a supported HTTPS deployment or localhost. The service worker caches only
the application shell and static assets; authenticated API responses, activity
media, maps, and provider data remain network-only.

The Google Pixel 8 Pro in Chrome is the primary mobile acceptance profile, but
the layout adapts to smaller phones, tablets, and desktop browsers. See the
[web and PWA smoke-test checklist](docs/mobile-pwa-smoke-test.md) for browser,
installability, responsive-layout, and cache checks.

Runnarr keeps a per-user notification inbox for generated or changed workouts,
Garmin calendar reconciliation, automatic activity matching, and
training-sheet writeback. Each category can be disabled, kept in-app, or also
sent as browser push. Push is opt-in per device; subscriptions are encrypted at
rest, can be renamed, tested, or removed from Settings, and can reach an
installed phone PWA while the app is closed. iPhone and iPad push requires the
site to be added to the Home Screen before permission is requested.

## Courses

The full experience includes a private course library on desktop and under
More on mobile. A course can be created from a GPS activity or from reviewed
GPX tracks, segments, and routes. Imports show invalid segments, duplicates,
discarded GPX data, route geometry, and available elevation before committing.
Course detail supports local metadata, favorites, duplication, permanent
deletion, GPX export, and a one-shot current-location overlay. Location is
requested only after pressing the control and is not stored separately by
Runnarr. When a saved course is available, its starting point seeds the next
new course draft. A saved revision can also be sent once to the connected
Garmin account. Garmin receives a private, separately managed copy: later local
edits create a new Garmin course, and Runnarr never replaces or deletes the
remote copy automatically. Inconclusive provider responses are retained as an
attention state instead of being retried and potentially creating duplicates.

The waypoint planner can keep individual legs direct or route them through an
optional backend-connected Valhalla service. The normal stack leaves this
heavier regional service off. See the [course-routing guide](docs/course-routing.md)
for graph sizing, Compose startup, privacy boundaries, and external-service
configuration.

If that port is already used on your host, change `RUNNARR_PORT` and `RUNNARR_BASE_URL` in `.env`.

For an HTTPS deployment behind Nginx Proxy Manager, see
[docs/internet-deployment.md](docs/internet-deployment.md). Public mode is an
explicit Compose override and does not change the local `localhost` path.

## Local Development

Backend:

```sh
source .env
go run ./cmd/runnarr
```

The example environment binds a directly-run backend to loopback. Docker
overrides the container listen address internally, while its host port remains
loopback-only.

Frontend:

```sh
cd web
npm install
npm run dev
```

For local non-docker full-stack development with Vite hot-reload, use:

```sh
scripts/dev.sh
```

`scripts/dev.sh` will create `.env` from `.env.example` on first run, generate a `RUNNARR_SECRET_KEY` if missing,
and run backend+frontend. `RUNNARR_ADMIN_PASSWORD` is preserved unless missing.
If `RUNNARR_HTTP_ADDR` is unset or `:8080`, it will be replaced with a random high port.
`RUNNARR_FRONTEND_PORT` (default `5173`) sets the preferred Vite port.
Point `DATABASE_URL` in `.env` at a PostgreSQL 16 database with PostGIS 3.5+
before running if you want a non-default database URL. The bundled Compose
database uses `postgis/postgis:16-3.5-alpine`. Existing deployments should read
the [PostGIS database upgrade notes](docs/postgis-upgrade.md) before taking the
course-support migration.
When using `docker compose up -d db` for local DB, keep `RUNNARR_DB_HOST_PORT` in `.env` aligned to the host-mapped postgres port (default `5432`).

See [docs/development.md](docs/development.md) for full non-dockerized setup notes.

Set `DATABASE_URL` to a running Postgres instance before starting the backend outside Docker.

## Garmin Connect Setup

Garmin Connect sync is configured from Settings after login. Enter your Garmin email/password, and enter an MFA code if Garmin asks for one. Runnarr stores Garmin Connect tokens in the Docker `app-data` volume and does not store your Garmin password.

Garmin workout scheduling is separately opt-in. Runnarr creates reusable
templates and schedules only the next seven local calendar days. It never
edits a Garmin workout in place. Unscheduling and cleanup require both the
locally tracked Garmin ID and the exact per-user Runnarr ownership marker;
matching names are never treated as ownership, so workouts created outside
Runnarr are left untouched.

The Garmin integration uses an unofficial Garmin Connect client because Garmin's official Activity API requires approval. If Garmin changes their private endpoints, reconnecting or updating the image dependency may be required.

## Repository

The intended upstream repository is:

```text
https://github.com/bznein/Runnarr
```
