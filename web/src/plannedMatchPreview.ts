export function plannedMatchActivityIsCurrent(activityId: string, currentActivityId?: string): boolean {
  return activityId === currentActivityId;
}

export function plannedMatchRequestIsCurrent(
  activityId: string,
  activityViewGeneration: number,
  currentActivityId: string | undefined,
  currentActivityViewGeneration: number
): boolean {
  return plannedMatchActivityIsCurrent(activityId, currentActivityId) && activityViewGeneration === currentActivityViewGeneration;
}

export function plannedMatchPreviewForActivity<T extends { activityId: string }>(preview: T, activityId?: string): T | undefined {
  return plannedMatchActivityIsCurrent(preview.activityId, activityId) ? preview : undefined;
}
