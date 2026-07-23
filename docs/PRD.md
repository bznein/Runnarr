# Runnarr Product Requirements Document

## 1. Product Summary

Runnarr is a self-hosted activity hub for endurance data. It imports activities from provider APIs and local files, normalizes them into a provider-independent schema, and presents a private, useful view of training history with maps, charts, and race context.

The first release is intentionally not a full training science platform. It should make the data easy to own, browse, verify, and extend. Deep training analysis is future scope.

## 2. Goals

- Provide a Dockerized service that can run on a home server, NAS, VPS, or local machine.
- Connect to Garmin Connect and import the authenticated user's activities without requiring official Garmin API approval.
- Import local activity files with an extensible parser architecture.
- Store activities in a canonical schema that is independent from Garmin or any one file format.
- Render a useful activity dashboard, activity list, detail view, route map, and basic charts.
- Establish a durable architecture for future providers, import formats, races, analytics, and trusted local multi-user support.

## 3. Non-Goals for V1

- Public social feed, public sharing, comments, kudos, leaderboards, or third-party community features.
- Writing activities back to Garmin or other providers.
- Deep physiological analytics, training load modeling, interval detection, structured workouts, or coach workflows.
- Public signup, email recovery, and hosted multi-tenant administration.
- Native mobile apps.
- Self-hosted map tile server.

## 4. Target User

The users are technically comfortable runners, cyclists, triathletes, or data-minded endurance athletes who want private ownership and a consolidated view of their activity data. The deployment may serve a trusted household or small local team.

The default deployment still works for one person, but local account management is part of the supported product. Administrators create and disable username/password accounts, reset passwords, and can enter an explicit read-only support view. There is no public signup or email-based recovery.

## 5. V1 Requirements

### Deployment

- The app must run via Docker Compose with an application container and a Postgres container.
- Configuration must be environment driven.
- The app must expose one HTTP port and serve both API and frontend.
- Database migrations must run automatically at startup.

### Authentication and Privacy

- The default local deployment must require a local account login.
- An explicit public deployment mode must support Google OIDC login with a
  verified-email allowlist mapped to existing local accounts, without public
  signup; local password login must be disabled by default in that mode.
- The first configured admin account must be bootstrapped from `RUNNARR_ADMIN_USERNAME` and the existing password/password-hash environment variables.
- Administrators must be able to create, disable, and reset local accounts.
- Activities, health metrics, imports, provider connections, sync jobs, gear, planned activities, and user preferences must be private to their owning account.
- Administrators must be able to enter an explicit read-only support view for another account and exit it without changing the target account's data.
- Sessions must use HTTP-only cookies.
- Mutating API calls must require CSRF protection.
- Provider tokens or token files must be stored with restricted access.
- Imported activity data must only be displayed to the authenticated account that owns it, except while an administrator is in read-only support view.

### Activity Imports

- V1 must import GPX, TCX, and FIT files.
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
- Provider imports should preserve a link to the original provider activity when the source exposes a stable activity URL or enough source metadata to construct one.
- The parser architecture must allow new formats without touching API handlers.

### Garmin Provider

- Garmin Connect is the primary automatic import path for v1.
- The Garmin integration may use an unofficial Garmin Connect client because official Garmin Activity API access requires approval.
- It must support Garmin login from Settings, including MFA when Garmin requires it.
- It must store reusable Garmin token files and provider account metadata without storing the Garmin password.
- It must support manual sync with an oldest-date selector for full historical backfill.
- It must support scheduled/background sync for newly uploaded Garmin activities.
- Re-running sync must update existing Garmin activities by Garmin activity ID and add newly discovered activities without creating duplicates.
- Garmin original activity downloads should be parsed from FIT where available, including session-level elevation gain and lap data.
- The provider settings UI should make clear that importing requires connecting Garmin first, then triggering sync.
- Matched training-sheet activities should write structured workout table rows back to Google Sheets when Garmin workout metadata is available, including exact repetitions, weighted aggregate rows, and fastest/slowest repetitions; ambiguous mappings must preserve existing cells and report a warning.

### Garmin Health Metrics

- The app should import daily Garmin health metrics as a separate sync workflow from activity import.
- Health sync should support a default recent backfill and a user-selected older start date for full-history backfills.
- The first health metric slice should normalize steps, calories, resting heart rate, sleep, stress, body battery, HRV, and weight/body composition where Garmin provides them.
- Daily body battery should emphasize gained, drained, and highest values; Garmin-style intraday body battery curves are future scope.
- Health metric storage should preserve nullable normalized fields plus raw Garmin payloads so provider-specific gaps can be debugged without losing data.
- The UI should include a top-level Health page with date-range controls, sync status, summary cards, trend charts, daily rows, and day-level details.
- Missing health metrics should be omitted or left blank rather than rendered as placeholder values.
- Garmin health support is read-only; the app must not write hydration, body composition, blood pressure, or other health values back to Garmin.

### Garmin Gear

