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
});
