export type PaceDisplayScale = {
  minPaceSPKM: number;
  maxPaceSPKM: number;
};

export const PACE_ROUTE_COLORS = ["#2f6df6", "#168fd2", "#18a7a2", "#2f9e44", "#8fbf26", "#f6c432", "#f28c28", "#e85d35", "#cf3f35"];

const PACE_SCALE_LOW_PERCENTILE = 0.05;
const PACE_SCALE_HIGH_PERCENTILE = 0.95;

export function speedToPaceSPKM(speedMPS: number | null | undefined) {
  if (typeof speedMPS !== "number" || !Number.isFinite(speedMPS) || speedMPS <= 0) {
    return undefined;
  }
  return 1000 / speedMPS;
}

export function paceForRouteSegment(previousSpeedMPS: number | undefined, currentSpeedMPS: number | undefined) {
  if (!isPositiveFiniteSpeed(previousSpeedMPS) || !isPositiveFiniteSpeed(currentSpeedMPS)) {
    return undefined;
  }
  return speedToPaceSPKM((previousSpeedMPS + currentSpeedMPS) / 2);
}

export function paceScaleFromSpeeds(speedsMPS: Array<number | null | undefined>) {
  return paceScaleFromPaces(speedsMPS.map(speedToPaceSPKM));
}

export function paceScaleFromPaces(pacesSPKM: Array<number | null | undefined>): PaceDisplayScale | undefined {
  const sorted = pacesSPKM
    .filter((pace): pace is number => typeof pace === "number" && Number.isFinite(pace) && pace > 0)
    .sort((left, right) => left - right);
  if (sorted.length === 0) {
    return undefined;
  }

  const actualMin = sorted[0];
  const actualMax = sorted[sorted.length - 1];
  let minPaceSPKM = quantile(sorted, PACE_SCALE_LOW_PERCENTILE);
  let maxPaceSPKM = quantile(sorted, PACE_SCALE_HIGH_PERCENTILE);
  if (maxPaceSPKM - minPaceSPKM < 1 && actualMax - actualMin >= 1) {
    minPaceSPKM = actualMin;
    maxPaceSPKM = actualMax;
  }
  return { minPaceSPKM, maxPaceSPKM };
}

export function clampPaceToScale(paceSPKM: number | null | undefined, scale?: PaceDisplayScale) {
  if (typeof paceSPKM !== "number" || !Number.isFinite(paceSPKM) || paceSPKM <= 0) {
    return undefined;
  }
  if (!scale) {
    return paceSPKM;
  }
  return Math.max(scale.minPaceSPKM, Math.min(scale.maxPaceSPKM, paceSPKM));
}

export function paceColorForPace(paceSPKM: number, scale: PaceDisplayScale, colors = PACE_ROUTE_COLORS) {
  const middleColor = colors[Math.floor(colors.length / 2)] ?? "#f6c432";
  if (colors.length === 0 || scale.maxPaceSPKM - scale.minPaceSPKM < 1) {
    return middleColor;
  }

  const clampedPace = clampPaceToScale(paceSPKM, scale);
  if (clampedPace === undefined) {
    return middleColor;
  }
  const normalized = (scale.maxPaceSPKM - clampedPace) / (scale.maxPaceSPKM - scale.minPaceSPKM);
  const index = Math.max(0, Math.min(colors.length - 1, Math.round(normalized * (colors.length - 1))));
  return colors[index];
}

function quantile(sortedValues: number[], quantileValue: number) {
  const position = Math.max(0, Math.min(sortedValues.length - 1, quantileValue * (sortedValues.length - 1)));
  const lowerIndex = Math.floor(position);
  const upperIndex = Math.ceil(position);
  if (lowerIndex === upperIndex) {
    return sortedValues[lowerIndex];
  }
  const ratio = position - lowerIndex;
  return sortedValues[lowerIndex] + (sortedValues[upperIndex] - sortedValues[lowerIndex]) * ratio;
}

function isPositiveFiniteSpeed(value: number | undefined): value is number {
  return typeof value === "number" && Number.isFinite(value) && value > 0;
}
