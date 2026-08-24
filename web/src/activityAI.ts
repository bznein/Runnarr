import { speedToPaceSPKM } from "./paceDisplay";
import type { Activity, ActivityAIContext, ActivityClimb, ActivityInterval, ActivityLap, ActivitySample, ActivityWorkoutStep } from "./types";

type MarkdownColumn<T> = {
  heading: string;
  value: (item: T) => string | undefined;
  always?: boolean;
};

type IntervalExportRow = {
  step: string;
  target?: string;
  laps: string;
  timeS: number;
  cumulativeS: number;
  distanceM: number;
  avgPaceSPKM?: number;
  avgGradeAdjustedPaceSPKM?: number;
  avgHeartRate?: number;
  maxHeartRate?: number;
  elevationGainM?: number;
  elevationLossM?: number;
  avgRunCadence?: number;
  avgGroundContactTimeMS?: number;
  avgPower?: number;
};

export function formatActivityForAI(activity: Activity, context?: ActivityAIContext) {
  const lines = [`# Activity: ${inlineText(activity.name)}`, "", "## Overview"];
  appendFields(lines, [
    ["Sport", activity.sportType],
    ["Started", activity.startTime],
    ["Distance", formatDistance(activity.distanceM)],
    ["Moving time", formatDuration(activity.movingTimeS)],
    ["Elapsed time", formatDuration(activity.elapsedTimeS)],
    ["Average pace", formatPace(activity.avgPaceSPKM)],
    ["Grade-adjusted pace", formatPace(activity.avgGradeAdjustedPaceSPKM)],
    ["Elevation gain", formatMeters(activity.elevationGainM)],
    ["Average heart rate", formatBPM(activity.avgHeartRate)],
    ["Maximum heart rate", formatBPM(activity.maxHeartRate)],
    ["Calories", formatInteger(activity.caloriesKcal, "kcal")],
    ["RPE", activity.rpe === undefined ? undefined : `${activity.rpe}/10`],
    ["Gear", formatGear(activity)],
    ["Matched plan", activity.trainingSheetMatch?.plannedActivityName]
  ]);

  if (context) {
    lines.push("", "## Weekly running context");
    appendFields(lines, [
      ["Activity day", formatContextWeekday(context.activityDate)],
      ["Window", `${formatContextDate(context.windowStart)} to ${formatContextDate(context.windowEnd)}`],
      ["Other runs", String(context.totals.runCount)],
      ["Distance", formatDistance(context.totals.distanceM)],
      ["Moving time", formatDuration(context.totals.movingTimeS)],
      ["Elevation gain", formatMeters(context.totals.elevationGainM)],
      ["Average pace", formatPace(context.totals.avgPaceSPKM)]
    ]);
    if (context.runs.length > 0) {
      lines.push("", ...weeklyContextTable(context));
    } else {
      lines.push("", "No other runs were recorded in this window.");
    }
  }

  if (activity.notes?.trim()) {
    lines.push("", "## Notes", "", ...quoteText(activity.notes));
  }
  if (activity.feedback?.trim()) {
    lines.push("", "## Reflection", "", ...quoteText(activity.feedback));
  }
  if (activity.workout && (activity.workout.name?.trim() || activity.workout.sportType?.trim())) {
    lines.push("", "## Workout");
    appendFields(lines, [
      ["Name", activity.workout.name],
      ["Sport", activity.workout.sportType]
    ]);
  }

  if ((activity.intervals?.length ?? 0) > 0) {
    lines.push("", "## Intervals", "", ...intervalTable(activity));
  } else if ((activity.laps?.length ?? 0) > 0) {
    lines.push("", "## Laps", "", ...lapTable(activity));
  }

  if ((activity.climbs?.length ?? 0) > 0) {
    lines.push("", "## Climbs", "", ...climbTable(activity.climbs!));
  }

  return `${lines.join("\n")}\n`;
}

