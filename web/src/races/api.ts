import { request } from "../api";
import type {
  Race,
  RaceCandidate,
  RaceDetail,
  RaceForecast,
  RaceInput,
  RacePage,
  RacePerformance,
  RaceReport,
  RaceResource,
  RaceSettings,
} from "./types";

const body = (method: string, value: unknown) => ({
  method,
  body: JSON.stringify(value),
});
const path = (id: string) => `/api/races/${encodeURIComponent(id)}`;
export const raceApi = {
  list: (query = "") => request<RacePage>(`/api/races?${query}`),
  detail: (id: string) => request<RaceDetail>(path(id)),
  save: (id: string | undefined, value: RaceInput) =>
    request<Race>(
      id ? path(id) : "/api/races",
      body(id ? "PUT" : "POST", value),
    ),
  delete: (id: string, revision: number) =>
    request<{ deleted: boolean }>(`${path(id)}?revision=${revision}`, {
      method: "DELETE",
    }),
  performance: (query = "") =>
    request<RacePerformance>(`/api/races/performance?${query}`),
  settings: () => request<RaceSettings>("/api/races/settings"),
  saveSettings: (value: RaceSettings) =>
    request<RaceSettings>("/api/races/settings", body("PUT", value)),
  resources: (kind: "groups" | "checklists") =>
    request<{ items: RaceResource[] }>(`/api/race-${kind}`),
  saveResource: (kind: "groups" | "checklists", value: Partial<RaceResource>) =>
    request<RaceResource>(
      `/api/race-${kind}${value.id ? `/${encodeURIComponent(value.id)}` : ""}`,
      body(value.id ? "PUT" : "POST", value),
    ),
  deleteResource: (kind: "groups" | "checklists", value: RaceResource) =>
    request<{ deleted: boolean }>(
      `/api/race-${kind}/${encodeURIComponent(value.id)}?revision=${value.revision}`,
      { method: "DELETE" },
    ),
  review: (offset = 0) =>
    request<{ activities: RaceCandidate[]; hasMore: boolean }>(
      `/api/races/review?offset=${offset}`,
    ),
  scan: () =>
    request<{ jobId: string }>("/api/races/review/scan", { method: "POST" }),
  dismiss: (id: string) =>
    request<{ dismissed: boolean }>(
      `/api/races/review/${encodeURIComponent(id)}/dismiss`,
      { method: "POST" },
    ),
  candidates: (id: string, query = "") =>
    request<{ activities: RaceCandidate[] }>(
      `${path(id)}/activity-candidates?q=${encodeURIComponent(query)}`,
    ),
  syncPredictions: () =>
    request<{ jobId: string }>("/api/races/predictions/sync", {
      method: "POST",
    }),
  forecast: (id: string) =>
    request<RaceForecast>(`${path(id)}/forecast`, { method: "POST" }),
  report: (id: string) =>
    request<{ draft: RaceReport; markdown: string }>(`${path(id)}/report`),
  saveReport: (id: string, value: RaceReport) =>
    request<RaceReport>(`${path(id)}/report`, body("PUT", value)),
  previewReport: (id: string, value: RaceReport) =>
    request<{ markdown: string }>(
      `${path(id)}/report/preview`,
      body("POST", value),
    ),
};
