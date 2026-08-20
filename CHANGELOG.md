# Changelog

## Unreleased

- Activity details can now copy a concise Markdown summary with metrics,
  notes, intervals or laps, and climbs for use with AI tools.
- Course maps now show zoom-aware kilometre markers while planning and when
  inspecting a saved course, with wider clean intervals when zoomed out.
- Keep the course planner map at the user's chosen position and zoom while waypoints and route geometry are adjusted.

- Fixed the initial production rollback-compatibility check to include
  staging's generated HTTPS and deployment environment settings.
- Fixed non-production ingress routing so the gateway resolves staging and
  preview aliases to their app containers instead of back to itself.
- Fixed GitHub-hosted deployment SSH to use the checksum-pinned native
  `cloudflared` client instead of a Docker-wrapped proxy command.

### Features

- Added explicit-submit place search to the course planner through an optional
  backend-proxied Nominatim-compatible geocoder, with map centering and direct
  waypoint creation from bounded results.
- Saved courses can now be sent to a connected Garmin account as private Garmin
  Connect courses, with per-revision duplicate protection, durable success or
  attention states, and an offline fake-Garmin testbed flow.
- Course-planner waypoints can now be reordered by mouse, touch, or pen dragging,
  with a labeled insertion preview and arrow controls for keyboard use.
- Added the PostGIS-backed, account-private course foundation with bounded
  geometry APIs, GPX preview/import/export, duplicate protection, and activity
  route snapshots.
- Added the responsive Courses library with search, sport and favorite filters,
  map and elevation-profile inspection, GPX review/import and export, activity
  route snapshots, metadata editing, duplication, permanent deletion, and an
  explicit one-shot current-location overlay.
- Added a waypoint course planner with sport-aware self-hosted Valhalla
  routing, draggable ordered waypoints, per-leg direct overrides and isolated
  fallbacks, optimistic revision protection, an optional regional routing
  Compose profile, and a single host-managed Valhalla graph that can be safely
  attached and route-smoke-tested across isolated pull-request previews. The
  planner previews elevation, ascent, descent, and coverage before saving and
  can add a final routed leg back to the starting point with one action.
- New course plans now begin at the starting point of the most recently updated
  saved course when one is available.
- Course planner waypoints can now be given custom names that persist across
  route edits and course duplication, with compact map labels that reveal the
  full name on hover or keyboard focus.
- Course planner maps can expand to the full viewport while selecting and
  adjusting waypoints, with an explicit exit control and Escape-key support.
- Matched completed activities now link directly to their source training sheet in both full and simple experiences.
- Added an account-selectable `/simple` experience containing only a status-aware run queue and the complete training-sheet matching, preview, writeback, retry, and unmatch workflow.
- Planned-run matching now shows color-coded match scores with date, duration, and workout-structure reasons, and avoids suggesting weak, ambiguous, or incompatible plans.
- Added structured workout authoring from training-sheet prescriptions or manual text, including nested repeats, time/distance/lap-button steps, exact and ranged pace targets, configurable pace tolerance, final-recovery skipping, editable manual copies, parse diagnostics, calendar links, and a dedicated Workouts UI.
- Added opt-in seven-day Garmin workout scheduling with per-user timezone settings, exact provider/date auto-matching after activity import, template cleanup, reconciliation status, and an offline fake-Garmin testbed.
- Garmin workout ownership is fail-closed: Runnarr schedules, unschedules, or deletes a remote object only when both its locally tracked provider ID and per-user Runnarr ownership marker match. Names never establish ownership, and foreign Garmin workouts are left untouched.
- Added a per-user notification inbox for workout generation and changes, Garmin calendar reconciliation, automatic activity matching, and training-sheet writeback, with configurable category delivery, 90-day history, actionable links, an unread-only bell menu, explicit severity icons and bulk-action feedback, and optional encrypted browser push subscriptions per device.
- Added an optional immutable-image deployment pipeline with isolated
  synthetic PR previews, persistent staging, Cloudflare Access protection,
  manually approved staging-to-production promotion, encrypted pre-deployment
  backups, build identity checks, and bounded rollback.

### Fixes

- Runtime images now install current Debian security updates and patched
  Python dependencies, then remove unused package-installation tooling before
  candidate vulnerability scans.
- Training-sheet and manual prescriptions such as `45mins w/surges` now create
  five-minute blocks with a 4:30 steady run and 30-second surge, retaining any
  leftover duration as a final run step.
