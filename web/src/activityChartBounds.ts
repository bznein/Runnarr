export function chartDisplayDomain(values: Array<number | null | undefined>): [number, number] | undefined {
  const sorted = values
    .filter((value): value is number => typeof value === "number" && Number.isFinite(value))
    .sort((left, right) => left - right);
  if (sorted.length === 0) {
    return undefined;
  }

  const actualMin = sorted[0];
  const actualMax = sorted[sorted.length - 1];
  if (sorted.length < 5) {
    return expandedDomain(actualMin, actualMax);
  }

  const low = quantile(sorted, 0.05);
  const high = quantile(sorted, 0.95);
  if (high - low < 1 && actualMax - actualMin >= 1) {
    return expandedDomain(actualMin, actualMax);
  }
  return expandedDomain(low, high);
}

function quantile(sorted: number[], ratio: number) {
  const position = Math.max(0, Math.min(sorted.length - 1, ratio * (sorted.length - 1)));
  const lower = Math.floor(position);
  const upper = Math.ceil(position);
  if (lower === upper) {
    return sorted[lower];
  }
  const fraction = position - lower;
  return sorted[lower] + (sorted[upper] - sorted[lower]) * fraction;
}

function expandedDomain(min: number, max: number): [number, number] {
  if (min !== max) {
    return [min, max];
  }
  const padding = Math.max(Math.abs(min) * 0.05, 1);
  return [min - padding, max + padding];
}
