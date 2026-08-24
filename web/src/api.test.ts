import { afterEach, describe, expect, it, vi } from "vitest";
import { api, setCsrfToken } from "./api";

describe("shared backend API contract", () => {
  afterEach(() => {
    setCsrfToken("");
    vi.unstubAllGlobals();
  });

  it("requests the server-owned dashboard period with the same filters used by other web views", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      activityCount: 2,
      distanceM: 12000,
      movingTimeS: 3600,
      elevationGainM: 100,
      recent: [],
      distanceBuckets: []
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.summary({ sports: ["running"], excludeSports: ["walking"], search: "morning" }, "monthly");

    const requestURL = String(fetchMock.mock.calls[0][0]);
    expect(requestURL).toContain("/api/stats/summary?");
    expect(requestURL).toContain("sport=running");
    expect(requestURL).toContain("excludeSport=walking");
    expect(requestURL).toContain("search=morning");
    expect(requestURL).toContain("period=monthly");
  });

  it("uses bounded activity-series requests for chart and map inspection", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      samples: [],
      points: [],
      totalSamples: 5000,
      sampled: true
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.activitySeries("activity/with spaces", 900);

    const requestURL = String(fetchMock.mock.calls[0][0]);
    expect(requestURL).toContain("/api/activities/activity%2Fwith%20spaces/series");
    expect(requestURL).toContain("maxPoints=900");
  });

  it("requests account-scoped AI context in the browser timezone", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      activityDate: "2026-08-20",
      windowStart: "2026-08-14",
      windowEnd: "2026-08-20",
      totals: { runCount: 0, distanceM: 0, movingTimeS: 0, elevationGainM: 0 },
      runs: []
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.activityAIContext("activity/with spaces", "Europe/Dublin");

    const requestURL = String(fetchMock.mock.calls[0][0]);
    expect(requestURL).toContain("/api/activities/activity%2Fwith%20spaces/ai-context?");
    expect(requestURL).toContain("timezone=Europe%2FDublin");
  });

  it("passes the browser timezone to calendar requests", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      date: "2026-07-01",
      activities: []
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.calendarDay("2026-07-01", "Europe/Dublin");

    const requestURL = String(fetchMock.mock.calls[0][0]);
    expect(requestURL).toContain("/api/stats/calendar/day?");
    expect(requestURL).toContain("date=2026-07-01");
    expect(requestURL).toContain("timezone=Europe%2FDublin");
  });

  it("requests activity neighbors with the current list filters and sort", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      previousId: "newer-activity",
      nextId: "older-activity"
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.activityNavigation("activity/with spaces", {
      sports: ["Running"],
      excludeSports: [],
      search: "morning run",
      sortBy: "distance",
      sortOrder: "asc"
    });

    const requestURL = String(fetchMock.mock.calls[0][0]);
    expect(requestURL).toContain("/api/activities/activity%2Fwith%20spaces/navigation?");
    expect(requestURL).toContain("sport=Running");
    expect(requestURL).toContain("search=morning+run");
    expect(requestURL).toContain("sortBy=distance");
    expect(requestURL).toContain("sortOrder=asc");
  });

  it("requests the status-aware training-sheet matching view", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      activities: [], limit: 100, offset: 0, hasMore: false
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.activities(undefined, { limit: 100, offset: 0, view: "training-sheet-matching", matchState: "attention" });

    const requestURL = String(fetchMock.mock.calls[0][0]);
    expect(requestURL).toContain("view=training-sheet-matching");
    expect(requestURL).toContain("matchState=attention");
  });

  it("scans and applies one confirmed training-sheet reconciliation", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ nextOffset: 4, scanned: 4, skipped: 0, done: false }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ updated: 1 }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    setCsrfToken("reconcile-csrf");

    await api.nextTrainingSheetReconciliation("2026-05-01", 0);
    await api.applyTrainingSheetReconciliation({ plannedActivityId: "planned", activityId: "activity", fingerprint: "live-sheet" });

    expect(String(fetchMock.mock.calls[0][0])).toContain("/api/training-sheet/reconciliation?notBefore=2026-05-01&offset=0");
    const [path, init] = fetchMock.mock.calls[1];
    expect(path).toBe("/api/training-sheet/reconciliation");
    expect(init.method).toBe("POST");
    expect(init.headers.get("X-CSRF-Token")).toBe("reconcile-csrf");
    expect(JSON.parse(init.body)).toEqual({ plannedActivityId: "planned", activityId: "activity", fingerprint: "live-sheet" });
  });

  it("encodes notification pagination and unread filters", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      notifications: [],
      unreadCount: 0
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.notifications({ limit: 25, cursor: "cursor/value", unread: true });

    const requestURL = String(fetchMock.mock.calls[0][0]);
    expect(requestURL).toContain("/api/notifications?");
    expect(requestURL).toContain("limit=25");
    expect(requestURL).toContain("cursor=cursor%2Fvalue");
    expect(requestURL).toContain("unread=true");
  });

  it("sends notification preference mutations with CSRF protection", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      categories: { activity_matching: "in_app" }
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    setCsrfToken("test-csrf");

    await api.updateNotificationSettings({
      workout_changes: "in_app_push",
      garmin_calendar: "in_app_push",
      activity_matching: "in_app",
      sheet_writeback: "in_app_push"
    });

    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/notification-settings");
    expect(init.method).toBe("PATCH");
    expect(init.headers.get("X-CSRF-Token")).toBe("test-csrf");
    expect(JSON.parse(init.body)).toMatchObject({ categories: { activity_matching: "in_app" } });
  });

  it("updates the opt-in weather fallback with CSRF protection", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ openMeteoFallbackEnabled: true }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    setCsrfToken("weather-csrf");

    await api.updateWeatherConfig({ openMeteoFallbackEnabled: true });

    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/config/weather");
    expect(init.method).toBe("PATCH");
    expect(init.headers.get("X-CSRF-Token")).toBe("weather-csrf");
    expect(JSON.parse(init.body)).toEqual({ openMeteoFallbackEnabled: true });
  });

  it("encodes course library filters", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      courses: [], limit: 50, offset: 0, hasMore: false
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.courses({ q: "river loop", sport: "Run", favorite: true, sort: "updated", order: "desc", limit: 50 });

    const requestURL = String(fetchMock.mock.calls[0][0]);
    expect(requestURL).toContain("/api/courses?");
    expect(requestURL).toContain("q=river+loop");
    expect(requestURL).toContain("sport=Run");
    expect(requestURL).toContain("favorite=true");
    expect(requestURL).toContain("sort=updated");
    expect(requestURL).toContain("order=desc");
  });

  it("re-uploads the reviewed GPX and selections when committing a course import", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      importId: "import-1", filename: "route.gpx", fileSHA256: "abc", created: []
    }), { status: 201, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    setCsrfToken("course-csrf");
    const file = new File(["<gpx />"], "route.gpx", { type: "application/gpx+xml" });

    await api.commitCourseImport(file, "abc", [{ key: "track:1", name: "River", sportType: "Run", notes: "Quiet roads" }]);

    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/course-imports/commit");
    expect(init.method).toBe("POST");
    expect(init.headers.get("X-CSRF-Token")).toBe("course-csrf");
    expect(init.body).toBeInstanceOf(FormData);
    expect((init.body as FormData).get("file")).toBe(file);
    expect(JSON.parse(String((init.body as FormData).get("input")))).toEqual({
      fileSHA256: "abc",
      selections: [{ key: "track:1", name: "River", sportType: "Run", notes: "Quiet roads" }]
    });
  });

  it("routes course waypoints through the backend without exposing a routing origin", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      routingEnabled: true,
      legs: []
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    await api.routeCourseLegs({
      sportType: "Cycling",
      waypoints: [{ index: 0, latitude: 53.3, longitude: -6.2 }, { index: 1, latitude: 53.4, longitude: -6.1 }],
      directLegIndexes: [0]
    });

    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/course-routing/legs");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body)).toEqual({
      sportType: "Cycling",
      waypoints: [{ index: 0, latitude: 53.3, longitude: -6.2 }, { index: 1, latitude: 53.4, longitude: -6.1 }],
      directLegIndexes: [0]
    });
  });

  it("encodes place searches through the Runnarr backend", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ results: [] }), {
      status: 200,
      headers: { "Content-Type": "application/json" }
    }));
    vi.stubGlobal("fetch", fetchMock);

    await api.searchCoursePlaces("St Stephen's Green & lake");

    expect(fetchMock.mock.calls[0][0]).toBe("/api/course-geocoding/search?q=St%20Stephen's%20Green%20%26%20lake");
  });

  it("sends a course revision to Garmin with CSRF protection", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      connected: true, current: true, status: "sent", providerCourseId: "321"
    }), { status: 201, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    setCsrfToken("course-csrf");

    await api.sendCourseToGarmin("course/with spaces");

    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/courses/course%2Fwith%20spaces/garmin");
    expect(init.method).toBe("POST");
    expect(init.headers.get("X-CSRF-Token")).toBe("course-csrf");
  });
});
