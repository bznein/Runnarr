import type { ThemePreference } from "./theme";

export type HeatmapAppearance = "app" | "light" | "dark";
export type HeatmapMetadata = {
  revision: number;
  sports: string[];
  mapped: number;
  excluded: number;
  pending: number;
  distanceM: number;
  bounds?: [number, number, number, number];
};

export function heatmapQuery(params: URLSearchParams, timezone = Intl.DateTimeFormat().resolvedOptions().timeZone) {
  const query = new URLSearchParams({ timezone });
  for (const key of ["from", "to"] as const) if (params.get(key)) query.set(key, params.get(key)!);
  const sports = params.getAll("sport");
  for (const sport of sports.length ? [...new Set(sports)].sort() : ["running"]) query.append("sport", sport);
  return query.toString();
}

export function heatmapAppearance(preference: HeatmapAppearance, theme: ThemePreference, systemDark: boolean): "light" | "dark" {
  if (preference !== "app") return preference;
  return theme === "midnight" || (theme === "system" && systemDark) ? "dark" : "light";
}

export function heatmapViewport(params: URLSearchParams): { center: [number, number]; zoom: number } | undefined {
  if (!["lat", "lng", "zoom"].every((key) => params.has(key))) return;
  const lat = Number(params.get("lat")), lng = Number(params.get("lng")), zoom = Number(params.get("zoom"));
  if (![lat, lng, zoom].every(Number.isFinite) || Math.abs(lat) > 85.051129 || Math.abs(lng) > 180 || zoom < 0 || zoom > 18) return;
  return { center: [lat, lng], zoom: Math.round(zoom) };
}

export function heatmapDatePreset(preset: string, now = new Date()) {
  const date = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
  if (preset === "all") return { from: "", to: "" };
  const start = new Date(now.getFullYear(), preset === "year" ? 0 : now.getMonth() - 12, preset === "year" ? 1 : now.getDate());
  return { from: date(start), to: date(now) };
}

export function heatmapExportSize(width: number, height: number) {
  const scale = Math.min(2, 4096 / Math.max(width, height));
  return { width: Math.round(width * scale), height: Math.round(height * scale), scale };
}

export function heatmapCaption(params: URLSearchParams) {
  const sports = params.getAll("sport");
  const sport = !sports.length || sports[0] === "running" ? "Running" : sports[0] === "all" ? "All sports" : sports.join(", ");
  const range = params.get("from") || params.get("to") ? `${params.get("from") || "Beginning"} – ${params.get("to") || "Today"}` : "All time";
  return `${sport} · ${range}`;
}
