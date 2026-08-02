import { describe, expect, it } from "vitest";
import { formatActivityChartTooltipValue, recordedActivityChartKey } from "./activityChartTooltip";

describe("activity chart tooltips", () => {
  it("maps every plotted trend to its recorded value", () => {
    expect(recordedActivityChartKey("elevationM")).toBe("rawElevationM");
    expect(recordedActivityChartKey("heartRate")).toBe("rawHeartRate");
    expect(recordedActivityChartKey("paceSPKM")).toBe("rawPaceSPKM");
    expect(recordedActivityChartKey("power")).toBe("rawPower");
    expect(recordedActivityChartKey("cadence")).toBe("rawCadence");
  });

  it("shows recorded and trend values only when their display values differ", () => {
    const format = (value: number) => `${Math.round(value)} bpm`;
    expect(formatActivityChartTooltipValue(151, 184, format)).toBe("184 bpm recorded · 151 bpm trend");
    expect(formatActivityChartTooltipValue(151.2, 151.4, format)).toBe("151 bpm");
    expect(formatActivityChartTooltipValue(151, undefined, format)).toBe("151 bpm");
  });
});
