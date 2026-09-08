import { describe, expect, it } from "vitest";
import { buildWorkoutSummary, summaryDuration, summaryPace, workoutGoalScore } from "./workoutSummary";
import type { ActivityInterval, ActivityWorkout, Workout, WorkoutStep } from "./types";

const work = (overrides: Partial<WorkoutStep> = {}): WorkoutStep => ({ order: 1, kind: "work", endCondition: { type: "time", value: 300 }, target: { type: "pace", paceSecondsPerKM: 240 }, ...overrides });
const workout = (steps = [work()], overrides: Partial<Workout> = {}): Workout => ({
  id: "local", source: "manual", name: "Matched efforts", sourceText: "5mins@4:00", sourceHash: "", sportType: "Run",
  definition: { version: 1, sportType: "Run", steps, estimatedDurationS: 300 }, parseStatus: "ready", parseMessages: [],
  garminExcluded: false, revision: 1, garmin: {}, generatedAt: "", createdAt: "", updatedAt: "", ...overrides
});
const interval = (overrides: Partial<ActivityInterval> = {}): ActivityInterval => ({ index: 0, category: "active", elapsedTimeS: 300, movingTimeS: 300, distanceM: 1250, avgPaceSPKM: 240, ...overrides });
const imported = (overrides: Partial<ActivityWorkout> = {}): ActivityWorkout => ({ provider: "garmin", name: "Watch workout", steps: [{ index: 1, order: 1, type: "interval", endCondition: "time", endConditionValue: 300, targetType: "pace.zone", targetValueOne: 4, targetValueTwo: 1000 / 230 }], ...overrides });

