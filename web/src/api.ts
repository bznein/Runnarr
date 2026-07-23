import type {
  Activity,
  ActivityClimbPreviewResponse,
  ActivityListPage,
  ActivitySeries,
  ActivityMedia,
  ActivityCalendar,
  CalendarDayView,
  ActivityTypeFilters,
  GoogleSheetsStatus,
  PlannedActivity,
  PlannedActivityMatchResponse,
  TrainingSheetWritebackPreview,
  TrainingSheetConfig,
  AppConfig,
  ClimbDetectionSettingsUpdate,
  DailyHealthMetric,
  HealthChartPoint,
  DeleteActivityMediaResult,
  DeleteActivityResult,
  GarminStatus,
  GearDetailResponse,
  GearListResponse,
  ImportFile,
  Session,
  SummaryStats,
  SyncJob,
  ToolsPaceRequest,
  ToolsPaceResponse,
  ToolsVdotRequest,
  ToolsVdotResponse,
  User,
  UserPreference
} from "./types";

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

export function activityGPXURL(id: string, includeSensors: boolean) {
  const query = includeSensors ? "?includeSensors=true" : "";
  return `/api/activities/${encodeURIComponent(id)}/gpx${query}`;
}

type ActivityPageOptions = {
  limit?: number;
  offset?: number;
};

type HealthRange = {
  from?: string;
  to?: string;
};

