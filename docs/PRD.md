# Runnarr Product Requirements Document

## 1. Product Summary

Runnarr is a self-hosted activity hub for endurance data. It imports local activity files, normalizes them into a durable schema, and presents a private, useful view of training history with maps, charts, and race context.

The first release is intentionally not a full training science platform. It should make the data easy to own, browse, verify, and extend. Deep analysis similar to Runalyze or intervals.icu is future scope.

## 2. Goals

- Provide a Dockerized service that can run on a home server, NAS, VPS, or local machine.
- Import local activity files with an extensible parser architecture.
- Store activities in a canonical schema that is independent from any one file format.
- Render a useful activity dashboard, activity list, detail view, route map, and basic charts.
- Establish a durable architecture for future providers, import formats, races, analytics, and multi-user support.

## 3. Non-Goals for V1

- Public social feed, public sharing, comments, kudos, leaderboards, or third-party community features.
- Writing activities back to external providers.
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
- Imported activity data must only be displayed to the local authenticated admin.

### Activity Imports

- V1 must import GPX, TCX, and FIT files.
- The importer must deduplicate repeated uploads by file hash and provider source identifiers.
- The import pipeline must normalize:
  - activity name
  - sport type
  - start time
  - elapsed and moving time where available
  - distance
  - elevation gain
  - heart rate summary where available
  - GPS samples
  - laps where available
- The parser architecture must allow new formats without touching API handlers.

### Activity Views

- Dashboard must show total activities, recent activity, total distance, total time, and recent weekly distance.
- Activity list must support scanning, searching, filtering by activity type/date, and sorting by date, distance, duration, elevation, and pace.
- Activity detail must show summary metrics, a route map when GPS samples exist, and charts for elevation, pace/speed, and heart rate where data exists.
- Activity detail should support overlaying compatible chart series in a single combined graph, such as elevation, pace/speed, heart rate, power, and cadence, with clear axes, legends, and per-series toggles.
- Activity detail should detect meaningful climbs from distance/elevation samples and summarize each climb with distance, ascent, average grade, difficulty, and map/profile highlighting.
- Climb detection thresholds should start with sensible defaults and may later become user-configurable settings.
- Route maps must support mouse-wheel zooming for detailed activity inspection.
- Route maps should show start and end markers, and should eventually support well-designed direction indicators along the route without cluttering the map.
- The admin must be able to delete activities from Runnarr, including their samples and laps.
- Import views must make the data pipeline visible enough to debug failed imports.

## 6. UX Principles

- Runnarr is an operational tool, not a marketing site.
- First screen after login should be the usable dashboard.
- UI should be dense but readable, with restrained styling and clear navigation.
- Maps and charts should favor inspection and accuracy over decorative presentation.
- Empty states should explain what action unlocks the page without over-explaining the product.

## 7. Canonical Data Model

The storage model should separate canonical activity fields from provider/raw details:

- `activities`: one row per normalized activity.
- `activity_samples`: time-series samples such as position, elevation, heart rate, cadence, power, distance, and speed.
- `activity_laps`: lap or split summaries from providers/files.
- `import_files`: uploaded file metadata and parser status.
- `provider_connections`: future external provider account metadata.
- `sync_jobs`: future provider sync/backfill job state.
- `auth_sessions`: local admin sessions and CSRF token state.

V1 is single-user, but future multi-user support should be possible by adding ownership columns and access control checks.

## 8. Future Roadmap

### Providers

- Garmin Connect or Garmin export ingestion.
- Wahoo, COROS, Polar, Suunto, Zwift, Komoot, TrainingPeaks, and direct device sync where practical.
- Provider-specific metadata preservation without polluting canonical activity fields.

### Import Formats

- Bulk ZIP archives.
- Provider account exports.
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
- Health metrics such as resting HR, HRV, sleep, body weight, and subjective notes if provider support exists.
- Grade Adjusted Pace (GAP), sourced directly from provider/workout data when available and computed from route grade and pace when not.
- Interactive chart zooming and panning for inspecting specific sections of an activity.
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
- Personal heatmaps.
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
- The admin can upload a sample GPX, TCX, or FIT file.
- Uploaded activities appear in the dashboard and activity list.
- A GPS activity detail page renders a map and charts.
- Duplicate imports do not create duplicate activities.
