export function reconcileVisibleActivitySeries<T extends string>(visible: T[], available: T[], defaults: T[]) {
  const availableSet = new Set(available);
  const retained = unique(visible.filter((key) => availableSet.has(key)));
  if (retained.length > 0) {
    return retained;
  }
  const fallback = unique(defaults.filter((key) => availableSet.has(key)));
  return fallback.length > 0 ? fallback : available.slice(0, 1);
}

function unique<T>(values: T[]) {
  return [...new Set(values)];
}
