import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "./api";

describe("shared backend API contract", () => {
  afterEach(() => {
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
});