- The app should import Garmin gear such as running shoes and cycling equipment as a read-only sync workflow.
- Gear sync should be manually triggerable from Settings and the dedicated Gear page, and Garmin activity sync should refresh gear afterward when possible.
- The first gear integration should normalize gear name, type, brand, model, retired state, Garmin-provided total distance, optional distance limit, first/last use dates, default activity types, and raw provider payloads.
- Gear assigned to Garmin activities should be linked to matching local activities when those activities have already been imported.
- The UI should include a top-level Gear page with active and retired gear sections, gear detail views, Garmin mileage, and assigned activity history.
- Activity list rows and activity detail pages should show assigned gear when present.
- Missing gear fields should be omitted rather than rendered as placeholder values.
- Garmin gear support is read-only; the app must not create, edit, retire, assign, or remove gear in Garmin in the first implementation.

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
- Activity detail should show an "Open original" link for provider-imported activities when an original provider activity URL is available; manual file imports should not show this link unless their source includes a meaningful external URL.
- Activity detail should allow locally renaming an activity without modifying the original provider activity or imported source file.
- Activity detail should allow local notes on an activity without modifying the original provider activity or imported source file.
- Activity detail should allow exporting a GPX track from stored GPS samples, with a user choice for whether to include sensor extensions such as heart rate, cadence, power, and speed.
- Activity detail should use a compact overflow menu for secondary actions such as rename and delete instead of showing destructive actions as primary page actions.
- Activity detail should support overlaying compatible chart series in a single combined graph, such as elevation, pace/speed, heart rate, power, and cadence, with clear axes, legends, and per-series toggles.
- Activity detail, list rows, and dashboard summaries should use sport-appropriate metrics, units, and labels rather than one generic endurance format for every activity.
- Pace should be formatted by activity type, for example min/km for running, speed for cycling where appropriate, and min/100 m for swimming.
- Each supported activity type should be reviewed for which metrics are meaningful to show, hide, or rename, including distance, pace/speed, elevation, cadence, power, laps, intervals, heart rate, and route maps.
- Activity detail should hide elevation charts for activity types where elevation is not meaningful, such as swimming, strength training, indoor workouts, and similar non-route activities.
- Activity detail should detect meaningful climbs from distance/elevation samples and summarize each climb with distance, ascent, average grade, average pace, Grade Adjusted Pace (GAP) where available, difficulty, and map/profile highlighting.
- Climb detection thresholds should start with sensible defaults, be configurable in Settings, and support a temporary activity-detail sensitivity override for inspection.
- Activity laps imported from providers should preserve provider interval metadata where available, including Garmin workout step or lap category fields if exposed by the source data.
- Activity detail should allow filtering laps/intervals by provider category, such as warm-up, active interval, recovery, cool-down, and other provider-defined labels when available.
- Garmin activities with an associated structured workout should preserve the provider workout definition, repeat structure, step targets, grouped interval summaries, and the lap indexes contributing to each interval.
- Activity detail should present structured workout intervals as grouped rows with expandable recorded laps, cumulative timing, target information, and sport-appropriate interval metrics; activities without structured workout metadata should retain the flat lap view.
- Route maps must support mouse-wheel zooming for detailed activity inspection.
- Route maps should show start and end markers, and should eventually support well-designed direction indicators along the route without cluttering the map.
- Activity detail should allow attaching photo media to an activity.
- Attached media should preserve useful metadata, including capture time and EXIF GPS coordinates when present.
- Activity detail should show attached media in a gallery; media with EXIF location should also appear on the activity map as small thumbnail markers at the recorded location.
- Media thumbnail markers should be unobtrusive and should open or highlight the corresponding media item when selected.
- The admin must be able to delete activities from Runnarr, including their samples and laps, without deleting provider connections or source files outside the app.
- Settings must make the data pipeline visible enough to debug failed imports or syncs without making manual import the dominant workflow.
- Provider imports should populate calories/energy expenditure on activities when the provider exposes it.

## 6. UX Principles

- Runnarr is an operational tool, not a marketing site.
- First screen after login should be the usable dashboard.
- UI should be dense but readable, with restrained styling and clear navigation.
- The app should include a recognizable favicon/app icon for browser tabs, bookmarks, and installable/PWA contexts.
- UI must support light and dark themes, defaulting automatically to the user's system color-scheme preference.
- Each user should be able to override the automatic theme preference and persist that choice per account.
- Maps and charts should favor inspection and accuracy over decorative presentation.
- Empty states should explain what action unlocks the page without over-explaining the product.

## 7. Canonical Data Model

The storage model should separate canonical activity fields from provider/raw details:

- `activities`: one row per normalized activity, including local-only name/notes overrides, calories/energy expenditure, and an original provider activity URL when provided by the source.
- `activity_samples`: time-series samples such as position, elevation, heart rate, cadence, power, distance, and speed.
- `activity_laps`: lap or split summaries from providers/files.
- `activity_media`: media attached to activities, including file metadata, thumbnail paths, capture time, and optional EXIF-derived latitude/longitude.
- `gears`: provider gear records with normalized metadata, Garmin distance totals, active/retired state, and raw provider payloads.
- `activity_gears`: many-to-many links between local activities and imported provider gear.
- `import_files`: uploaded file metadata and parser status.
- `provider_connections`: external provider account metadata; provider token files or credentials must be protected with restricted storage access.
- `sync_jobs`: provider sync/backfill job state.
- `users`: local accounts, roles, password hashes, disabled state, and login timestamps.
- `user_settings`: per-user display, activity table, climb-detection, training-sheet, and planning preferences.
- `auth_sessions`: local sessions, CSRF token state, actor identity, and optional support-view target.

