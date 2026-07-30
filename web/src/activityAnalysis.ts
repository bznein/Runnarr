import type { Activity } from "./types";

export type ActivityAnalysisTab = "stats" | "intervals";

export function hasIntervalAnalysis(activity: Pick<Activity, "intervals" | "laps"> | undefined) {
  return (activity?.intervals?.length ?? 0) > 0 || (activity?.laps?.length ?? 0) > 0;
}

export function resolveActivityAnalysisTab(selected: ActivityAnalysisTab, intervalsAvailable: boolean): ActivityAnalysisTab {
  return selected === "intervals" && !intervalsAvailable ? "stats" : selected;
}
