# Changelog

## Unreleased

### Features

- Health sync controls and activity/job status now live in Settings with the other sync controls.
- Added Makefile targets for standard backend, frontend, and Playwright checks.
- Health now shows Garmin sleep score in the summary cards, trend chart, and daily metrics table when available.
- Activity photos can now be pinned or moved to a specific map location when EXIF GPS data is unavailable.
- Added a responsive mobile web shell with mobile navigation, mobile activity and health cards, calendar agenda rendering, safe-area handling, and an installable PWA shell that keeps authenticated data network-only.
- Added a bounded activity-series API so chart and map clients use server-limited sample payloads while full samples remain available for server-side exports and analysis.
- Added a shared web/PWA smoke-test checklist covering Pixel-sized layouts, installability, service-worker updates, and private-data cache boundaries.

- Added explicit local and internet-facing deployment modes: local Docker and
  Vite startup stays loopback-only and password-based, while public mode uses
  HTTPS-only Google OIDC with an email allowlist, host-only secure sessions,
  hardened proxy networking, and deployment guidance.
- Added security hardening across sessions, CSRF/origin checks, response
  headers, request limits, image/resource bounds, static-file containment,
  sync-job concurrency, and non-root container execution.
- Sync jobs can now be canceled cooperatively from progress views, diagnostics, training-plan import, and training-sheet write-back controls, while retaining completed partial work.
- Added local multi-user accounts with administrator-managed username/password access, disabled accounts, password resets, per-user preferences, private datasets, per-user provider connections and scheduled syncs, and read-only administrator support views.
- Matched training-sheet activities now write summary metrics, structured workout interval tables, and separate athlete feedback/RPE back to Google Sheets with conflict-safe retries and safe warnings for ambiguous interval mappings.
- Training-sheet matching now accepts optional feedback, partially maps unrepresented structured intervals with warnings, and allows proposed preview values to be edited before write-back.
- Training-sheet matching now records the default RPE of 5 when the user leaves the RPE slider unchanged.
- Training-sheet feedback write-back now refreshes the feedback cell with the latest saved reflection, queues updates that arrive during another sheet job, and repairs HR cells that were interpreted as time values by the workbook format.
- Planned activity matching now offers a read-only training-sheet change preview with explicit Apply, conflict visibility, and stale-sheet revalidation before writeback.
- Training-sheet match previews now render a focused, sheet-like live grid with proposed values in place, formatting when available, and selectable current/proposed cell details.
- Planned activity matching now offers nearby pending plans, date-based suggestions, and feedback controls based on each plan's requested sheet section.
- Activity and lap pace now prefer provider timer/average-speed data, exclude recorded pauses, and use moving-time fallbacks for write-back and display.
- Garmin structured workouts now preserve workout steps, interval categories, targets, and grouped lap metrics; activity details provide a filterable, expandable Intervals view with a flat-lap fallback.
- Garmin activity sync now defaults to today, with an explicit All data option for full-history syncs.
- Training-sheet feedback sections now associate correctly with single-day workout notes during sync.
- Training-sheet sync now refreshes metadata for existing past planned activities without importing new historical activities.
- Local XLSX training-sheet reference files are ignored by Git.

- Added a new `/tools` page and backend `/api/tools/pace` endpoint for pace calculations. Users can enter any two of distance, time, and pace to compute the missing value while calculation remains server-side.

- Added read-only Garmin gear sync with active/retired gear views, gear detail pages, Garmin mileage, and assigned activity links.
- Activity list columns can now be toggled, and long activity/gear names are clipped more aggressively to keep the list scannable.
- Activity detail pages now support local-only notes that do not modify provider activities.
- Activity detail pages can export a GPX track, with an option to include sensor extensions.
- Garmin health sync now imports daily health metrics, including steps, calories, resting heart rate, sleep, stress, body battery, HRV, and body composition where Garmin provides them.
- Health defaults now open on a 7-day range by default, matching the 7D preset.
- Added a Health page with date-range controls, Garmin health sync, summary cards, trend charts, selectable daily rows, and day-level details.
- Body battery daily views now emphasize gained, drained, and highest values.
- Health charts now switch from bars to lines for date ranges longer than 30 days.
- Selecting a health metrics row now auto-scrolls to the opened day-detail section.
- Garmin body-composition weight is now normalized from grams to kilograms and shown as measurement-only points.
- Garmin-synced activities now preserve grade-adjusted pace when provided and show GAP on activity details and laps.
- Activity lap tables now show per-lap pace derived from lap distance and elapsed time.
- Imported activities now preserve provider/file calories when available, show them on activity detail and list views, and support sorting by calories.
- Added a self-bootstrap local development flow: `scripts/dev.sh` now creates `.env` from `.env.example` and seeds missing credentials/defaults for first-run non-Docker setup.
- Improved non-Docker dev bootstrapping by auto-selecting a high random backend port (instead of `:8080`) and auto-starting local PostgreSQL via `docker compose up -d db` when `DATABASE_URL` points to localhost.
- Fixed local dev bootstrap false-failure with Dockerized postgres by exposing postgres in compose on `RUNNARR_DB_HOST_PORT` (default `5432`) and adding clearer error guidance in `scripts/dev.sh` when local DB reachability is blocked by missing host port mapping.
- `scripts/dev.sh` now proactively clears a stale local Runnarr Vite process on `5173` (unless `RUNNARR_KEEP_LEGACY_FRONTEND=1`) to avoid accidentally opening old frontend instances that send `/api` calls to the wrong port.
- `scripts/dev.sh` no longer overwrites a user-provided `RUNNARR_ADMIN_PASSWORD` (including `change-me`); it only auto-generates one when missing, so the login value you set stays valid.
- Gear list and gear detail pages now support sorting by last used, first used, distance, percent-to-limit, and activity count.
- Activity route coloring now supports switching between pace and GAP (when lap GAP is available) for segment coloring and legend labels.
- Added more metric card graphics/icons on dashboard and health pages to improve scanability of steps/energy/sleep/HRV/more core fields.
- Climb detection settings now live in Settings with persistent preset controls and a temporary per-activity sensitivity override; activity climb detections re-compute after saved changes.
- Added a new Calendar view with a month-by-month activity grid, month navigation, and clickable activity links.

