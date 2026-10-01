import { describe, expect, it } from "vitest";
import {
  checklistDueDate,
  eventToday,
  formatRaceTime,
  newRace,
  parseRaceTime,
  raceInput,
} from "./utils";
import type { Race } from "./types";

describe("race time and calendar boundaries", () => {
  it("preserves fractions and long ultra durations", () => {
    for (const [text, value] of [
      ["39:12.345", 2352345],
      ["27:01:02", 97262000],
      ["59.5", 59500],
    ] as const) {
      expect(parseRaceTime(text)).toBe(value);
      expect(parseRaceTime(formatRaceTime(value))).toBe(value);
    }
    expect(formatRaceTime(-500)).toBe("−0:00.5");
    expect(formatRaceTime(undefined)).toBe("");
  });
  it("rejects malformed clocks without silently rounding", () => {
    for (const text of [
      "0",
      "-1",
      "1:60",
      "1:99:12",
      "1.2345",
      "NaN",
      "1:2:3:4",
    ])
      expect(() => parseRaceTime(text)).toThrow();
    expect(parseRaceTime("  ")).toBeUndefined();
  });
  it("uses event-local dates across timezone boundaries", () => {
    const now = new Date("2026-01-01T00:30:00Z");
    expect(eventToday("America/New_York", now)).toBe("2025-12-31");
    expect(eventToday("Asia/Tokyo", now)).toBe("2026-01-01");
  });
  it("keeps checklist offsets on calendar dates across DST", () => {
    expect(
      checklistDueDate(
        { label: "Pack", done: false, daysBefore: 1 },
        "2026-03-30",
      ),
    ).toBe("2026-03-29");
    expect(
      checklistDueDate({ label: "Pack", done: false, daysBefore: 1 }),
    ).toBe("");
  });
  it("does not assume watch values are official results", () => {
    const race = newRace();
    expect(race.result.confirmed).toBe(false);
    expect(race.distanceM).toBeUndefined();
    const saved = {
      ...race,
      id: "a",
      createdAt: "today",
      updatedAt: "today",
      goals: [{ name: "Finish" }],
    } as Race;
    const input = raceInput(saved);
    input.goals[0].name = "Different";
    expect(saved.goals[0].name).toBe("Finish");
    expect(input).not.toHaveProperty("id");
  });
});
