# Runnarr Product Requirements Document

## 1. Product Summary

Runnarr is a self-hosted activity hub for endurance data. It imports activities from provider APIs and local files, normalizes them into a provider-independent schema, and presents a private, useful view of training history with maps, charts, and race context.

The first release is intentionally not a full training science platform. It should make the data easy to own, browse, verify, and extend. Deep analysis similar to Runalyze or intervals.icu is future scope.

## 2. Goals

- Provide a Dockerized service that can run on a home server, NAS, VPS, or local machine.
- Connect to Strava through OAuth and import the authenticated athlete's activities.
- Provide a subscription-free Strava import path using the user's Strava data export or downloaded activity files.
- Import local activity files with an extensible parser architecture.
- Store activities in a canonical schema that is independent from Strava or any one file format.
- Render a useful activity dashboard, activity list, detail view, route map, and basic charts.
- Establish a durable architecture for future providers, import formats, races, analytics, and multi-user support.

## 3. Non-Goals for V1

- Public social feed, public sharing, comments, kudos, leaderboards, or Strava-like community features.
- Writing activities back to Strava or other providers.
- Deep physiological analytics, training load modeling, interval detection, structured workouts, or coach workflows.
- Full multi-user account management.
- Native mobile apps.
- Self-hosted map tile server.

## 4. Target User

The v1 user is a technically comfortable runner, cyclist, triathlete, or data-minded endurance athlete who wants private ownership and a consolidated view of their activity data.

The default deployment is single-user. The architecture should avoid blocking future multi-user support, but v1 should not add account-management complexity beyond one admin session.

## 5. V1 Requirements

### Deployment

- The app must run via Docker Compose with an application container and a Postgres container.
- Configuration must be environment driven.
- The app must expose one HTTP port and serve both API and frontend.
- Database migrations must run automatically at startup.

### Authentication and Privacy

- The UI must require a local admin login.
- Sessions must use HTTP-only cookies.
- Mutating API calls must require CSRF protection.
- Provider tokens must be encrypted at rest.
- Strava data must only be displayed to the local authenticated admin.

### Activity Imports

- V1 must import GPX, TCX, and FIT files.
- V1 must import Strava account bulk export archives so users can migrate their Strava history without a paid Strava API/developer subscription.
- Strava archive import must locate supported activity files inside the archive, import each supported activity, and report unsupported files without failing the entire archive.
- Manual file/archive import must remain available, but should be a secondary workflow rather than a primary navigation item or prominent dashboard action.
- The importer must deduplicate repeated uploads by file hash and provider source identifiers.
- The import pipeline must normalize:
  - activity name
  - sport type
  - start time
  - elapsed and moving time where available
  - distance
  - elevation gain
  - calories or energy expenditure where available
  - heart rate summary where available
  - GPS samples
  - laps where available
- The parser architecture must allow new formats without touching API handlers.

### Strava Provider

- OAuth/API sync is an optional convenience path, not the only way to import Strava data.
- The Strava integration must support OAuth connect and callback.
- Strava setup must be driven by `STRAVA_CLIENT_ID`, `STRAVA_CLIENT_SECRET`, and `RUNNARR_BASE_URL`; for the default local port, the OAuth callback URL is `http://localhost:37617/api/providers/strava/callback`.
- It must store accepted scopes and provider account metadata.
- It must refresh short-lived access tokens using the latest refresh token.
- It must support manual sync.
- Manual sync must backfill accessible Strava activities from the athlete activity feed, not only activities created since the last sync.
- Re-running sync must update existing Strava activities by provider activity ID and add newly discovered activities without creating duplicates.
- The provider settings UI should make clear that importing requires connecting Strava first, then triggering sync.
- It must track rate-limit response headers where available.
- Webhook endpoints may exist in v1, but polling/manual sync is acceptable for self-hosted local deployments without public callback URLs.

### Provider Sync Status

- Provider settings must show sync status for each provider, including whether it is connected, idle, running, completed, or failed.
- Sync status must include the latest sync time, imported/updated activity counts where available, and any failure message.
- The main provider UI should emphasize the current or latest sync status rather than a full sync-job table.
- Detailed sync history may remain available for debugging, but it should be collapsed by default, behind a secondary control, or moved to a less prominent diagnostics view.
- Scheduled/background syncs must be represented in sync history when diagnostics are opened, but should not dominate the normal provider workflow.

### Settings and Navigation

- The app must have a top-level Settings page for configuration and maintenance workflows.
- Settings should contain provider connections, sync controls/status, theme preferences, unit/display preferences, data import tools, and diagnostics as those features exist.
- Manual import should move into Settings or another secondary location, with clear labeling for file/archive upload but reduced visual prominence compared with dashboard, activity browsing, and provider sync status.
- Primary navigation should prioritize Dashboard, Activities, and Settings; import/upload should not need its own first-level navigation item once Settings exists.
- Settings sections should be structured so advanced or debugging workflows, such as detailed sync history and raw import history, are available without dominating the default view.

