import { describe, expect, it } from "vitest";
import { hasIntervalAnalysis, resolveActivityAnalysisTab } from "./activityAnalysis";
import type { ActivityInterval, ActivityLap } from "./types";

describe("hasIntervalAnalysis", () => {
  it("rejects activities without structured intervals or laps", () => {
    expect(hasIntervalAnalysis(undefined)).toBe(false);
    expect(hasIntervalAnalysis({})).toBe(false);
    expect(hasIntervalAnalysis({ intervals: [], laps: [] })).toBe(false);
  });

  it("accepts structured-interval and lap-only activities", () => {
    expect(hasIntervalAnalysis({ intervals: [{} as ActivityInterval] })).toBe(true);
    expect(hasIntervalAnalysis({ laps: [{} as ActivityLap] })).toBe(true);
  });
});

describe("resolveActivityAnalysisTab", () => {
  it("falls back to Stats when Intervals is unavailable", () => {
    expect(resolveActivityAnalysisTab("intervals", false)).toBe("stats");
  });

  it("preserves valid Stats and Intervals selections", () => {
    expect(resolveActivityAnalysisTab("stats", false)).toBe("stats");
    expect(resolveActivityAnalysisTab("stats", true)).toBe("stats");
    expect(resolveActivityAnalysisTab("intervals", true)).toBe("intervals");
  });
});
