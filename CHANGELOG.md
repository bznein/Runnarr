# Changelog

## Unreleased

### Features

- Added read-only Garmin gear sync with active/retired gear views, gear detail pages, Garmin mileage, and assigned activity links.
- Activity list columns can now be toggled, and long activity/gear names are clipped more aggressively to keep the list scannable.
- Activity detail pages now support local-only notes that do not modify provider activities.
- Activity detail pages can export a GPX track, with an option to include sensor extensions.
- API request logging and Garmin sync lifecycle logs were expanded for easier troubleshooting.
- Garmin health sync now imports daily health metrics, including steps, calories, resting heart rate, sleep, stress, body battery, HRV, and body composition where Garmin provides them.
- Added a Health page with date-range controls, Garmin health sync, summary cards, trend charts, selectable daily rows, and day-level details.
- Body battery daily views now emphasize gained, drained, and highest values.
- Health charts now switch from bars to lines for date ranges longer than 30 days.
- Garmin body-composition weight is now normalized from grams to kilograms and shown as measurement-only points.
- Garmin-synced activities now preserve grade-adjusted pace when provided and show GAP on activity details and laps.
- Activity lap tables now show per-lap pace derived from lap distance and elapsed time.
- Imported activities now preserve provider/file calories when available, show them on activity detail and list views, and support sorting by calories.

### Fixes

- Garmin gear last-used dates now come from linked activities instead of Garmin gear setup metadata.
- Health dashboard date edits no longer reload data until the edited range is applied.

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
