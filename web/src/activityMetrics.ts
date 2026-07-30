const strengthSportTypes = new Set([
  "strength",
  "strengthtraining",
  "weightlifting",
  "weighttraining"
]);

export function supportsRouteMetrics(sportType: string): boolean {
  const normalizedSportType = sportType.trim().toLowerCase().replace(/[^a-z0-9]/g, "");
  return !strengthSportTypes.has(normalizedSportType);
}
