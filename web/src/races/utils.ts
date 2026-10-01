import type { Activity } from "../types";
import type { Race, RaceChecklistItem, RaceInput } from "./types";

export const raceStatuses = [
  "wishlist",
  "planned",
  "registered",
  "finished",
  "dns",
  "dnf",
  "disqualified",
  "cancelled",
  "postponed",
] as const;
export const raceDisciplines = [
  "road",
  "track",
  "trail",
  "cross_country",
] as const;
export const raceKinds = ["race", "parkrun", "virtual", "time_trial"] as const;
export const distancePresets = [
  { label: "1500 m", distanceM: 1500 },
  { label: "Mile", distanceM: 1609.344 },
  { label: "3K", distanceM: 3000 },
  { label: "5K", distanceM: 5000 },
  { label: "5 miles", distanceM: 8046.72 },
  { label: "10K", distanceM: 10000 },
  { label: "10 miles", distanceM: 16093.44 },
  { label: "Half marathon", distanceM: 21097.5 },
  { label: "Marathon", distanceM: 42195 },
  { label: "50K", distanceM: 50000 },
  { label: "50 miles", distanceM: 80467.2 },
  { label: "100K", distanceM: 100000 },
  { label: "100 miles", distanceM: 160934.4 },
];
export function raceLabel(value: string) {
  return ["dns", "dnf"].includes(value)
    ? value.toUpperCase()
    : value.replace(/_/g, " ").replace(/^./, (c) => c.toUpperCase());
}
export function formatRaceTime(ms?: number): string {
  if (ms === undefined || !Number.isFinite(ms)) return "";
  const sign = ms < 0 ? "−" : "";
  const value = Math.round(Math.abs(ms));
  const seconds = Math.floor(value / 1000);
  const fraction =
    value % 1000
      ? `.${String(value % 1000)
          .padStart(3, "0")
          .replace(/0+$/, "")}`
      : "";
  return `${sign}${seconds >= 3600 ? `${Math.floor(seconds / 3600)}:${String(Math.floor(seconds / 60) % 60).padStart(2, "0")}` : Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}${fraction}`;
}
export function parseRaceTime(text: string): number | undefined {
  if (!text.trim()) return undefined;
  if (!/^\d+(?::\d{1,2}){0,2}(?:\.\d{1,3})?$/.test(text.trim()))
    throw new Error(
      "Use seconds, m:ss or h:mm:ss, with up to three decimal places.",
    );
  const parts = text.trim().split(":").map(Number);
  if (parts.slice(1).some((v) => v >= 60))
    throw new Error("Minutes and seconds after a colon must be below 60.");
  const ms = Math.round(
    parts.reduce((sum, value) => sum * 60 + value, 0) * 1000,
  );
  if (ms <= 0 || ms > 365 * 86400000)
    throw new Error("Enter a positive duration shorter than a year.");
  return ms;
}
export function raceDistance(distanceM?: number): string {
  return distanceM === undefined
    ? ""
    : `${Number((distanceM / 1000).toFixed(6))} km`;
}
export function resultTime(r: Pick<Race, "result">) {
  return r.result.chipTimeMs ?? r.result.gunTimeMs ?? r.result.manualTimeMs;
}
export function eventToday(timezone: string, now = new Date()): string {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: timezone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(now);
  return `${parts.find((p) => p.type === "year")!.value}-${parts.find((p) => p.type === "month")!.value}-${parts.find((p) => p.type === "day")!.value}`;
}
export function raceCountdown(
  r: Pick<Race, "date" | "timezone" | "status">,
): string {
  if (
    !r.date ||
    !["wishlist", "planned", "registered", "postponed"].includes(r.status)
  )
    return "";
  const days = Math.round(
    (Date.parse(`${r.date}T00:00:00Z`) -
      Date.parse(`${eventToday(r.timezone)}T00:00:00Z`)) /
      86400000,
  );
  return days === 0
    ? "Today"
    : days > 0
      ? `${days} day${days === 1 ? "" : "s"} to go`
      : "Outcome awaiting update";
}
export function checklistDueDate(item: RaceChecklistItem, raceDate?: string) {
  if (item.dueDate) return item.dueDate;
  if (item.daysBefore !== undefined && raceDate)
    return new Date(
      Date.parse(`${raceDate}T12:00:00Z`) - item.daysBefore * 86400000,
    )
      .toISOString()
      .slice(0, 10);
  return "";
}
export function newRace(activity?: Activity): RaceInput {
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  return {
    revision: 0,
    name: activity?.name ?? "",
    date: activity
      ? eventToday(timezone, new Date(activity.startTime))
      : undefined,
    timezone,
    status: activity ? "finished" : "planned",
    discipline: "road",
    kind: "race",
    activityId: activity?.id,
    raceOnly: false,
    result: { confirmed: false, excluded: false, checkpoints: [] },
    goals: [],
    checklist: [],
  };
}
export function raceInput(r: Race): RaceInput {
  const {
    id: _id,
    createdAt: _created,
    updatedAt: _updated,
    courseSnapshot: _course,
    ...input
  } = r;
  return structuredClone(input);
}