function weeklyContextTable(context: ActivityAIContext) {
  return markdownTable(context.runs, [
    { heading: "Day", value: (item) => formatContextRunDate(item.date), always: true },
    { heading: "Activity", value: (item) => item.name, always: true },
    { heading: "Distance", value: (item) => formatDistance(item.distanceM), always: true },
    { heading: "Moving time", value: (item) => formatDuration(item.movingTimeS), always: true },
    { heading: "Avg pace", value: (item) => formatPace(item.avgPaceSPKM), always: true },
    { heading: "Avg HR", value: (item) => formatBPM(item.avgHeartRate) }
  ]);
}

function formatContextWeekday(value: string) {
  return contextDate(value).toLocaleDateString("en-GB", { weekday: "long", timeZone: "UTC" });
}

function formatContextDate(value: string) {
  return contextDate(value).toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric", timeZone: "UTC" });
}

function formatContextRunDate(value: string) {
  return contextDate(value).toLocaleDateString("en-GB", { weekday: "short", day: "numeric", month: "short", timeZone: "UTC" });
}

function contextDate(value: string) {
  return new Date(`${value}T12:00:00Z`);
}

function appendFields(lines: string[], fields: Array<[string, string | undefined]>) {
  for (const [label, value] of fields) {
    const cleaned = value?.trim();
    if (cleaned) lines.push(`- ${label}: ${inlineText(cleaned)}`);
  }
}

function intervalTable(activity: Activity) {
  const intervals = activity.intervals ?? [];
  const laps = activity.laps ?? [];
  const samples = activity.samples ?? [];
  const lapsByIndex = new Map(laps.map((lap) => [lap.index, lap]));
  const rows = intervals.flatMap<IntervalExportRow>((interval) => {
    const lapIndexes = intervalLapIndexesForDisplay(interval, intervals, laps);
    const intervalRows: IntervalExportRow[] = [{
      step: intervalStepLabel(interval, activity.sportType),
      target: intervalTargetLabel(activity.workout, interval),
      laps: formatLapRange(lapIndexes),
      timeS: intervalDisplayTimeS(interval),
      cumulativeS: intervalCumulativeTime(interval, intervals),
      distanceM: interval.distanceM,
      avgPaceSPKM: interval.avgPaceSPKM,
      avgGradeAdjustedPaceSPKM: interval.avgGradeAdjustedPaceSPKM,
      avgHeartRate: interval.avgHeartRate,
      maxHeartRate: interval.maxHeartRate,
      elevationGainM: interval.elevationGainM,
      elevationLossM: interval.elevationLossM,
      avgRunCadence: interval.avgRunCadence,
      avgGroundContactTimeMS: interval.avgGroundContactTimeMS,
      avgPower: interval.avgPower
    }];
    for (const lapIndex of lapIndexes) {
      const lap = lapsByIndex.get(lapIndex);
      if (!lap) continue;
      intervalRows.push({
        step: `Lap ${lap.index + 1}`,
        laps: String(lap.index + 1),
        timeS: lapDisplayTimeS(lap, samples),
        cumulativeS: lapCumulativeTime(lap, laps, samples),
        distanceM: lap.distanceM,
        avgPaceSPKM: lapPaceSPKM(lap, samples),
        avgGradeAdjustedPaceSPKM: lap.avgGradeAdjustedPaceSPKM,
        avgHeartRate: lap.avgHeartRate,
        maxHeartRate: lap.maxHeartRate,
        elevationGainM: lap.elevationGainM,
        elevationLossM: lap.elevationLossM,
        avgRunCadence: lap.avgRunCadence,
        avgGroundContactTimeMS: lap.avgGroundContactTimeMS,
        avgPower: lap.avgPower
      });
    }
    return intervalRows;
  });

  return markdownTable(rows, [
    { heading: "Step", value: (item) => item.step, always: true },
    { heading: "Target", value: (item) => item.target },
    { heading: "Laps", value: (item) => item.laps, always: true },
    { heading: "Time", value: (item) => formatDuration(item.timeS), always: true },
    { heading: "Cumulative", value: (item) => formatDuration(item.cumulativeS), always: true },
    { heading: "Distance", value: (item) => formatDistance(item.distanceM), always: true },
    { heading: "Avg pace", value: (item) => formatPace(item.avgPaceSPKM), always: true },
    { heading: "Avg GAP", value: (item) => formatPace(item.avgGradeAdjustedPaceSPKM) },
    { heading: "Avg HR", value: (item) => formatBPM(item.avgHeartRate) },
    { heading: "Max HR", value: (item) => formatBPM(item.maxHeartRate) },
    { heading: "Gain", value: (item) => formatMeters(item.elevationGainM) },
    { heading: "Loss", value: (item) => formatMeters(item.elevationLossM) },
    { heading: "Avg cadence", value: (item) => formatInteger(item.avgRunCadence, "spm") },
    { heading: "Avg GCT", value: (item) => formatInteger(item.avgGroundContactTimeMS, "ms") },
    { heading: "Avg power", value: (item) => formatInteger(item.avgPower, "W") }
  ]);
}

