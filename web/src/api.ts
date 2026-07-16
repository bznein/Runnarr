import type { Activity, ActivityTypeFilters, AppConfig, DeleteActivityResult, GarminStatus, ImportFile, Session, SummaryStats, SyncJob } from "./types";

export class ApiError extends Error {
  status: number;

  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

let csrfToken = "";

export function setCsrfToken(value?: string) {
  csrfToken = value ?? "";
}

function activityFilterQuery(filters?: ActivityTypeFilters) {
  const params = new URLSearchParams();
  for (const sport of filters?.sports ?? []) {
    params.append("sport", sport);
  }
  for (const sport of filters?.excludeSports ?? []) {
    params.append("excludeSport", sport);
  }
  if (filters?.search?.trim()) {
    params.set("search", filters.search.trim());
  }
  if (filters?.dateFrom) {
    params.set("dateFrom", filters.dateFrom);
  }
  if (filters?.dateTo) {
    params.set("dateTo", filters.dateTo);
  }
  if (filters?.sortBy) {
    params.set("sortBy", filters.sortBy);
  }
  if (filters?.sortOrder) {
    params.set("sortOrder", filters.sortOrder);
  }
  return params.toString();
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (!(init.body instanceof FormData) && init.body !== undefined && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (csrfToken && init.method && init.method !== "GET") {
    headers.set("X-CSRF-Token", csrfToken);
  }
  const response = await fetch(path, {
    credentials: "same-origin",
    ...init,
    headers
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new ApiError(payload.error ?? "Request failed", response.status);
  }
  return payload as T;
}

export const api = {
  session: () => request<Session>("/api/session"),
  login: (password: string) =>
    request<Session>("/api/session/login", {
      method: "POST",
      body: JSON.stringify({ password })
    }),
  logout: () => request<Session>("/api/session/logout", { method: "POST" }),
  config: () => request<AppConfig>("/api/config"),
  summary: (filters?: ActivityTypeFilters) => request<SummaryStats>(`/api/stats/summary?${activityFilterQuery(filters)}`),
  activities: (filters?: ActivityTypeFilters) => {
    const filtersQuery = activityFilterQuery(filters);
    return request<{ activities: Activity[] | null }>(`/api/activities?limit=100${filtersQuery ? `&${filtersQuery}` : ""}`);
  },
  activityTypes: () => request<{ activityTypes: string[] | null }>("/api/activity-types"),
  activity: (id: string) => request<{ activity: Activity }>(`/api/activities/${id}`),
  renameActivity: (id: string, name: string) =>
    request<{ activity: Activity }>(`/api/activities/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ name })
    }),
  deleteActivity: (id: string) => request<DeleteActivityResult>(`/api/activities/${id}`, { method: "DELETE" }),
  imports: () => request<{ imports: ImportFile[] | null }>("/api/imports"),
  upload: (file: File) => {
    const body = new FormData();
    body.set("file", file);
    return request<{ activity: Activity; import: ImportFile }>("/api/imports", {
      method: "POST",
      body
    });
  },
  garminStatus: () => request<GarminStatus>("/api/providers/garmin/status"),
  garminConnect: (body: { email: string; password: string; mfaCode?: string }) =>
    request<{ connected: boolean; connection: GarminStatus["connection"] }>("/api/providers/garmin/connect", {
      method: "POST",
      body: JSON.stringify(body)
    }),
  garminSync: (oldest: string) =>
    request<{ jobId: string; status: string }>("/api/providers/garmin/sync", {
      method: "POST",
      body: JSON.stringify({ oldest })
    }),
  syncJobs: () => request<{ jobs: SyncJob[] | null }>("/api/sync-jobs")
};
