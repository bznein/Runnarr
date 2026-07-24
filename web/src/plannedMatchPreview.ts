export function plannedMatchPreviewForActivity<T extends { activityId: string }>(preview: T, activityId?: string): T | undefined {
  return preview.activityId === activityId ? preview : undefined;
}