function lapTable(activity: Activity) {
  const laps = activity.laps ?? [];
  const samples = activity.samples ?? [];
  return markdownTable(laps, [
    { heading: "Lap", value: (item) => String(item.index + 1), always: true },
    { heading: "Distance", value: (item) => formatDistance(item.distanceM), always: true },
    { heading: "Time", value: (item) => formatDuration(lapDisplayTimeS(item, samples)), always: true },
    { heading: "Pace", value: (item) => formatPace(lapPaceSPKM(item, samples)), always: true },
    { heading: "GAP", value: (item) => formatPace(item.avgGradeAdjustedPaceSPKM) },
    { heading: "Gain", value: (item) => formatMeters(item.elevationGainM) },
    { heading: "Loss", value: (item) => formatMeters(item.elevationLossM) }
  ]);
}

function climbTable(climbs: ActivityClimb[]) {
  return markdownTable(climbs, [
    { heading: "Climb", value: (item) => String(item.index + 1), always: true },
    { heading: "Difficulty", value: (item) => item.difficulty ? humanize(item.difficulty) : undefined },
    { heading: "Distance", value: (item) => formatDistance(item.distanceM), always: true },
    { heading: "Gain", value: (item) => formatMeters(item.elevationGainM), always: true },
    { heading: "Avg grade", value: (item) => formatDecimal(item.avgGradePct, "%"), always: true },
    { heading: "Pace", value: (item) => formatPace(item.paceSPKM) },
    { heading: "GAP", value: (item) => formatPace(item.gapSPKM) }
  ]);
}

function markdownTable<T>(items: T[], columns: MarkdownColumn<T>[]) {
  const visible = columns.filter((column) => column.always || items.some((item) => Boolean(column.value(item))));
  return [
    `| ${visible.map((column) => escapeTableCell(column.heading)).join(" | ")} |`,
    `| ${visible.map(() => "---").join(" | ")} |`,
    ...items.map((item) => `| ${visible.map((column) => escapeTableCell(column.value(item) ?? "")).join(" | ")} |`)
  ];
}

function formatDistance(value?: number) {
  if (!isFiniteNonNegative(value)) return undefined;
  if (value < 1000) return `${Math.round(value)} m`;
  return `${(value / 1000).toFixed(2)} km`;
}

function formatDuration(value?: number) {
  if (!isFiniteNonNegative(value)) return undefined;
  const seconds = Math.round(value);
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remainder = seconds % 60;
  return hours > 0
    ? `${hours}:${String(minutes).padStart(2, "0")}:${String(remainder).padStart(2, "0")}`
    : `${minutes}:${String(remainder).padStart(2, "0")}`;
}

