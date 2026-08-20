import type { Activity, ActivityClimb, ActivityInterval, ActivityLap } from "./types";

type MarkdownColumn<T> = {
  heading: string;
  value: (item: T) => string | undefined;
  always?: boolean;
};

export function formatActivityForAI(activity: Activity) {
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
    lines.push("", "## Intervals", "", ...intervalTable(activity.intervals!));
  } else if ((activity.laps?.length ?? 0) > 0) {
    lines.push("", "## Laps", "", ...lapTable(activity.laps!));
  }

  if ((activity.climbs?.length ?? 0) > 0) {
    lines.push("", "## Climbs", "", ...climbTable(activity.climbs!));
  }

  return `${lines.join("\n")}\n`;
}

function appendFields(lines: string[], fields: Array<[string, string | undefined]>) {
  for (const [label, value] of fields) {
    const cleaned = value?.trim();
    if (cleaned) lines.push(`- ${label}: ${inlineText(cleaned)}`);
  }
}

function intervalTable(intervals: ActivityInterval[]) {
  return markdownTable(intervals, [
    { heading: "Step", value: (item) => String(item.index + 1), always: true },
    { heading: "Type", value: (item) => humanize(item.category), always: true },
    { heading: "Laps", value: (item) => formatIndexes(item.lapIndexes) },
    { heading: "Repeat", value: (item) => item.workoutRepeatIndex === undefined ? undefined : String(item.workoutRepeatIndex) },
    { heading: "Time", value: (item) => formatDuration(preferredDuration(item)), always: true },
    { heading: "Distance", value: (item) => formatDistance(item.distanceM), always: true },
    { heading: "Avg pace", value: (item) => formatPace(item.avgPaceSPKM) },
    { heading: "Avg GAP", value: (item) => formatPace(item.avgGradeAdjustedPaceSPKM) },
    { heading: "Avg HR", value: (item) => formatBPM(item.avgHeartRate) },
    { heading: "Max HR", value: (item) => formatBPM(item.maxHeartRate) },
    { heading: "Gain", value: (item) => formatMeters(item.elevationGainM) },
    { heading: "Loss", value: (item) => formatMeters(item.elevationLossM) },
    { heading: "Cadence", value: (item) => formatInteger(item.avgRunCadence, "spm") },
    { heading: "GCT", value: (item) => formatInteger(item.avgGroundContactTimeMS, "ms") },
    { heading: "Avg power", value: (item) => formatInteger(item.avgPower, "W") },
    { heading: "Max power", value: (item) => formatInteger(item.maxPower, "W") },
    { heading: "Normalized power", value: (item) => formatInteger(item.normalizedPower, "W") },
    { heading: "Calories", value: (item) => formatInteger(item.caloriesKcal, "kcal") }
  ]);
}

function lapTable(laps: ActivityLap[]) {
  return markdownTable(laps, [
    { heading: "Lap", value: (item) => String(item.index + 1), always: true },
    { heading: "Intensity", value: (item) => item.intensityType ? humanize(item.intensityType) : undefined },
    { heading: "Repeat", value: (item) => item.workoutRepeatIndex === undefined ? undefined : String(item.workoutRepeatIndex) },
    { heading: "Time", value: (item) => formatDuration(preferredDuration(item)), always: true },
    { heading: "Distance", value: (item) => formatDistance(item.distanceM), always: true },
    { heading: "Avg pace", value: (item) => formatPace(item.avgPaceSPKM) },
    { heading: "Avg GAP", value: (item) => formatPace(item.avgGradeAdjustedPaceSPKM) },
    { heading: "Avg HR", value: (item) => formatBPM(item.avgHeartRate) },
    { heading: "Max HR", value: (item) => formatBPM(item.maxHeartRate) },
    { heading: "Gain", value: (item) => formatMeters(item.elevationGainM) },
    { heading: "Loss", value: (item) => formatMeters(item.elevationLossM) },
    { heading: "Cadence", value: (item) => formatInteger(item.avgRunCadence, "spm") },
    { heading: "GCT", value: (item) => formatInteger(item.avgGroundContactTimeMS, "ms") },
    { heading: "Avg power", value: (item) => formatInteger(item.avgPower, "W") },
    { heading: "Max power", value: (item) => formatInteger(item.maxPower, "W") },
    { heading: "Normalized power", value: (item) => formatInteger(item.normalizedPower, "W") }
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

function preferredDuration(item: { movingTimeS: number; elapsedTimeS: number }) {
  return item.movingTimeS > 0 ? item.movingTimeS : item.elapsedTimeS;
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

function formatIndexes(indexes?: number[]) {
  if (!indexes?.length) return undefined;
  return indexes.map((index) => index + 1).join(", ");
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