- Preview and staging deployments now provide every required synthetic-fixture
  date variable when seeding candidate databases, while candidate seed files
  remain compatible with hosts whose deployment helper predates those inputs.
- Rerunning the deployment host installer now preserves the backup recipient,
  staging seed username, and production URL in `deploy.conf`.
- Course-map current-location controls now center the map on the detected
  position instead of only adding an off-screen marker.
- Course-planner metrics now lead with distance, while elevation coverage is
  shown only as an incomplete-data notice instead of occupying a primary card
  when coverage is complete.
- New Valhalla-routed course legs now fetch regional elevation profiles and
  calculate inspectable ascent and descent while preserving routes when the
  elevation service is unavailable.
- GPX and activity course imports now keep source geometry intact while
  limiting curvature-selected editable control points to a manageable set.
- Automatic training-sheet matches now prompt for RPE and feedback, and later reflection saves update only those sheet cells without re-running completed summary and interval writeback; writeback statuses are also shown with readable labels.
- Garmin workout cleanup now treats provider 404 responses as already deleted, clearing stale calendar and template tracking without reporting an ownership conflict.
- Planned and completed activity views now link to their corresponding Runnarr workout.
- Removed the misleading managed-workout link to Garmin Connect because the app does not support exact-workout deep links and the website requires a separate browser login.
- Garmin workout ownership verification now handles numeric workout IDs and nested calendar response IDs from real Garmin responses without false conflicts.
- Manual workout plans no longer create training-sheet writeback records or jobs when matched to an activity.
- Planned-run matching now treats a single all-activity interval step as a continuous run instead of rejecting an otherwise exact continuous plan match.
- Calendar entries for completed runs now retain matched-plan provenance and show the original planned date when it differs, without displaying the completed plan as a duplicate pending activity.
- Activity details now hide the Intervals tab when there are no structured intervals or recorded laps.
- Strength-training activities no longer show distance, elevation, pace, or grade-adjusted pace summary metrics.

## 1.0.0 - 2026-07-30

### Features

- Planned-run matching candidates now use a vertical calendar timeline with the activity date highlighted.
- Calendar days with health or activity data now open a day view with daily health details and the day's completed or planned activities.
- Added account-scoped visual themes with named Runnarr, Ocean, Sunset, and Midnight palettes plus a system option.
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

### Fixes

- Exiting administrator support view from another user’s activity now returns to the dashboard instead of reloading the invalid activity URL.
- Login rate limiting now counts failed password attempts without penalizing successful sign-ins.
- Planned-run matching remains retryable when the initial candidate load fails.
- Planned-run candidate loading no longer carries candidates between activities, and retries now show a busy state.
- Planned-match previews are cleared and ignored when navigating between activities.
- Planned-match mutations no longer update a later activity view after navigation.
- Planned-match callbacks are ignored after editing or closing the matching dialog.
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
- Activity detail pages now provide previous/next arrows for browsing activities in the current list order.
- Activity detail navigation now waits for refreshed neighbor data before allowing another move.
- Activity list name, type, and date filters now share one filter dialog with a single selectable activity-type list.
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
- Climb details now show recorded pace and overlapping lap GAP alongside the elevation profile when those values are available.
- Added more metric card graphics/icons on dashboard and health pages to improve scanability of steps/energy/sleep/HRV/more core fields.
- Climb detection settings now live in Settings with persistent preset controls and a temporary per-activity sensitivity override; activity climb detections re-compute after saved changes.
- Added a new Calendar view with a month-by-month activity grid, month navigation, and clickable activity links.

### Fixes

- Calendar day and month queries now honor the browser timezone at activity date boundaries, including planned entries around midnight, while keeping date-only planned entries on their planned day.
- Fixed same-year training-sheet replacements leaving stale pending suggestions and duplicate imported planned activities, including after unmatching a plan from a replaced workbook.
- Climb detection thresholds and difficulty now account for cycling activities separately from running-style activities.
- Activity detail planned-run matching now uses a compact Match/Unmatch action, and assigned gear appears as a small chip beside the activity title.
- Health range changes no longer load the preserved raw Garmin payload for every day, making first-time 30D and 90D views responsive.
- Activity type filters now use a compact include/exclude control with checkbox bulk actions.
- Activity detail charts now recover when the available metrics change.
- Health summary cards now show the actual metric date and remain pinned to today's data while chart ranges change.
- Activity detail charts now use robust display bounds for isolated outliers while preserving raw samples.
- Planned-run suggestions and matching now only apply to running activities.
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