describe("workout summary goals and scoring", () => {
  it("prefers the matched prescription, falling back only for unusable plans", () => {
    const activity = { workout: imported(), intervals: [interval()] };
    expect(buildWorkoutSummary(activity, workout(), 0)).toMatchObject({ source: "matched", name: "Matched efforts", score: 100 });
    expect(buildWorkoutSummary(activity, workout([], { parseStatus: "error" }), 0)).toMatchObject({ source: "imported", name: "Watch workout", score: 100 });
    expect(buildWorkoutSummary({}, workout([], { parseStatus: "error" }), 0)).toMatchObject({ source: "matched", prescription: "5mins@4:00", score: undefined });
    expect(buildWorkoutSummary({})).toBeUndefined();
  });

  it("gives partial credit, inclusive bounds, and zero for large deviations", () => {
    expect(workoutGoalScore(270, 300)).toBeCloseTo(90);
    expect(workoutGoalScore(330, 300)).toBeCloseTo(90);
    expect(workoutGoalScore(230, 230, 250)).toBe(100);
    expect(workoutGoalScore(250, 230, 250)).toBe(100);
    expect(workoutGoalScore(275, 230, 250)).toBeCloseTo(90);
    expect(workoutGoalScore(900, 300)).toBe(0);
    expect(workoutGoalScore(NaN, 300)).toBeUndefined();
    expect(workoutGoalScore(300, 0)).toBeUndefined();
  });

  it("averages goals within steps and work steps equally, excluding warmups", () => {
    const plan = workout([work({ kind: "warmup", target: { type: "none" } }), work(), work({ order: 2, endCondition: { type: "distance", value: 1000 } })]);
    const result = buildWorkoutSummary({ intervals: [interval({ category: "warmup", movingTimeS: 30 }), interval({ index: 1, movingTimeS: 270, avgPaceSPKM: 264 }), interval({ index: 2, distanceM: 1000 })] }, plan, 0)!;
    expect(result.rows.map((row) => Math.round(row.score!))).toEqual([10, 90, 100]);
    expect(result.score).toBeCloseTo(95);
    expect(result).toMatchObject({ workSteps: 2, scoredSteps: 2 });
  });

  it("uses effective exact-pace tolerance but does not widen explicit ranges", () => {
    const activity = { intervals: [interval({ avgPaceSPKM: 250 })] };
    expect(buildWorkoutSummary(activity, workout(), 10)?.score).toBe(100);
    expect(buildWorkoutSummary(activity, workout(undefined, { paceToleranceSeconds: 0 }), 10)?.score).toBeLessThan(100);
    expect(buildWorkoutSummary(activity, workout(), undefined)?.score).toBeUndefined();
    const plan = workout([work({ target: { type: "pace", paceFastSecondsPerKM: 235, paceSlowSecondsPerKM: 245 } })]);
    expect(buildWorkoutSummary(activity, plan, 60)?.score).toBeLessThan(100);
  });

  it("preserves displayed provider pace and raw duration, deriving only missing pace", () => {
    const actual = interval({ movingTimeS: 290, avgPaceSPKM: 250, raw: { duration: "300.5" } });
    expect(summaryDuration(actual)).toBe(300.5);
    expect(summaryPace(actual)).toBe(250);
    expect(summaryPace(interval({ avgPaceSPKM: undefined, distanceM: 1000 }))).toBe(300);
    expect(summaryPace(interval({ avgPaceSPKM: NaN, distanceM: 0 }))).toBeUndefined();
    expect(summaryDuration(interval({ raw: { duration: " " } }))).toBe(300);
  });

  it("supports absolute HR and power, but excludes unresolved zones and missing goal metrics", () => {
    for (const [targetType, targetValueUnit, metric] of [["heart.rate.zone", "bpm", "avgHeartRate"], ["power.zone", "watts", "avgPower"]] as const) {
      const definition = imported({ steps: [{ index: 1, order: 1, type: "interval", endCondition: "time", endConditionValue: 300, targetType, targetValueUnit, targetValueOne: 150, targetValueTwo: 170 }] });
      expect(buildWorkoutSummary({ workout: definition, intervals: [interval({ [metric]: 160 })] })?.score).toBe(100);
      expect(buildWorkoutSummary({ workout: definition, intervals: [interval()] })?.score).toBeUndefined();
      definition.steps![0].zoneNumber = 3;
      const result = buildWorkoutSummary({ workout: definition, intervals: [interval({ [metric]: 160 })] })!;
      expect(result.score).toBeUndefined();
      expect(result.rows[0].step?.target).toContain("zone 3");
    }
  });

  it("keeps open steps and incomplete results unscored rather than averaging available goals", () => {
    expect(buildWorkoutSummary({ intervals: [interval()] }, workout([work({ endCondition: { type: "lap_button" }, target: { type: "none" } })]), 0)?.score).toBeUndefined();
    const result = buildWorkoutSummary({ intervals: [interval({ avgPaceSPKM: undefined, distanceM: 0 })] }, workout(), 0)!;
    expect(result.rows[0].status).toBe("Missing goal data");
    expect(result.scoredSteps).toBe(0);
  });
});

