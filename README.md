# Runnarr

Runnarr is a self-hosted, Dockerized activity hub. It imports activities from Garmin Connect and local activity files, presents a private dashboard with activity history, maps, and charts, and builds structured running workouts from training plans or manual prescriptions. Multiple local accounts can use one deployment while keeping activity, health, provider, gear, workout, and planning data private to each account.

The v1 scope covers the existing private activity, health, calendar, gear, tools,
planning, Garmin, manual-import, map, chart, multi-user, and PWA workflows.
Course support provides private per-account storage, a searchable/favorite
library, GPX review/import/export, course inspection, activity route snapshots,
and waypoint planning with optional self-hosted Valhalla routing. Race support
adds private planning, confirmed results, performance analysis, and editable
reports for fixed-distance running events. Printable
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

## Races and performance

Races in the full experience (under More on mobile) keeps upcoming events and
race history independently of recorded activities. Create an event manually,
use **Mark as race** on a run, or review suggestions from imported activities.
The Review tab can scan existing history; suggestions always require a
confirmation and dismissals are remembered. Road, track, trail, cross-country,
parkrun, virtual events, time trials, and fixed-distance ultras are supported.
Unknown dates and distances can stay blank.

Each race can hold A/B/C importance, time and non-time goals, registration and
travel notes, reusable preparation checklists, a training-plan link, and an
independent snapshot of a saved running course. Calendar entries use the event
local date and display its timezone when a start time is known. A race links to
one complete recording; linking never applies an activity/training-plan match
or queues a Google Sheets writeback.

Official distance, chip/gun/manual finish times, places, and manually entered
cumulative checkpoints stay separate from watch measurements and laps. Confirm
finished results to include them in PBs and calendar-year bests by exact
distance and discipline. Results can be excluded individually. Recurring-event
groups let you compare editions while retaining each edition's distance and
course. The 12-week build-up shows recorded running before race day.

Halfway prefers an exact official midpoint checkpoint. Otherwise confirm that
the complete activity contains only the race. The estimate uses the midpoint
of recorded distance, full original samples, and elapsed time including stops.
It rejects distance resets, invalid timestamps, incomplete boundaries, and
interpolation gaps exceeding 60 seconds or 250 metres. Compatible confirmed
chip/manual times can scale both elapsed halves; gun time never supplies that
scaling. Every estimate identifies its source and timing basis.

Performance includes result, VDOT, and age-grade trends. VDOT uses confirmed
road/track results from 1500 m through marathon; current equivalents default to
the best eligible VDOT in the last 90 days, with an explicit reference override.
No older result is silently substituted. Age grading uses bundled CC0 USATF
MLDR road 2025 tables, exact listed distances and ages 5–99, with optional birth
date/table preferences or per-race overrides. The source revision and license
are retained in `internal/app/data/race-age-grades/`.

Garmin prediction sync and Open-Meteo race forecasts are separate opt-ins in
race settings. Prediction sync initially requests 365 days, then refreshes the
last 14 days daily or manually, retaining nullable results and raw responses.
Comparisons exclude same-day Garmin predictions without a reliable timestamp;
historical data fetched after race day is labelled. Local VDOT predictions are
saved before the race and never reconstructed retrospectively from its result.
Forecasts send rounded event coordinates, cover the next 16 days, refresh at
most every six hours through the shared weather limiter, and preserve the last
pre-start forecast. A date-only event uses local midnight as its cutoff.
Forecast errors preserve prior data. Observed activity weather stays separate.

Reports combine chosen facts, goals, results, checkpoints, halfway analysis,
optional watch laps/build-up/forecast, and your own account. Save the draft,
copy/download Markdown, or use the print view to save PDF. Generated exports
exclude birth date and private logistics; review your own narrative before
sharing. No AI service or automatic publishing is involved.

Existing activities need no Garmin resync: use the local historical scan.
Only the optional Garmin prediction history requires its own sync. No
training-sheet sync or writeback is needed for race features. Timed events,
relays/stage races, trimmed or multiple recordings, external result imports,
public sharing, and automated race notifications remain outside this scope.

## Personal heatmap

Heatmap in the full experience (under More on mobile) shows your GPS activity
history, initially all-time running. Choose sports and local-calendar dates,
pan or zoom, fit all selected routes, or expand the map to fullscreen. Filters,
appearance, and position stay in the URL. Appearance follows the app theme by
default and can be set to Light or Dark independently.

Heat measures distinct activities at each location: more samples, slower pace,
or repeated laps within one activity do not inflate it. The logarithmic legend
uses the same scale across filters. Counts and distance describe the entire
filtered selection, with separate counts for routes being prepared and
activities without usable GPS. GPS gaps and dateline crossings are split;
summary polylines are used only when sample GPS is absent.

Download PNG previews the current crop at up to twice its displayed resolution
(maximum 4096 pixels per edge). Map includes the muted basemap and attribution;
Artwork puts the routes on a plain background. Map export reuses visible
basemap tiles and requires provider CORS support. If a custom provider prevents
export, use Artwork. Both include the selected sports and dates.

The route index is derived from locally stored activities. Historical backfill
runs automatically and resumes after interruption; new imports, reimports,
and deletions update the map automatically. No Garmin sync or training-sheet
sync/writeback is needed. Heatmap tiles are authenticated, account-private,
and never cached by the service worker; there is no public sharing endpoint.

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

The planner can also search for a town, landmark, or address through an
optional backend-connected Nominatim-compatible geocoder. Search is disabled
until an operator chooses a public or self-hosted endpoint. See the
[course place-search guide](docs/course-place-search.md) for setup, provider
policy, and privacy boundaries.

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

Garmin remains the primary source for activity weather. An account can opt in under Settings to use Open-Meteo when Garmin returns no usable conditions. The fallback sends rounded activity-midpoint coordinates and the activity date to Open-Meteo. For recent activities it requests 15-minute UKMO, ICON, and ECMWF values together, then stores the complete record from the model with the median temperature; older activities use the nearest hourly archive value. The selection method, chosen model, and attribution appear in activity details and Copy for AI. The built-in limiter conservatively counts a three-model request as three calls and stays below the [free non-commercial API limits](https://open-meteo.com/en/terms); commercial deployments need an appropriate Open-Meteo plan. Open-Meteo data is provided under [CC BY 4.0](https://open-meteo.com/en/license).

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
