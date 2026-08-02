# Web and PWA smoke test

Run this checklist after backend or frontend changes. The Google Pixel 8 Pro
in Chrome is the primary mobile acceptance profile; repeat the layout checks
at a smaller phone width and on desktop.

## Automated checks

From the repository root:

```sh
env GOCACHE=/tmp/runnarr-go-cache go test ./...
env GOCACHE=/tmp/runnarr-go-cache go test -race ./...
env GOCACHE=/tmp/runnarr-go-cache go vet ./...
test -z "$(gofmt -l cmd internal)"
cd web && npm test
cd web && npm run build
```

## Browser smoke test

1. Open the deployment over HTTPS and verify the manifest, service worker,
   icons, and favicon load successfully.
2. Log in with local credentials. Verify invalid credentials show an actionable
   error and do not create a partial session.
3. If configured, complete Google login and return to the dashboard.
4. Open the browser install prompt and launch Runnarr in standalone mode.
5. Refresh the standalone app and verify the session and persisted theme remain
   available.
6. Switch Dashboard between weekly, monthly, and yearly distance scales and
   apply activity-type filters.
7. Search, sort, paginate, and open an activity. Verify its bounded chart,
   route map, laps, intervals, gear, notes, media, and secondary actions.
8. Open Calendar, change month, and open an activity from the agenda.
9. Open Health and verify the backend-provided seven-day default, short-range
   bar charts, longer-range line charts, daily selection, and retry behavior.
10. Open Settings, Tools, Gear, imports, Garmin status, and sync controls.
11. Confirm direct navigation to each route still works after a page refresh.

## Notification and push test

1. Open Settings > Notifications and verify the four category defaults:
   workout changes, Garmin calendar, and sheet writeback use in-app plus push;
   activity matching uses in-app only.
2. Change a category to each mode and refresh. Confirm the saved mode remains.
3. Enable the current browser with a recognizable device name. Permission must
   be requested only after clicking Enable this device.
4. Click Test for that device with Runnarr in the background. Confirm the OS
   notification appears and opens Settings > Notifications when clicked.
5. On a phone, repeat from the installed PWA, fully close it, click Test from a
   second session, and confirm the notification still arrives and opens the
   intended route after login. On iPhone or iPad, first add Runnarr to the Home
   Screen and enable notifications from the installed app.
6. Generate or change a structured training-plan workout and reconcile Garmin.
   Confirm one inbox thread accumulates its workout and Garmin timeline events,
   the unread badge updates, and Open scrolls to the Garmin status.
7. Exercise an automatic match and a failed/partial then successful sheet
   writeback. Confirm their links open the activity and writeback status.
8. Mark one thread read/unread, use Mark all read, clear read history, and load
   more than one page. Confirm other users never see those records.
9. Rename and remove a push device. Log out from another enabled browser and
   confirm its server subscription and browser subscription are removed.

## Pixel-sized layout test

At the Pixel 8 Pro mobile viewport, verify:

- no horizontal scrolling or clipped primary controls;
- fixed header and bottom navigation respect status/navigation safe areas;
- cards, forms, charts, maps, tables, dialogs, and activity detail sections
  remain readable and do not overlap;
- switching tabs and opening activity details stays responsive;
- the bottom navigation does not cover the last content or dialog actions.

Repeat the same checks at 320px wide and at a desktop viewport.

## Cache and offline test

- API and activity-media requests must not appear in Cache Storage.
- After a new frontend build, the service worker must update and remove stale
  shell caches.
- With the network disabled, the shell may load, but private activity and
  health data must show a retryable network state rather than stale cached data.
- Authenticated notification APIs and push subscription details must not appear
  in Cache Storage.

Do not rebuild or restart a deployment while a Garmin or training-sheet sync is
running. Use fixtures or a dedicated test account for destructive actions.
