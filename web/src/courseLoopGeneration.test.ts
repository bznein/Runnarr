import { describe, expect, it } from "vitest";
import { courseLoopDeviationLabel, courseLoopDistanceHint, parseCourseLoopDistanceKM } from "./courseLoopGeneration";

describe("course loop generation helpers", () => {
  it("applies sport-specific distance limits", () => {
    expect(parseCourseLoopDistanceKM("1", "Run")).toBe(1);
    expect(parseCourseLoopDistanceKM("100", "Hike")).toBe(100);
    expect(parseCourseLoopDistanceKM("0.9", "Walk")).toBeUndefined();
    expect(parseCourseLoopDistanceKM("5", "Cycling")).toBe(5);
    expect(parseCourseLoopDistanceKM("300", "Cycling")).toBe(300);
    expect(parseCourseLoopDistanceKM("301", "Cycling")).toBeUndefined();
    expect(courseLoopDistanceHint("Run")).toBe("1–100 km");
    expect(courseLoopDistanceHint("Cycling")).toBe("5–300 km");
  });

  it("formats signed deviations for comparison", () => {
    expect(courseLoopDeviationLabel(0)).toBe("On target");
    expect(courseLoopDeviationLabel(-4.25)).toBe("4.3% shorter");
    expect(courseLoopDeviationLabel(7.81)).toBe("7.8% longer");
  });
});