function formatPace(value?: number) {
  if (!isFinitePositive(value)) return undefined;
  return `${formatDuration(value)} /km`;
}

function formatMeters(value?: number) {
  return formatInteger(value, "m");
}

function formatBPM(value?: number) {
  return formatInteger(value, "bpm");
}

function formatInteger(value: number | undefined, unit: string) {
  if (!isFiniteNonNegative(value)) return undefined;
  return `${Math.round(value)} ${unit}`;
}

function formatDecimal(value: number | undefined, unit: string) {
  if (value === undefined || !Number.isFinite(value)) return undefined;
  return `${value.toFixed(1).replace(/\.0$/, "")} ${unit}`;
}

function formatGear(activity: Activity) {
  const names = (activity.gear ?? []).map((item) => item.name.trim()).filter(Boolean);
  return names.length > 0 ? names.join(", ") : undefined;
}

function humanize(value: string) {
  const normalized = value.trim().replace(/[_-]+/g, " ").replace(/([a-z])([A-Z])/g, "$1 $2");
  return normalized ? normalized.charAt(0).toUpperCase() + normalized.slice(1) : "";
}

function inlineText(value: string) {
  return value.replace(/\s+/g, " ").trim();
}

function escapeTableCell(value: string) {
  return inlineText(value).replace(/\\/g, "\\\\").replace(/\|/g, "\\|");
}

function quoteText(value: string) {
  return value.trim().split(/\r?\n/).map((line) => line ? `> ${line}` : ">");
}

function isFiniteNonNegative(value: number | undefined): value is number {
  return value !== undefined && Number.isFinite(value) && value >= 0;
}

function isFinitePositive(value: number | undefined): value is number {
  return value !== undefined && Number.isFinite(value) && value > 0;
}

function intervalCategoryLabel(category: string, sportType: string) {
  switch (category.toLowerCase()) {
    case "warmup": return "Warm Up";
    case "active": return /run|walk|hike/i.test(sportType) ? "Run" : "Active";
    case "recovery": return "Recovery";
    case "cooldown": return "Cool Down";
    default: return category.replace(/(^|[-_])([a-z])/g, (_, prefix: string, letter: string) => `${prefix ? " " : ""}${letter.toUpperCase()}`);
  }
}

function intervalStepLabel(interval: ActivityInterval, sportType: string) {
  const category = intervalCategoryLabel(interval.category, sportType);
  if (interval.workoutRepeatIndex !== undefined && (interval.category === "active" || interval.category === "recovery")) {
    return `${interval.workoutRepeatIndex}. ${category}`;
  }
  return category;
}

function formatLapRange(lapIndexes: number[]) {
  if (lapIndexes.length === 0) return "";
  const first = lapIndexes[0] + 1;
  const last = lapIndexes[lapIndexes.length - 1] + 1;
  return first === last ? String(first) : `${first}–${last}`;
}

function intervalLapIndexesForDisplay(interval: ActivityInterval, intervals: ActivityInterval[], laps: ActivityLap[]) {
  if (interval.lapIndexes?.length) return interval.lapIndexes;
  return intervals.length === 1 ? laps.map((lap) => lap.index) : [];
}

function intervalCumulativeTime(interval: ActivityInterval, intervals: ActivityInterval[]) {
  const index = intervals.findIndex((candidate) => candidate.index === interval.index);
  return Math.round(intervals.slice(0, index + 1).reduce((total, candidate) => total + intervalDisplayTimeS(candidate), 0));
}

function lapCumulativeTime(lap: ActivityLap, laps: ActivityLap[], samples: ActivitySample[]) {
  return Math.round(laps
    .filter((candidate) => candidate.index <= lap.index)
    .reduce((total, candidate) => total + lapDisplayTimeS(candidate, samples), 0));
}

function intervalDisplayTimeS(interval: ActivityInterval) {
  return rawDurationS(interval.raw) ?? (interval.movingTimeS > 0 ? interval.movingTimeS : interval.elapsedTimeS);
}

