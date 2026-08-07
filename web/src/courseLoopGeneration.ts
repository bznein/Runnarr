import type { CourseSport } from "./types";

export function courseLoopDistanceRange(sport: CourseSport) {
  return sport === "Cycling" ? { minimumKM: 5, maximumKM: 300 } : { minimumKM: 1, maximumKM: 100 };
}

export function parseCourseLoopDistanceKM(value: string, sport: CourseSport) {
  const distanceKM = Number(value);
  const range = courseLoopDistanceRange(sport);
  if (!Number.isFinite(distanceKM) || distanceKM < range.minimumKM || distanceKM > range.maximumKM) return undefined;
  return distanceKM;
}

export function courseLoopDistanceHint(sport: CourseSport) {
  const range = courseLoopDistanceRange(sport);
  return `${range.minimumKM}–${range.maximumKM} km`;
}

export function courseLoopDeviationLabel(deviationPct: number) {
  if (Math.abs(deviationPct) < 0.05) return "On target";
  return `${Math.abs(deviationPct).toFixed(1)}% ${deviationPct < 0 ? "shorter" : "longer"}`;
}