function activityFilterQuery(filters?: ActivityTypeFilters, page?: ActivityPageOptions) {
  const params = new URLSearchParams();
  if (page?.limit !== undefined) {
    params.set("limit", String(page.limit));
  }
  if (page?.offset !== undefined) {
    params.set("offset", String(page.offset));
  }
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

function healthRangeQuery(range?: HealthRange) {
  const params = new URLSearchParams();
  if (range?.from) {
    params.set("from", range.from);
  }
  if (range?.to) {
    params.set("to", range.to);
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
  login: (username: string, password: string) =>
    request<Session>("/api/session/login", {
      method: "POST",
      body: JSON.stringify({ username, password })
    }),
  logout: () => request<Session>("/api/session/logout", { method: "POST" }),
  startSupport: (userId: string) => request<Session>("/api/session/support", {
    method: "POST",
    body: JSON.stringify({ userId })
  }),
  stopSupport: () => request<Session>("/api/session/support", { method: "DELETE" }),
  changePassword: (currentPassword: string, newPassword: string) => request<{ updated: boolean }>("/api/session/password", {
    method: "POST",
    body: JSON.stringify({ currentPassword, newPassword })
  }),
  users: () => request<{ users: User[] }>("/api/users"),
  createUser: (body: { username: string; displayName: string; role: "admin" | "user"; password: string }) => request<{ user: User }>("/api/users", {
    method: "POST",
    body: JSON.stringify(body)
  }),
  updateUser: (id: string, body: { displayName?: string; role?: "admin" | "user"; disabled?: boolean }) => request<{ user: User }>(`/api/users/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify(body)
  }),
  resetUserPassword: (id: string, password: string) => request<{ updated: boolean }>(`/api/users/${encodeURIComponent(id)}/password`, {
    method: "POST",
    body: JSON.stringify({ password })
  }),
  preferences: () => request<UserPreference>("/api/preferences"),
  updatePreferences: (body: UserPreference) => request<UserPreference>("/api/preferences", {
    method: "PATCH",
    body: JSON.stringify(body)
  }),
  config: () => request<AppConfig>("/api/config"),
  summary: (filters?: ActivityTypeFilters, period: "weekly" | "monthly" | "yearly" = "weekly") => {
    const params = new URLSearchParams(activityFilterQuery(filters));
    params.set("period", period);
    return request<SummaryStats>(`/api/stats/summary?${params.toString()}`);
  },
  activityCalendar: (filters?: ActivityTypeFilters) => request<ActivityCalendar>(`/api/stats/calendar?${activityFilterQuery(filters)}`),
  calendarDay: (date: string) => request<CalendarDayView>(`/api/stats/calendar/day?date=${encodeURIComponent(date)}`),
  healthDaily: (range?: HealthRange) => {
    const query = healthRangeQuery(range);
    return request<{ from?: string; to?: string; metrics: DailyHealthMetric[] | null; chart?: HealthChartPoint[] }>(`/api/health/daily${query ? `?${query}` : ""}`);
  },
  toolsPace: (body: ToolsPaceRequest) => request<ToolsPaceResponse>("/api/tools/pace", {
    method: "POST",
    body: JSON.stringify(body)
  }),
  toolsVDOT: (body: ToolsVdotRequest) => request<ToolsVdotResponse>("/api/tools/vdot", {
    method: "POST",
    body: JSON.stringify(body)
  }),
  activityClimbPreview: (id: string, sensitivity: number) => request<ActivityClimbPreviewResponse>(`/api/activities/${encodeURIComponent(id)}/climbs-preview`, {
    method: "POST",
    body: JSON.stringify({ sensitivity })
  }),
  updateClimbDetectionSettings: (body: ClimbDetectionSettingsUpdate) => request<AppConfig>("/api/config/climb-detection", {
    method: "PATCH",
    body: JSON.stringify(body)
  }),
  activities: (filters?: ActivityTypeFilters, page?: ActivityPageOptions) => {
    const query = activityFilterQuery(filters, page);
    return request<ActivityListPage>(`/api/activities${query ? `?${query}` : ""}`);
  },
  activityTypes: () => request<{ activityTypes: string[] | null }>("/api/activity-types"),
  activity: (id: string) => request<{ activity: Activity }>(`/api/activities/${id}`),
  activitySeries: (id: string, maxPoints = 1200) => request<ActivitySeries>(`/api/activities/${encodeURIComponent(id)}/series?maxPoints=${maxPoints}`),
  gears: () => request<GearListResponse>("/api/gears"),
  gear: (id: string) => request<GearDetailResponse>(`/api/gears/${id}`),
  renameActivity: (id: string, name: string) =>
    request<{ activity: Activity }>(`/api/activities/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ name })
    }),
  updateActivityNotes: (id: string, notes: string) =>
    request<{ activity: Activity }>(`/api/activities/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ notes })
    }),
  updateActivityFeedback: (id: string, feedback: string) =>
    request<{ activity: Activity }>(`/api/activities/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ feedback })
    }),
  updateActivityRPE: (id: string, rpe: number | null) =>
    request<{ activity: Activity }>(`/api/activities/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ rpe })
    }),
  updateActivityReflection: (id: string, feedback: string, rpe: number | null) =>
    request<{ activity: Activity }>(`/api/activities/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ feedback, rpe })
    }),
  deleteActivity: (id: string) => request<DeleteActivityResult>(`/api/activities/${id}`, { method: "DELETE" }),
  uploadActivityMedia: (id: string, file: File) => {
    const body = new FormData();
    body.set("file", file);
    return request<{ media: ActivityMedia }>(`/api/activities/${id}/media`, {
      method: "POST",
      body
    });
  },
  updateActivityMediaLocation: (activityId: string, mediaId: string, latitude: number | null, longitude: number | null) =>
    request<{ media: ActivityMedia }>(`/api/activities/${activityId}/media/${mediaId}`, {
      method: "PATCH",
      body: JSON.stringify({ latitude, longitude })
    }),
  deleteActivityMedia: (activityId: string, mediaId: string) =>
    request<DeleteActivityMediaResult>(`/api/activities/${activityId}/media/${mediaId}`, { method: "DELETE" }),
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
  garminSync: (options: { oldest?: string; allData?: boolean }) =>
    request<{ jobId: string; status: string }>("/api/providers/garmin/sync", {
      method: "POST",
      body: JSON.stringify(options)
    }),
  garminHealthSync: (range?: HealthRange) =>
    request<{ jobId: string; status: string }>("/api/providers/garmin/health-sync", {
      method: "POST",
      body: JSON.stringify(range ?? {})
    }),
  garminGearSync: () => request<{ jobId: string; status: string }>("/api/providers/garmin/gear-sync", { method: "POST" }),
  googleSheetsStatus: () => request<GoogleSheetsStatus>("/api/providers/google/status"),
  trainingSheetConfig: () => request<TrainingSheetConfig>("/api/config/training-sheet"),
  updateTrainingSheetConfig: (body: Partial<TrainingSheetConfig> & { restoreDefaults?: boolean }) => request<TrainingSheetConfig>("/api/config/training-sheet", {
    method: "PATCH",
    body: JSON.stringify(body)
  }),
  trainingSheetSync: () => request<{ jobId: string; status: string }>("/api/training-sheet/sync", { method: "POST" }),
  plannedActivities: (from?: string, to?: string) => request<{ planned: PlannedActivity[] | null }>(`/api/planned-activities${from || to ? `?${new URLSearchParams({ ...(from ? { from } : {}), ...(to ? { to } : {}) }).toString()}` : ""}`),
  plannedMatchCandidates: (activityID: string, windowDays = 7) => request<PlannedActivityMatchResponse>(`/api/activities/${activityID}/planned-match-candidates?windowDays=${windowDays}`),
  plannedMatchPreview: (activityID: string, body: { plannedActivityId: string; feedback?: string; rpe: number | null; rpeSet: boolean; overrides?: Record<string, string> }) =>
    request<{ preview: TrainingSheetWritebackPreview }>(`/api/activities/${activityID}/planned-match-preview`, {
      method: "POST",
      body: JSON.stringify(body)
    }),
  applyPlannedMatchPreview: (activityID: string, body: { plannedActivityId: string; feedback?: string; rpe: number | null; rpeSet: boolean; overrides?: Record<string, string>; fingerprint: string }) =>
    request<{ planned: PlannedActivity; writebackJobId?: string; status: string }>(`/api/activities/${activityID}/planned-match-apply`, {
      method: "POST",
      body: JSON.stringify(body)
    }),
  matchPlannedActivity: (activityID: string, plannedActivityId: string) => request<{ planned: PlannedActivity }>(`/api/activities/${activityID}/planned-match`, {
    method: "POST",
    body: JSON.stringify({ plannedActivityId })
  }),
  unmatchPlannedActivity: (activityID: string) => request<{ matched: boolean }>(`/api/activities/${activityID}/planned-match`, { method: "DELETE" }),
  retryPlannedWriteback: (activityID: string) => request<{ jobId: string; status: string }>(`/api/activities/${activityID}/planned-writeback`, { method: "POST" }),
  syncJobs: () => request<{ jobs: SyncJob[] | null }>("/api/sync-jobs"),
  cancelSyncJob: (jobId: string) => request<{ jobId: string; status: string; cancelRequested: boolean }>(`/api/sync-jobs/${encodeURIComponent(jobId)}/cancel`, { method: "POST" })
};
