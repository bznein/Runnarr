import type { Activity } from "./types";

export type ActivityAnalysisTab = "stats" | "intervals" | "workout";

export function hasIntervalAnalysis(activity: Pick<Activity, "intervals" | "laps"> | undefined) {
  return (activity?.intervals?.length ?? 0) > 0 || (activity?.laps?.length ?? 0) > 0;
}

export function resolveActivityAnalysisTab(selected: ActivityAnalysisTab, intervalsAvailable: boolean, workoutAvailable = false): ActivityAnalysisTab {
  return (selected === "intervals" && !intervalsAvailable) || (selected === "workout" && !workoutAvailable) ? "stats" : selected;
}
