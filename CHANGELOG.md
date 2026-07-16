# Changelog

## Unreleased

### Features

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
