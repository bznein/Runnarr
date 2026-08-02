import { describe, expect, it } from "vitest";
import { chartDisplayDomain } from "./activityChartBounds";

describe("activity chart display bounds", () => {
  it("uses the complete processed trend range with five-percent padding", () => {
    const domain = chartDisplayDomain([...Array.from({ length: 100 }, (_, index) => 300 + index), 10000]);
    expect(domain).toEqual([-185, 10485]);
    expect(chartDisplayDomain([300, 600])).toEqual([285, 615]);
  });

  it("ignores invalid values and expands constant domains", () => {
    expect(chartDisplayDomain([undefined, Number.NaN, 42, 42])).toEqual([39.9, 44.1]);
    expect(chartDisplayDomain([undefined, null])).toBeUndefined();
  });
});
