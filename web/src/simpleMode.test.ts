import { describe, expect, it } from "vitest";
import type { Activity } from "./types";
import { fullPathForSimplePath, normalizeSimpleMatchFilter, shouldRedirectToSimple, simpleIntervalSummary, simpleMatchStatusLabel } from "./simpleMode";

const activity = (updates: Partial<Activity> = {}): Activity => ({
  id: "activity-1",
  source: "garmin",
  sourceId: "source-1",
  name: "Morning run",
  sourceName: "Morning run",
  sportType: "Run",
  startTime: "2026-07-31T07:00:00Z",
  distanceM: 10000,
  movingTimeS: 3000,
  elapsedTimeS: 3000,
  elevationGainM: 50,
  createdAt: "2026-07-31T08:00:00Z",
  ...updates
});

describe("simple matching mode", () => {
  it("normalizes list filters", () => {
    expect(normalizeSimpleMatchFilter("attention")).toBe("attention");
    expect(normalizeSimpleMatchFilter("invalid")).toBe("all");
  });

  it("redirects only the root landing outside support mode", () => {
    expect(shouldRedirectToSimple("/", "simple")).toBe(true);
    expect(shouldRedirectToSimple("/activities", "simple")).toBe(false);
    expect(shouldRedirectToSimple("/", "simple", true)).toBe(false);
  });

  it("maps simple pages back to their full equivalents", () => {
    expect(fullPathForSimplePath("/simple")).toBe("/activities");
    expect(fullPathForSimplePath("/simple/activities/abc")).toBe("/activities/abc");
  });

  it("formats match and interval summaries", () => {
    expect(simpleMatchStatusLabel(activity())).toBe("Unmatched");
    expect(simpleMatchStatusLabel(activity({ trainingSheetMatch: { state: "attention" } }))).toBe("Needs attention");
    expect(simpleIntervalSummary(activity())).toBe("Continuous run");
    expect(simpleIntervalSummary(activity({ workout: { provider: "garmin", name: "Tempo" }, intervals: [
      { index: 0, category: "active", elapsedTimeS: 600, movingTimeS: 600, distanceM: 2000 }
    ] }))).toBe("Tempo · 1 structured interval · active");
  });
});
