import { describe, expect, it } from "vitest";
import { chartDisplayDomain } from "./activityChartBounds";

describe("activity chart display bounds", () => {
  it("uses robust percentile bounds while preserving ordinary values", () => {
    const domain = chartDisplayDomain([...Array.from({ length: 100 }, (_, index) => 300 + index), 10000]);
    expect(domain?.[0]).toBeGreaterThan(300);
    expect(domain?.[1]).toBeLessThan(10000);
    expect(chartDisplayDomain([300, 600])).toEqual([300, 600]);
  });

  it("ignores invalid values and expands constant domains", () => {
    expect(chartDisplayDomain([undefined, Number.NaN, 42, 42])).toEqual([39.9, 44.1]);
    expect(chartDisplayDomain([undefined, null])).toBeUndefined();
  });
});
