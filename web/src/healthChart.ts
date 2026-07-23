export const HEALTH_CHART_Y_AXIS_WIDTH = 64;

export function formatHealthAxisInteger(value?: number) {
  return isFiniteNumber(value) ? Math.round(value).toLocaleString() : "";
}

export function formatHealthAxisHours(value?: number) {
  return isFiniteNumber(value) ? `${value.toFixed(1)} h` : "";
}

export function formatHealthAxisBPM(value?: number) {
  return isFiniteNumber(value) ? `${Math.round(value)} bpm` : "";
}

export function formatHealthAxisMS(value?: number) {
  return isFiniteNumber(value) ? `${Math.round(value)} ms` : "";
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}
