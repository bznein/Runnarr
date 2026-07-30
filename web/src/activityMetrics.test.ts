import { describe, expect, it } from "vitest";
import { supportsDistanceAndPaceMetrics } from "./activityMetrics";

describe("activity metric visibility", () => {
  it.each([
    "Strength",
    "Strength Training",
    "strength_training",
    "Weight Training",
    "WeightTraining",
    "Weightlifting"
  ])("hides distance and pace for %s", (sportType) => {
    expect(supportsDistanceAndPaceMetrics(sportType)).toBe(false);
  });

  it.each(["Running", "Cycling", "Swimming", "Walking"])("keeps distance and pace for %s", (sportType) => {
    expect(supportsDistanceAndPaceMetrics(sportType)).toBe(true);
  });
});