describe("workout summary alignment", () => {
  it("skips only the trailing recovery in the last set", () => {
    const plan = workout([{ order: 1, kind: "repeat", target: { type: "none" }, repeatCount: 2, skipLastRecovery: true, children: [work(), work({ kind: "recovery" }), work(), work({ kind: "recovery" })] }]);
    const result = buildWorkoutSummary({}, plan, 0)!;
    expect(result.rows).toHaveLength(7);
    expect(result.rows.filter((row) => row.step?.category === "recovery")).toHaveLength(3);
  });

  it("expands nested sets and skips final recovery without losing leaf identity", () => {
    const plan = workout([{ order: 1, kind: "repeat", target: { type: "none" }, repeatCount: 2, children: [{ order: 2, kind: "repeat", target: { type: "none" }, repeatCount: 2, skipLastRecovery: true, children: [work({ order: 3 }), work({ order: 4, kind: "recovery" })] }] }]);
    const result = buildWorkoutSummary({}, plan, 0)!;
    expect(result.rows.map((row) => row.step?.label)).toEqual(["Set 1.1 · Work", "Set 1.1 · Recovery", "Set 1.2 · Work", "Set 2.1 · Work", "Set 2.1 · Recovery", "Set 2.2 · Work"]);
    expect(result.workSteps).toBe(4);
    expect(new Set(result.rows.filter((row) => row.step?.category === "active").map((row) => row.step?.key)).size).toBe(1);
  });

  it("uses repeat identifiers to retain a partial score without shifting later reps", () => {
    const plan = workout([{ order: 1, kind: "repeat", target: { type: "none" }, repeatCount: 3, children: [work()] }]);
    const result = buildWorkoutSummary({ intervals: [interval({ workoutRepeatIndex: 1 }), interval({ index: 1, workoutRepeatIndex: 3, avgPaceSPKM: 264 })] }, plan, 0)!;
    expect(result.rows.map((row) => row.actual?.index)).toEqual([0, undefined, 1]);
    expect(result).toMatchObject({ workSteps: 3, scoredSteps: 2 });
    expect(result.score).toBeCloseTo(97.5);
  });

  it("does not guess which repeated effort is missing or which additional effort belongs", () => {
    const plan = workout([work(), work({ order: 2 })]);
    const result = buildWorkoutSummary({ intervals: [interval()] }, plan, 0)!;
    expect(result.scoredSteps).toBe(0);
    expect(result.rows.map((row) => row.status)).toEqual(["No reliable result", "No reliable result", "Additional recorded interval"]);
    expect(buildWorkoutSummary({ intervals: [interval(), interval({ index: 1 }), interval({ index: 2 })] }, plan, 0)?.scoredSteps).toBe(0);
  });

  it("retains unique category matches around ambiguous efforts and additional intervals", () => {
    const plan = workout([work({ kind: "warmup" }), work(), work({ order: 2 }), work({ kind: "cooldown" })]);
    const result = buildWorkoutSummary({ intervals: [interval({ category: "warmup" }), interval({ index: 1 }), interval({ index: 2, category: "cooldown" }), interval({ index: 3, category: "other" })] }, plan, 0)!;
    expect(result.rows.slice(0, 4).map((row) => row.actual?.index)).toEqual([0, undefined, undefined, 2]);
    expect(result.rows.filter((row) => !row.step).map((row) => row.actual?.index)).toEqual([1, 3]);
  });

  it("resolves imported provider references without assuming synthetic array indexes", () => {
    const definition = imported({ steps: [{ index: 201, order: 4, type: "interval", endCondition: "time", endConditionValue: 300 }, { index: 202, order: 8, type: "interval", endCondition: "time", endConditionValue: 600 }] });
    const result = buildWorkoutSummary({ workout: definition, intervals: [interval({ workoutStepIndex: 8, movingTimeS: 600 })] })!;
    expect(result.rows.map((row) => row.actual?.index)).toEqual([undefined, 0]);
    expect(result.score).toBe(100);
  });

  it("uses identified workout laps and aggregates multiple laps for one effort", () => {
    const definition = imported();
    const laps = [interval({ movingTimeS: 150, elapsedTimeS: 150, distanceM: 625, workoutStepIndex: 1 }), interval({ index: 1, movingTimeS: 150, elapsedTimeS: 150, distanceM: 625, workoutStepIndex: 1 })];
    const result = buildWorkoutSummary({ workout: definition, laps })!;
    expect(result.score).toBe(100);
    expect(result.rows[0].actual?.distanceM).toBe(1250);
    expect(summaryDuration(result.rows[0].actual!)).toBe(300);
    expect(buildWorkoutSummary({ workout: definition, laps: laps.map((lap) => ({ ...lap, workoutStepIndex: undefined, intensityType: "active" })) })?.score).toBeUndefined();
  });

  it("prefers intervals over laps and never double-counts actuals", () => {
    const result = buildWorkoutSummary({ workout: imported(), intervals: [interval()], laps: [interval({ workoutStepIndex: 1 })] })!;
    expect(result.rows).toHaveLength(1);
    expect(result.scoredSteps).toBe(1);
  });

  it("bounds malformed or huge repeats without producing an overall score", () => {
    const plan = workout([{ order: 1, kind: "repeat", target: { type: "none" }, repeatCount: 100000, children: [work()] }]);
    const result = buildWorkoutSummary({ intervals: [interval()] }, plan, 0)!;
    expect(result.rows.length).toBeLessThanOrEqual(501);
    expect(result.warnings.join(" ")).toContain("too large");
    expect(result.score).toBeUndefined();
    expect(buildWorkoutSummary({}, workout([{ order: 1, kind: "repeat", target: { type: "none" }, repeatCount: 3, children: [] }]), 0)?.score).toBeUndefined();
  });
});
