import type {
  Activity,
  ActivityClimbPreviewResponse,
  ActivityListPage,
  ActivityNavigation,
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
  Course,
  CourseGarminStatus,
  CourseImportPreview,
  CourseImportResult,
  CourseImportSelection,
  CourseListPage,
  CoursePlanInput,
  CoursePlaceResult,
  CourseRoutingResponse,
  CourseSport,
  CourseSummary,
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
  UserPreference,
  Workout,
  WorkoutConfig,
  WorkoutMutation,
  WorkoutParseResult,
  WorkoutReconcileResult,
  NotificationPage,
  NotificationSettings,
  PushSubscriptionDevice,
  RunnarrNotification
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

export function courseGPXURL(id: string) {
  return `/api/courses/${encodeURIComponent(id)}/gpx`;
}

type ActivityPageOptions = {
  limit?: number;
  offset?: number;
  view?: "training-sheet-matching";
  matchState?: "all" | "unmatched" | "matched" | "attention";
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
  if (page?.view) {
    params.set("view", page.view);
  }
  if (page?.matchState) {
    params.set("matchState", page.matchState);
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
  notifications: (options: { limit?: number; cursor?: string; unread?: boolean } = {}) => {
    const params = new URLSearchParams();
    if (options.limit) params.set("limit", String(options.limit));
    if (options.cursor) params.set("cursor", options.cursor);
    if (options.unread) params.set("unread", "true");
    return request<NotificationPage>(`/api/notifications${params.size ? `?${params}` : ""}`);
  },
  notification: (id: string) => request<RunnarrNotification>(`/api/notifications/${encodeURIComponent(id)}`),
  setNotificationRead: (id: string, read: boolean) => request<{ updated: boolean }>(`/api/notifications/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify({ read })
  }),
  markAllNotificationsRead: () => request<{ updated: boolean }>("/api/notifications/read-all", { method: "POST" }),
  deleteNotification: (id: string) => request<{ deleted: boolean }>(`/api/notifications/${encodeURIComponent(id)}`, { method: "DELETE" }),
  clearNotifications: (scope: "read" | "all") => request<{ deleted: boolean }>(`/api/notifications?scope=${scope}`, { method: "DELETE" }),
  notificationSettings: () => request<NotificationSettings>("/api/notification-settings"),
  updateNotificationSettings: (categories: NotificationSettings["categories"]) => request<NotificationSettings>("/api/notification-settings", {
    method: "PATCH",
    body: JSON.stringify({ categories })
  }),
  pushSubscriptions: () => request<{ subscriptions: PushSubscriptionDevice[] }>("/api/push-subscriptions"),
  createPushSubscription: (body: PushSubscriptionJSON & { deviceName: string }) => request<PushSubscriptionDevice>("/api/push-subscriptions", {
    method: "POST",
    body: JSON.stringify(body)
  }),
  renamePushSubscription: (id: string, deviceName: string) => request<{ updated: boolean }>(`/api/push-subscriptions/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify({ deviceName })
  }),
  deletePushSubscription: (id: string) => request<{ deleted: boolean }>(`/api/push-subscriptions/${encodeURIComponent(id)}`, { method: "DELETE" }),
  deleteCurrentPushSubscription: (endpoint: string) => request<{ deleted: boolean }>("/api/push-subscriptions/current", {
    method: "DELETE",
    body: JSON.stringify({ endpoint })
  }),
  testPushSubscription: (id: string) => request<{ delivered: boolean }>(`/api/push-subscriptions/${encodeURIComponent(id)}/test`, { method: "POST" }),
  config: () => request<AppConfig>("/api/config"),
  summary: (filters?: ActivityTypeFilters, period: "weekly" | "monthly" | "yearly" = "weekly") => {
    const params = new URLSearchParams(activityFilterQuery(filters));
    params.set("period", period);
    return request<SummaryStats>(`/api/stats/summary?${params.toString()}`);
  },
  activityCalendar: (filters?: ActivityTypeFilters, timezone?: string) => {
    const params = new URLSearchParams(activityFilterQuery(filters));
    if (timezone) {
      params.set("timezone", timezone);
    }
    return request<ActivityCalendar>(`/api/stats/calendar?${params.toString()}`);
  },
  calendarDay: (date: string, timezone?: string) => {
    const params = new URLSearchParams({ date });
    if (timezone) {
      params.set("timezone", timezone);
    }
    return request<CalendarDayView>(`/api/stats/calendar/day?${params.toString()}`);
  },
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
  courses: (options: { q?: string; sport?: CourseSport | ""; favorite?: boolean; sort?: string; order?: string; limit?: number; offset?: number } = {}) => {
    const params = new URLSearchParams();
    if (options.q?.trim()) params.set("q", options.q.trim());
    if (options.sport) params.set("sport", options.sport);
    if (options.favorite !== undefined) params.set("favorite", String(options.favorite));
    if (options.sort) params.set("sort", options.sort);
    if (options.order) params.set("order", options.order);
    if (options.limit !== undefined) params.set("limit", String(options.limit));
    if (options.offset !== undefined) params.set("offset", String(options.offset));
    return request<CourseListPage>(`/api/courses${params.size ? `?${params}` : ""}`);
  },
  course: (id: string) => request<Course>(`/api/courses/${encodeURIComponent(id)}`),
  courseGarminStatus: (id: string) => request<CourseGarminStatus>(`/api/courses/${encodeURIComponent(id)}/garmin`),
  sendCourseToGarmin: (id: string) => request<CourseGarminStatus>(`/api/courses/${encodeURIComponent(id)}/garmin`, { method: "POST" }),
  createCourse: (body: CoursePlanInput) => request<Course>("/api/courses", { method: "POST", body: JSON.stringify(body) }),
  updateCoursePlan: (id: string, body: CoursePlanInput & { revision: number }) =>
    request<Course>(`/api/courses/${encodeURIComponent(id)}/plan`, { method: "PUT", body: JSON.stringify(body) }),
  routeCourseLegs: (body: { sportType: CourseSport; waypoints: Array<{ index: number; latitude: number; longitude: number }>; directLegIndexes: number[] }) =>
    request<CourseRoutingResponse>("/api/course-routing/legs", { method: "POST", body: JSON.stringify(body) }),
  searchCoursePlaces: (query: string) =>
    request<{ results: CoursePlaceResult[] }>(`/api/course-geocoding/search?q=${encodeURIComponent(query)}`),
  updateCourseDetails: (id: string, body: { revision: number; name: string; sportType: CourseSport; notes: string }) =>
    request<Course>(`/api/courses/${encodeURIComponent(id)}/details`, { method: "PATCH", body: JSON.stringify(body) }),
  setCourseFavorite: (id: string, favorite: boolean) =>
    request<CourseSummary>(`/api/courses/${encodeURIComponent(id)}/favorite`, { method: "PUT", body: JSON.stringify({ favorite }) }),
  duplicateCourse: (id: string, body: { revision: number; name: string; notes: string }) =>
    request<Course>(`/api/courses/${encodeURIComponent(id)}/duplicate`, { method: "POST", body: JSON.stringify(body) }),
  deleteCourse: (id: string, revision: number) =>
    request<Record<string, never>>(`/api/courses/${encodeURIComponent(id)}?revision=${revision}`, { method: "DELETE" }),
  previewCourseImport: (file: File) => {
    const body = new FormData();
    body.set("file", file);
    return request<CourseImportPreview>("/api/course-imports/preview", { method: "POST", body });
  },
  commitCourseImport: (file: File, fileSHA256: string, selections: CourseImportSelection[]) => {
    const body = new FormData();
    body.set("file", file);
    body.set("input", JSON.stringify({ fileSHA256, selections }));
    return request<CourseImportResult>("/api/course-imports/commit", { method: "POST", body });
  },
  courseImport: (id: string) => request<CourseImportResult>(`/api/course-imports/${encodeURIComponent(id)}`),
  saveActivityAsCourse: (id: string, body: { name: string; sportType: CourseSport; notes: string }) =>
    request<Course>(`/api/activities/${encodeURIComponent(id)}/course`, { method: "POST", body: JSON.stringify(body) }),
  activityNavigation: (id: string, filters?: ActivityTypeFilters) => {
    const query = activityFilterQuery(filters);
    return request<ActivityNavigation>(`/api/activities/${encodeURIComponent(id)}/navigation${query ? `?${query}` : ""}`);
  },
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
  workoutConfig: () => request<WorkoutConfig>("/api/config/workouts"),
  updateWorkoutConfig: (body: Partial<WorkoutConfig>) => request<WorkoutConfig>("/api/config/workouts", {
    method: "PATCH",
    body: JSON.stringify(body)
  }),
  workouts: (filter = "upcoming") => request<{ workouts: Workout[] }>(`/api/workouts?filter=${encodeURIComponent(filter)}`),
  workout: (id: string) => request<Workout>(`/api/workouts/${encodeURIComponent(id)}`),
  parseWorkout: (sourceText: string) => request<WorkoutParseResult>("/api/workouts/parse", {
    method: "POST",
    body: JSON.stringify({ sourceText })
  }),
  createWorkout: (body: WorkoutMutation) => request<Workout>("/api/workouts", {
    method: "POST",
    body: JSON.stringify(body)
  }),
  updateWorkout: (id: string, body: WorkoutMutation) => request<Workout>(`/api/workouts/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify(body)
  }),
  duplicateWorkout: (id: string) => request<Workout>(`/api/workouts/${encodeURIComponent(id)}/duplicate`, { method: "POST" }),
  deleteWorkout: (id: string) => request<{ deleted: boolean; archived: boolean }>(`/api/workouts/${encodeURIComponent(id)}`, { method: "DELETE" }),
  previewWorkoutReconcile: () => request<WorkoutReconcileResult>("/api/workouts/reconcile"),
  reconcileWorkouts: () => request<{ jobId: string; status: string }>("/api/workouts/reconcile", { method: "POST" }),
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
