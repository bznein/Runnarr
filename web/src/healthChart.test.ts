import { describe, expect, it } from "vitest";
import {
  formatHealthAxisBPM,
  formatHealthAxisHours,
  formatHealthAxisInteger,
  formatHealthAxisMS,
  HEALTH_CHART_Y_AXIS_WIDTH
} from "./healthChart";

describe("health chart axes", () => {
  it("reserves enough width for grouped health values", () => {
    expect(HEALTH_CHART_Y_AXIS_WIDTH).toBeGreaterThanOrEqual(60);
    expect(formatHealthAxisInteger(12450)).toBe((12450).toLocaleString());
  });

  it("includes units for health metrics that need them", () => {
    expect(formatHealthAxisHours(7.5)).toBe("7.5 h");
    expect(formatHealthAxisBPM(58.4)).toBe("58 bpm");
    expect(formatHealthAxisMS(42.6)).toBe("43 ms");
  });

  it("omits invalid axis values", () => {
    expect(formatHealthAxisInteger(undefined)).toBe("");
    expect(formatHealthAxisHours(Number.NaN)).toBe("");
    expect(formatHealthAxisBPM(Number.POSITIVE_INFINITY)).toBe("");
    expect(formatHealthAxisMS(-Number.POSITIVE_INFINITY)).toBe("");
  });
});
