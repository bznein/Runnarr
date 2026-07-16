import { describe, expect, it } from "vitest";
import { PACE_ROUTE_COLORS, clampPaceToScale, paceColorForPace, paceScaleFromPaces, paceScaleFromSpeeds, speedToPaceSPKM } from "./paceDisplay";

describe("pace display scaling", () => {
  it("converts only finite positive speeds to pace", () => {
    expect(speedToPaceSPKM(undefined)).toBeUndefined();
    expect(speedToPaceSPKM(null)).toBeUndefined();
    expect(speedToPaceSPKM(0)).toBeUndefined();
    expect(speedToPaceSPKM(-1)).toBeUndefined();
    expect(speedToPaceSPKM(Number.POSITIVE_INFINITY)).toBeUndefined();
    expect(speedToPaceSPKM(Number.NaN)).toBeUndefined();
    expect(speedToPaceSPKM(2)).toBe(500);
  });

  it("ignores invalid speeds when building a scale", () => {
    const scale = paceScaleFromSpeeds([undefined, null, 0, -1, Number.NaN, Number.POSITIVE_INFINITY, 2, 1]);

    expect(scale?.minPaceSPKM).toBeCloseTo(525);
    expect(scale?.maxPaceSPKM).toBeCloseTo(975);
  });

  it("does not flatten slow activities above ten minutes per kilometer", () => {
    const scale = paceScaleFromPaces([700, 800, 1000, 1400, 2000]);
    const plotted = [700, 800, 1000, 1400, 2000].map((pace) => clampPaceToScale(pace, scale));

    expect(scale?.minPaceSPKM).toBeGreaterThan(600);
    expect(new Set(plotted.map((pace) => Math.round(pace ?? 0))).size).toBeGreaterThan(1);
  });

  it("clamps outliers to percentile bounds", () => {
    const normalPaces = Array.from({ length: 100 }, (_, index) => 300 + index);
    const scale = paceScaleFromPaces([...normalPaces, 10000]);

    expect(scale?.maxPaceSPKM).toBeLessThan(10000);
    expect(clampPaceToScale(10000, scale)).toBe(scale?.maxPaceSPKM);
    expect(clampPaceToScale(100, scale)).toBe(scale?.minPaceSPKM);
    expect(clampPaceToScale(350, scale)).toBe(350);
  });

  it("uses the middle color for truly constant pace", () => {
    const scale = paceScaleFromPaces([500, 500, 500]);

    expect(scale).toBeDefined();
    expect(paceColorForPace(500, scale!)).toBe(PACE_ROUTE_COLORS[Math.floor(PACE_ROUTE_COLORS.length / 2)]);
  });

  it("maps fastest pace to red and slowest pace to blue", () => {
    const scale = { minPaceSPKM: 300, maxPaceSPKM: 600 };

    expect(paceColorForPace(300, scale)).toBe(PACE_ROUTE_COLORS[PACE_ROUTE_COLORS.length - 1]);
    expect(paceColorForPace(600, scale)).toBe(PACE_ROUTE_COLORS[0]);
  });
});
