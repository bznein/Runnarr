import { describe, expect, it } from "vitest";
import { formatActivityForAI } from "./activityAI";
import type { Activity } from "./types";

function activity(overrides: Partial<Activity> = {}): Activity {
  return {
    id: "activity-secret-id",
    source: "garmin",
    sourceId: "provider-secret-id",
    name: "Morning Run",
    sourceName: "Provider Morning Run",
    sportType: "Run",
    startTime: "2026-08-20T06:30:00Z",
    distanceM: 10000,
    movingTimeS: 3000,
    elapsedTimeS: 3035,
    elevationGainM: 125,
    createdAt: "2026-08-20T07:30:00Z",
    ...overrides
  };
}

describe("formatActivityForAI", () => {
  it("formats a complete activity with structured intervals and climbs", () => {
    const result = formatActivityForAI(activity({
      avgPaceSPKM: 300,
      avgGradeAdjustedPaceSPKM: 295,
      avgHeartRate: 151,
      maxHeartRate: 178,
      caloriesKcal: 720,
      rpe: 7,
      notes: "Felt controlled.",
      feedback: "Strong finish.",
      summaryPolyline: "private-polyline",
      samples: [{ index: 0, latitude: 53.34, longitude: -6.26 }],
      gear: [{ id: "gear-secret-id", providerGearId: "provider-gear-secret", name: "Daily Trainers", retired: false }],
      workout: { provider: "garmin", providerWorkoutId: "workout-secret-id", name: "Threshold Repeats", sportType: "Run" },
      intervals: [{
        index: 0,
        category: "active",
        elapsedTimeS: 600,
        movingTimeS: 600,
        distanceM: 2000,
        avgPaceSPKM: 285,
        avgHeartRate: 165,
        maxHeartRate: 178,
        elevationGainM: 18,
        lapIndexes: [1, 2],
        raw: { latitude: 53.34, providerToken: "secret" }
      }],
      climbs: [{
        index: 0,
        difficulty: "moderate",
        startSampleIndex: 5,
        endSampleIndex: 20,
        startDistanceM: 1000,
        endDistanceM: 1800,
        distanceM: 800,
        elevationGainM: 42,
        avgGradePct: 5.25,
        startElevationM: 20,
        endElevationM: 62,
        paceSPKM: 320,
        gapSPKM: 295
      }]
    }));

    expect(result).toContain("# Activity: Morning Run");
    expect(result).toContain("- Distance: 10.00 km");
    expect(result).toContain("- Moving time: 50:00");
    expect(result).toContain("- RPE: 7/10");
    expect(result).toContain("## Notes\n\n> Felt controlled.");
    expect(result).toContain("## Workout\n- Name: Threshold Repeats");
    expect(result).toContain("## Intervals");
    expect(result).toContain("| 1 | Active | 2, 3 | 10:00 | 2.00 km | 4:45 /km | 165 bpm | 178 bpm | 18 m |");
    expect(result).toContain("## Climbs");
    expect(result).not.toMatch(/activity-secret-id|provider-secret-id|private-polyline|providerToken|latitude|longitude|workout-secret-id|gear-secret-id/);
  });

  it("uses laps when structured intervals are absent and omits unavailable columns", () => {
    const result = formatActivityForAI(activity({
      distanceM: 400,
      movingTimeS: 0,
      elapsedTimeS: 120,
      elevationGainM: 0,
      laps: [
        { index: 0, elapsedTimeS: 60, movingTimeS: 0, distanceM: 200 },
        { index: 1, elapsedTimeS: 60, movingTimeS: 60, distanceM: 200 }
      ]
    }));

    expect(result).toContain("- Distance: 400 m");
    expect(result).toContain("## Laps");
    expect(result).toContain("| Lap | Time | Distance |");
    expect(result).toContain("| 1 | 1:00 | 200 m |");
    expect(result).not.toContain("Avg HR");
    expect(result).not.toContain("## Intervals");
  });

  it("keeps multiline notes bounded and escapes Markdown table cells", () => {
    const result = formatActivityForAI(activity({
      notes: "First line\n\n# Not a section",
      intervals: [{
        index: 0,
        category: "run|fast",
        elapsedTimeS: 60,
        movingTimeS: 60,
        distanceM: 250
      }]
    }));

    expect(result).toContain("> First line\n>\n> # Not a section");
    expect(result).toContain("Run\\|fast");
  });
});