All top-level user-owned records carry a non-null owner reference after bootstrap. Child samples, laps, media, and gear assignments remain scoped through their owning activity or gear record. Existing legacy records are assigned to the first bootstrapped administrator during upgrade.

## 8. Future Roadmap

### Providers

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
- Training load, fitness/fatigue, monotony, and freshness models.
- Heart rate zones, pace zones, power zones, and distribution charts.
- Heart-rate zones should be configurable by the admin when heart-rate data is available.
- If no custom heart-rate zones are configured, the app may offer sensible defaults based on max heart rate or threshold heart rate when known.
- Activity detail should show time in heart-rate zones for activities with heart-rate samples.
- Dashboard analytics should summarize heart-rate zone distribution across the active time scale and activity filters where enough data exists.
- Additional health metrics and wellness context such as SpO2, respiration, hydration, blood pressure, subjective notes, intraday body battery charts, and other intraday health time series if provider support exists.
- Video attachments for activities, without requiring EXIF or location metadata support.
- Grade Adjusted Pace (GAP), sourced directly from provider/workout data when available and computed from route grade and pace when not; climb summaries should use the same GAP source/calculation so climb-level GAP is comparable with activity-level GAP.
- Interactive chart zooming and panning for inspecting specific sections of an activity.
- Activity detail charts should allow collapsing separate metric graphs into one overlaid inspection graph where that improves comparison.
- Selection tools on elevation/profile charts to calculate distance, elevation gain/loss, average grade, pace/speed, heart rate, and other available metrics for the selected range.
- Synchronized chart hover so inspecting one activity graph highlights the matching point on all other graphs and places a marker at the corresponding location on the map.
- Activity tagging for custom organization such as workout type, terrain, commute, race, injury context, or personal labels.

### Preferences and Localization

- Metric and imperial unit switching.
- Additional locale and unit preferences per user.
- Locale-aware dates, times, number formatting, and week-start settings.

### Gear

- Editing and retiring gear from Runnarr when provider support and write safety are understood.
- Mileage and retirement alerts.
- Automatic assignment rules based on sport, source, route, or user preference.
- Non-Garmin gear providers and manually managed gear.

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

- Optional self-hosted OIDC and reverse-proxy deployment integration.
- Household/team deployment improvements and user-facing audit history.
- User-scoped export, backup, and retention controls.
- Shared datasets or controlled collaboration, if a future use case requires them.

## 9. V1 Acceptance Criteria

- A fresh clone can start with `docker compose up --build`.
- The app applies migrations and serves the UI.
- The configured administrator can log in with the bootstrap credentials.
- An administrator can create a local user, and that user can log in with the assigned credentials.
- Each account sees only its own activity, health, import, provider, gear, planning, and preference data.
- An administrator can enter a read-only support view for another account and exit it.
- The UI automatically follows the system light/dark preference and supports a persistent manual theme override.
- The admin can reach a Settings page for provider connections, display preferences, data import tools, and diagnostics.
- The admin can upload a sample GPX, TCX, or FIT file from the secondary manual import area.
- Uploaded activities appear in the dashboard and activity list.
- The admin can change the dashboard time scale and see activity totals/charts update for the selected scale.
- The admin can search activities by name, filter by type/source/metrics/location, and sort the activity list by date, name, type, source, distance, time, and elevation.
- Dashboard totals and charts update consistently when activity filters are applied.
- A GPS activity detail page renders a map and charts.
- The admin can zoom an activity route map with the mouse wheel.
- The admin can delete an activity and it no longer appears in the dashboard, activity list, or detail view.
- The admin can connect Garmin from Settings and trigger a manual sync.
- Garmin sync imports historical accessible activities as well as new activities, while repeated syncs avoid duplicate activity rows.
- Scheduled Garmin sync imports newly uploaded activities after initial connection.
- The admin can manually sync Garmin gear, see active and retired gear on the Gear page, and inspect assigned local activities for each gear item.
- The admin can see whether provider sync is running, completed, or failed, including the latest result and error details.
- Duplicate imports do not create duplicate activities.
- Matched structured workouts populate the corresponding training-sheet interval table without overwriting existing values, while activities without sufficient structured metadata leave the table unchanged and expose a reviewable warning.
- Before matching a Garmin activity to a planned training-sheet run, the user can preview summary, interval, and feedback cell changes, review preserved conflicts, and explicitly apply the match and writeback after live-sheet revalidation.
- The training-sheet preview presents the matched day and workout section in a spreadsheet-like live grid, highlighting writable cells and preserved conflicts in their sheet positions with current/proposed cell details.