function lapDisplayTimeS(lap: ActivityLap, samples: ActivitySample[]) {
  return rawDurationS(lap.raw) ?? lapMovingTimeS(lap, samples);
}

function rawDurationS(raw?: Record<string, unknown>) {
  const value = raw?.duration;
  const parsed = typeof value === "string" ? Number(value) : value;
  return typeof parsed === "number" && Number.isFinite(parsed) && parsed >= 0 ? parsed : undefined;
}

function lapPaceSPKM(lap: ActivityLap, samples: ActivitySample[]) {
  if (isFinitePositive(lap.avgPaceSPKM)) return lap.avgPaceSPKM;
  if (!isFinitePositive(lap.distanceM)) return undefined;
  const movingTimeS = lapMovingTimeS(lap, samples);
  return movingTimeS > 0 ? movingTimeS / (lap.distanceM / 1000) : undefined;
}

function lapMovingTimeS(lap: ActivityLap, samples: ActivitySample[]) {
  return lap.movingTimeS > 0 ? lap.movingTimeS : movingLapTimeFromSamples(lap, samples);
}

function movingLapTimeFromSamples(lap: ActivityLap, samples: ActivitySample[]) {
  if (!lap.startTime || lap.elapsedTimeS <= 0 || samples.length < 2) return lap.elapsedTimeS;
  const startMs = Date.parse(lap.startTime);
  if (!Number.isFinite(startMs)) return lap.elapsedTimeS;
  const endMs = startMs + lap.elapsedTimeS * 1000;
  let movingMs = 0;
  for (let index = 1; index < samples.length; index += 1) {
    const previous = samples[index - 1];
    const current = samples[index];
    if (!previous.timestamp || !current.timestamp) continue;
    const previousMs = Date.parse(previous.timestamp);
    const currentMs = Date.parse(current.timestamp);
    const segmentStart = Math.max(startMs, previousMs);
    const segmentEnd = Math.min(endMs, currentMs);
    if (!Number.isFinite(previousMs) || !Number.isFinite(currentMs) || segmentEnd <= segmentStart) continue;
    const distanceDelta = (current.distanceM ?? 0) - (previous.distanceM ?? 0);
    if ((previous.speedMPS ?? 0) > 0.5 || (current.speedMPS ?? 0) > 0.5 || distanceDelta > 0.5) {
      movingMs += segmentEnd - segmentStart;
    }
  }
  return movingMs > 0 ? Math.round(movingMs / 1000) : lap.elapsedTimeS;
}

function intervalTargetLabel(workout: Activity["workout"], interval: ActivityInterval) {
  const stepType = interval.category === "active" ? "interval" : interval.category;
  const step = flattenWorkoutSteps(workout?.steps).find((candidate) => candidate.type?.toLowerCase() === stepType);
  if (!step) return undefined;
  if (step.targetType?.toLowerCase() === "pace.zone" && step.targetValueOne !== undefined && step.targetValueTwo !== undefined) {
    const paces = [speedToPaceSPKM(step.targetValueOne), speedToPaceSPKM(step.targetValueTwo)]
      .filter((pace): pace is number => pace !== undefined)
      .sort((left, right) => left - right);
    return paces.length === 2 ? `Target ${formatPace(paces[0])}–${formatPace(paces[1])}` : undefined;
  }
  if (step.endCondition?.toLowerCase() === "time" && step.endConditionValue !== undefined) {
    return `Target ${formatDuration(step.endConditionValue)}`;
  }
  return undefined;
}

function flattenWorkoutSteps(steps?: ActivityWorkoutStep[]) {
  const flattened: ActivityWorkoutStep[] = [];
  const visit = (items?: ActivityWorkoutStep[]) => {
    for (const item of items ?? []) {
      flattened.push(item);
      visit(item.children);
    }
  };
  visit(steps);
  return flattened;
}
