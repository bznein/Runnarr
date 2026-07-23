import { describe, expect, it } from "vitest";
import { reconcileVisibleActivitySeries } from "./activityChartSeries";

describe("activity chart series selection", () => {
  it("falls back when the selected series is no longer available", () => {
    expect(reconcileVisibleActivitySeries(["power"], ["elevationM", "heartRate"], ["elevationM"])).toEqual(["elevationM"]);
  });

  it("keeps a valid user selection when data changes", () => {
    expect(reconcileVisibleActivitySeries(["heartRate", "power"], ["heartRate", "elevationM"], ["elevationM"])).toEqual(["heartRate"]);
  });
});
