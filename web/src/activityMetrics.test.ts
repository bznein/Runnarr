import { describe, expect, it } from "vitest";
import { supportsRouteMetrics } from "./activityMetrics";

describe("activity metric visibility", () => {
  it.each([
    "Strength",
    "Strength Training",
    "strength_training",
    "Weight Training",
    "WeightTraining",
    "Weightlifting"
  ])("hides route metrics for %s", (sportType) => {
    expect(supportsRouteMetrics(sportType)).toBe(false);
  });

  it.each(["Running", "Cycling", "Swimming", "Walking"])("keeps route metrics for %s", (sportType) => {
    expect(supportsRouteMetrics(sportType)).toBe(true);
  });
});