### Fixes

- RPE sliders now use effort-based colors from easy through maximum effort.
- Hide per-activity climb sensitivity controls for activity types such as swimming, kayaking, and treadmill runs where climbs are not meaningful.
- Document the Cloudflare JavaScript Detections setting required to preserve Runnarr's strict CSP.
- Round VDOT distance presets to the precision accepted by the calculator input.
- Structured Garmin intervals now retain and display recorded laps when Garmin omits explicit interval-to-lap indexes.
- Hide the interval step-type selector when all intervals use the same step type.
- Interval and lap cumulative times now follow Garmin’s recorded durations instead of wall-clock timestamps that include pauses; single-type interval views open expanded.
- Pace formatting now carries rounded seconds into the next minute instead of displaying invalid values such as `4:60 /km`.
- Paused Garmin timer intervals now remain visible in route geometry without contributing their walking speed to pace charts or pace-colored routes.
- Health chart axes now reserve enough space for grouped values such as steps
  and show units for sleep, resting heart rate, and HRV.
- Fixed training-sheet writeback status lookups failing when PostgreSQL UUID columns were compared with text parameters.
- Removed the inline theme bootstrap script so strict Content Security Policy no longer reports script violations on SPA routes such as Calendar.
- Display preferences, activity-list columns, and gear sorting now persist per user instead of being shared through browser-local storage.
- Garmin gear last-used dates now come from linked activities instead of Garmin gear setup metadata.
- Health dashboard date edits no longer reload data until the edited range is applied.
- `scripts/dev.sh` now selects and reports the actual Vite port it starts on (with optional `RUNNARR_FRONTEND_PORT`), which prevents logging stale localhost:5173 URLs when ports are already taken and avoids loading the wrong frontend instance that causes `/api/...` 404s.
- Activity type names from providers are now normalized for UI consistency (for example, Cycling, Treadmill Run, and Swimming variants render with readable labels).
- Route GAP/PACE selector now uses a clean sliding control without an extra divider edge under Pace.
- Gear distance usage bars now use green, yellow, and red thresholds at 70% and 95%.

## 0.3.0 - 2026-07-16

### Features

- Activity maps now show every detected climb with start markers, and clicking a climb on the map or in the list selects it.
- Activity browsing now loads additional pages on demand instead of stopping at the first 100 activities.
- Settings now consolidates Garmin sync, display preferences, manual file import, and collapsed diagnostics.
- Added a persistent light/dark/system theme preference.

### Fixes

- Dashboard chart tooltips now inherit the active theme instead of using the default light tooltip in dark mode.
- Climb profile charts now show height above the climb start instead of dipping below zero for relative elevation data.

## 0.2.0 - 2026-07-16

### Features

- Photo media with EXIF GPS coordinates now appears as thumbnail markers on activity maps, and selecting a marker opens the matching photo preview.
- Activity photo uploads with an authenticated gallery, thumbnails, EXIF metadata extraction, preview, and deletion.
- Local activity renaming from the activity detail page. Renames are stored only in Runnarr and survive future provider syncs.
- Original Garmin activity links on activity detail pages when a provider URL is available.
- Compact activity-detail action menu for rename, open-original, and delete actions.

## 0.1.0 - 2026-07-16

### Features

- Initial self-hosted Runnarr application with Docker Compose, automatic Postgres migrations, and a combined API/frontend service.
- Local admin authentication with HTTP-only sessions and CSRF protection for mutating API calls.
- GPX, TCX, and FIT file imports with parser-based normalization, file-hash deduplication, GPS samples, heart-rate summaries, and lap support where available.
- Garmin Connect connection and sync from Settings, including MFA support, token-file reuse, historical backfill, scheduled sync, sync progress, and duplicate-safe provider imports.
- Dashboard summaries for activity count, distance, moving time, elevation, recent activities, and weekly distance.
- Activity browsing with search, date filters, activity-type include/exclude filters, sorting, and activity deletion.
- Activity detail views with route maps, mouse-wheel map zoom, start/end markers, combined elevation/pace/heart-rate/power/cadence graphs, synchronized chart-to-map hover, and lap tables.
- Climb detection with climb summaries, difficulty labels, profile charts, and route highlighting.
- Sync exclusion tracking so deleted provider-synced activities are not re-imported on future syncs.
