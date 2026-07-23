import { describe, expect, it } from "vitest";
import { climbPerformanceFor, gapForClimbLaps, paceForClimbSamples, samplesForClimbPerformance } from "./climbPerformance";
import type { ActivityClimb, ActivityLap, ActivitySample } from "./types";

const climb: ActivityClimb = {
  index: 0,
  difficulty: "Moderate",
  startSampleIndex: 0,
  endSampleIndex: 2,
  startDistanceM: 0,
  endDistanceM: 1000,
  distanceM: 1000,
  elevationGainM: 30,
  avgGradePct: 3,
  startElevationM: 100,
  endElevationM: 130
};

function samples(): ActivitySample[] {
  return [
    { index: 0, elapsedS: 0, distanceM: 0, speedMPS: 3.333333 },
    { index: 1, elapsedS: 150, distanceM: 500, speedMPS: 3.333333 },
    { index: 2, elapsedS: 300, distanceM: 1000, speedMPS: 3.333333 }
  ];
}

describe("climb performance", () => {
  it("computes climb pace from moving sample distance and time", () => {
    expect(paceForClimbSamples(samples(), climb)).toBeCloseTo(300);
  });

  it("uses bounded series samples when the activity detail omits raw samples", () => {
    const availableSamples = samplesForClimbPerformance(undefined, samples());
    expect(paceForClimbSamples(availableSamples, climb)).toBeCloseTo(300);
  });

  it("weights GAP by the lap distance that overlaps the climb", () => {
    const laps: ActivityLap[] = [
      { index: 0, elapsedTimeS: 300, movingTimeS: 300, distanceM: 500, avgGradeAdjustedPaceSPKM: 320 },
      { index: 1, elapsedTimeS: 300, movingTimeS: 300, distanceM: 500, avgGradeAdjustedPaceSPKM: 280 }
    ];
    expect(gapForClimbLaps(laps, climb)).toBeCloseTo(300);
  });

  it("omits performance when the source has no usable values", () => {
    const result = climbPerformanceFor([{ index: 0, distanceM: 0 }, { index: 2, distanceM: 1000 }], [], climb);
    expect(result.paceSPKM).toBeUndefined();
    expect(result.gapSPKM).toBeUndefined();
  });
});
