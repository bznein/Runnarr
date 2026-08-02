export function chartDisplayDomain(values: Array<number | null | undefined>): [number, number] | undefined {
  const finite = values.filter((value): value is number => typeof value === "number" && Number.isFinite(value));
  if (finite.length === 0) {
    return undefined;
  }

  return expandedDomain(Math.min(...finite), Math.max(...finite));
}

function expandedDomain(min: number, max: number): [number, number] {
  const padding = min === max
    ? Math.max(Math.abs(min) * 0.05, 1)
    : (max - min) * 0.05;
  return [min - padding, max + padding];
}
