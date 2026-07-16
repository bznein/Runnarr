import type { Activity, AppConfig, ImportFile, Session, SummaryStats } from "./types";

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
  summary: () => request<SummaryStats>("/api/stats/summary"),
  activities: () => request<{ activities: Activity[] | null }>("/api/activities?limit=100"),
  activity: (id: string) => request<{ activity: Activity }>(`/api/activities/${id}`),
  imports: () => request<{ imports: ImportFile[] | null }>("/api/imports"),
  upload: (file: File) => {
    const body = new FormData();
    body.set("file", file);
    return request<{ activity: Activity; import: ImportFile }>("/api/imports", {
      method: "POST",
      body
    });
  }
};