### Activity Views

- Dashboard must show total activities, recent activity, total distance, total time, and recent weekly distance.
- Dashboard charts must allow changing the time scale, such as weekly, monthly, and yearly views, without leaving the dashboard.
- Dashboard metrics and charts must support the same core activity filters as the activity list so excluded activity types do not affect totals.
- Dashboard and activity-list time-scale controls may be separate in the first implementation, but the target UX is shared filter state so time scale and other core filters stay synchronized across both views.
- Activity list must support scanning, filtering, and sorting by date, activity type, source, distance, moving time, elapsed time, elevation, and name.
- Activity filtering must support searching by activity name and include/exclude rules for activity types, including common exclusions such as hikes, walks, commutes, strength/weight training, and indoor/trainer activities.
- Activity filtering should support geo-based filters for GPS activities, such as start/end location, activities within a map bounds, activities near a selected point, and activities intersecting a selected area.
- Activity type filters should be hidden when the current dataset contains zero or one distinct activity type.
- Activity sorting must support ascending and descending order for sortable columns and preserve the selected filters.
- Activity detail must show summary metrics, a route map when GPS samples exist, and charts for elevation, pace/speed, and heart rate where data exists.
- Activity detail should support overlaying compatible chart series in a single combined graph, such as elevation, pace/speed, heart rate, power, and cadence, with clear axes, legends, and per-series toggles.
- Activity detail, list rows, and dashboard summaries should use sport-appropriate metrics, units, and labels rather than one generic endurance format for every activity.
- Pace should be formatted by activity type, for example min/km for running, speed for cycling where appropriate, and min/100 m for swimming.
- Each supported activity type should be reviewed for which metrics are meaningful to show, hide, or rename, including distance, pace/speed, elevation, cadence, power, laps, intervals, heart rate, and route maps.
- Activity detail should hide elevation charts for activity types where elevation is not meaningful, such as swimming, strength training, indoor workouts, and similar non-route activities.
- Activity laps imported from providers should preserve provider interval metadata where available, including Intervals.icu `type` and `label` fields from `icu_intervals`.
- Activity detail should allow filtering laps/intervals by provider category, such as warm-up, active interval, recovery, cool-down, and other provider-defined labels when available.
- Route maps must support mouse-wheel zooming for detailed activity inspection.
- Route maps should show start and end markers, and should eventually support well-designed direction indicators along the route without cluttering the map.
- Activity detail should allow attaching media such as photos to an activity.
- Attached media should preserve useful metadata, including capture time and EXIF GPS coordinates when present.
- Activity detail should show attached media in a gallery; media with EXIF location should also appear on the activity map as small thumbnail markers at the recorded location.
- Media thumbnail markers should be unobtrusive and should open or highlight the corresponding media item when selected.
- The admin must be able to delete activities from Runnarr, including their samples and laps, without deleting provider connections or source files outside the app.
- Settings must make the data pipeline visible enough to debug failed imports or syncs without making manual import the dominant workflow.
- Provider imports should populate calories/energy expenditure on activities when the provider exposes it, including Intervals.icu where available.

## 6. UX Principles

- Runnarr is an operational tool, not a marketing site.
- First screen after login should be the usable dashboard.
- UI should be dense but readable, with restrained styling and clear navigation.
- UI must support light and dark themes, defaulting automatically to the user's system color-scheme preference.
- The admin should be able to override the automatic theme preference and persist that choice locally.
- Maps and charts should favor inspection and accuracy over decorative presentation.
- Empty states should explain what action unlocks the page without over-explaining the product.

## 7. Canonical Data Model

The storage model should separate canonical activity fields from provider/raw details:

- `activities`: one row per normalized activity, including calories/energy expenditure when provided by the source.
- `activity_samples`: time-series samples such as position, elevation, heart rate, cadence, power, distance, and speed.
- `activity_laps`: lap or split summaries from providers/files.
- `activity_media`: media attached to activities, including file metadata, thumbnail paths, capture time, and optional EXIF-derived latitude/longitude.
- `import_files`: uploaded file metadata and parser status.
- `provider_connections`: external provider accounts and encrypted tokens.
- `sync_jobs`: provider sync/backfill job state.
- `auth_sessions`: local admin sessions and CSRF token state.

V1 is single-user, but future multi-user support should be possible by adding ownership columns and access control checks.

## 8. Future Roadmap

### Providers

- Garmin Connect or Garmin export ingestion.
- Wahoo, COROS, Polar, Suunto, Zwift, Komoot, TrainingPeaks, and direct device sync where practical.
- Provider-specific metadata preservation without polluting canonical activity fields.

