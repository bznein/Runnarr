import type { Activity } from "./types";

export type SimpleMatchFilter = "all" | "unmatched" | "matched" | "attention";

export function normalizeSimpleMatchFilter(value: string | null): SimpleMatchFilter {
  return value === "unmatched" || value === "matched" || value === "attention" ? value : "all";
}

export function simpleMatchStatusLabel(activity: Activity) {
  switch (activity.trainingSheetMatch?.state) {
    case "pending": return "Pending writeback";
    case "writing": return "Writing";
    case "complete": return "Written";
    case "attention": return "Needs attention";
    default: return "Unmatched";
  }
}

export function shouldRedirectToSimple(pathname: string, defaultExperience: "full" | "simple" | undefined, supportMode = false) {
  return pathname === "/" && defaultExperience === "simple" && !supportMode;
}

export function fullPathForSimplePath(pathname: string) {
  const match = pathname.match(/^\/simple\/activities\/([^/]+)$/);
  return match ? `/activities/${match[1]}` : "/activities";
}

export function simpleIntervalSummary(activity: Activity) {
  const intervals = activity.intervals ?? [];
  if (intervals.length === 0) {
    return "Continuous run";
  }
  const categories = Array.from(new Set(intervals.map((interval) => interval.category).filter(Boolean)));
  const count = `${intervals.length} structured interval${intervals.length === 1 ? "" : "s"}`;
  const workout = activity.workout?.name?.trim();
  const categoryText = categories.length > 0 ? categories.join(", ") : "";
  return [workout, count, categoryText].filter(Boolean).join(" · ");
}