### Import Formats

- Bulk ZIP archives.
- Garmin account exports.
- CSV and JSON exports from popular platforms.
- Folder-watch imports for NAS or device-drop workflows.

### Race Module

- Race calendar with date, location, distance, goal, result, and notes.
- Course map and elevation profile.
- Association between a race and one or more activities.
- Race reports with splits, pacing, weather notes, and personal reflection.

### Analytics

- Weekly/monthly/yearly trends.
- Best efforts and personal records.
- Segment-like efforts on saved routes.
- Intervals and workout structure detection.
- Training load, fitness/fatigue, monotony, and freshness models.
- Heart rate zones, pace zones, power zones, and distribution charts.
- Heart-rate zones should be configurable by the admin when heart-rate data is available.
- If no custom heart-rate zones are configured, the app may offer sensible defaults based on max heart rate or threshold heart rate when known.
- Activity detail should show time in heart-rate zones for activities with heart-rate samples.
- Dashboard analytics should summarize heart-rate zone distribution across the active time scale and activity filters where enough data exists.
- Health metrics such as resting HR, HRV, sleep, body weight, and subjective notes if provider support exists.
- Grade Adjusted Pace (GAP), sourced directly from provider/workout data when available and computed from route grade and pace when not.
- Interactive chart zooming and panning for inspecting specific sections of an activity.
- Activity detail charts should allow collapsing separate metric graphs into one overlaid inspection graph where that improves comparison.
- Selection tools on elevation/profile charts to calculate distance, elevation gain/loss, average grade, pace/speed, heart rate, and other available metrics for the selected range.
- Synchronized chart hover so inspecting one activity graph highlights the matching point on all other graphs and places a marker at the corresponding location on the map.
- Activity tagging for custom organization such as workout type, terrain, commute, race, injury context, or personal labels.

### Preferences and Localization

- Metric and imperial unit switching.
- Per-user display preferences once multi-user support exists.
- Locale-aware dates, times, number formatting, and week-start settings.

### Gear

- Shoe and bike tracking.
- Mileage and retirement alerts.
- Automatic assignment rules based on sport, source, route, or user preference.

### Routes and Maps

- Saved route library.
- Saved courses with map, distance, elevation profile, climb metadata, and activity history on the course.
- Personal heatmaps generated from stored GPS activity samples.
- Heatmaps must respect the active activity filters and time scale, including activity type include/exclude rules and future source/metric filters.
- Heatmaps should support at least all activities, runs only, and rides only, with room for additional sport-specific views.
- Heatmap generation may be computed on demand initially, but should be cacheable or precomputed once activity volume makes rendering slow.
- Climb detection and climb summaries.
- Automatic climb detection with grade, length, elevation gain, and difficulty classification.
- Route comparison and repeated-course history.
- Optional self-hosted map tiles.

### Automation and Portability

- Scheduled syncs.
- Public webhook setup helper for hosted deployments.
- Backup and restore jobs.
- Full export of canonical data and raw provider/file payloads.
- API tokens for local integrations.

### Multi-User

- Local accounts.
- Per-user provider connections.
- Household/team deployment mode.
- User-scoped privacy controls.
- Optional OIDC or reverse-proxy auth integration.

## 9. V1 Acceptance Criteria

- A fresh clone can start with `docker compose up --build`.
- The app applies migrations and serves the UI.
- The admin can log in with configured credentials.
- The UI automatically follows the system light/dark preference and supports a persistent manual theme override.
- The admin can reach a Settings page for provider connections, display preferences, data import tools, and diagnostics.
- The admin can upload a sample GPX, TCX, or FIT file from the secondary manual import area.
- The admin can upload a Strava account export archive from the secondary manual import area and import supported activities without configuring Strava OAuth/API credentials.
- Uploaded activities appear in the dashboard and activity list.
- The admin can change the dashboard time scale and see activity totals/charts update for the selected scale.
- The admin can search activities by name, filter by type/source/metrics/location, and sort the activity list by date, name, type, source, distance, time, and elevation.
- Dashboard totals and charts update consistently when activity filters are applied.
- A GPS activity detail page renders a map and charts.
- The admin can zoom an activity route map with the mouse wheel.
- The admin can delete an activity and it no longer appears in the dashboard, activity list, or detail view.
- Strava OAuth routes are present and report clear configuration errors when credentials are missing.
- With Strava credentials configured, the admin can connect Strava and trigger a manual import.
- Strava sync imports historical accessible activities as well as new activities, while repeated syncs avoid duplicate activity rows.
- The admin can see whether provider sync is running, completed, or failed, including the latest result and error details.
- Duplicate imports do not create duplicate activities.
