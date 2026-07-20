export type Session = {
  authenticated: boolean;
  csrfToken?: string;
};

export type ToolsPaceRequest = {
  distanceKm?: string;
  time?: string;
  pace?: string;
};

export type ToolsPaceResponse = {
  distanceKm: number;
  timeSeconds: number;
  paceSecondsPerKm: number;
  computed: "distance" | "time" | "pace";
  distanceLabel: string;
  timeLabel: string;
  paceLabel: string;
};

export type ToolsVdotEquivalent = {
  race: string;
  distanceKm: number;
  distanceLabel: string;
  timeSeconds: number;
  timeLabel: string;
};

export type ToolsVdotRequest = {
  distanceKm?: string;
  time?: string;
};

export type ToolsVdotResponse = {
  distanceKm: number;
  timeSeconds: number;
  vdot: number;
  vdotLabel: string;
  distanceLabel: string;
  timeLabel: string;
  equivalents: ToolsVdotEquivalent[];
};

export type ClimbDetectionSettings = {
  climbSmoothingRadiusM: number;
  minClimbDistanceM: number;
  minClimbElevationGainM: number;
  minClimbAverageGradePct: number;
  maxClimbMergeDipDistanceM: number;
  maxClimbMergeElevationLossM: number;
  climbStartGainM: number;
};

export type ClimbDetectionConfig = {
  preset: string;
  settings: ClimbDetectionSettings;
  sensitivity: number;
};

export type ClimbDetectionSettingsUpdate = {
  preset?: string;
  settings?: ClimbDetectionSettings;
  sensitivity?: number;
};

export type AppConfig = {
  mapTileURL: string;
  baseURL: string;
  climbDetection: ClimbDetectionConfig;
  trainingSheet: TrainingSheetConfig;
};

export type TrainingSheetConfig = {
  enabled: boolean;
  sheetURL: string;
  checkEveryHours: number;
  lastSyncedAt?: string;
};

export type ActivityClimbPreviewResponse = {
  climbs: ActivityClimb[];
};

export type ActivitySample = {
  index: number;
  timestamp?: string;
  elapsedS?: number;
  distanceM?: number;
  latitude?: number;
  longitude?: number;
  elevationM?: number;
  heartRate?: number;
  cadence?: number;
  power?: number;
  speedMPS?: number;
};

export type ActivityLap = {
  index: number;
  startTime?: string;
  elapsedTimeS: number;
  distanceM: number;
  elevationGainM?: number;
  elevationLossM?: number;
  avgGradeAdjustedPaceSPKM?: number;
};

export type ActivityClimb = {
  index: number;
  difficulty: string;
  startSampleIndex: number;
  endSampleIndex: number;
  startDistanceM: number;
  endDistanceM: number;
  distanceM: number;
  elevationGainM: number;
  avgGradePct: number;
  startElevationM: number;
  endElevationM: number;
};

export type ActivityMedia = {
  id: string;
  activityId: string;
  originalFilename: string;
  contentType: string;
  sizeBytes: number;
  sha256: string;
  width: number;
  height: number;
  captureTime?: string;
  latitude?: number;
  longitude?: number;
  createdAt: string;
};

export type GearSummary = {
  id: string;
  providerGearId: string;
  name: string;
  gearType?: string;
  brand?: string;
  model?: string;
  retired: boolean;
  totalDistanceM?: number;
  maxDistanceM?: number;
  defaultActivityTypes?: string[];
  lastUsedAt?: string;
};

export type Gear = GearSummary & {
  provider: string;
  firstUsedAt?: string;
  activityCount?: number;
  raw?: Record<string, unknown>;
  statsRaw?: Record<string, unknown>;
  createdAt?: string;
  updatedAt?: string;
};

export type Activity = {
  id: string;
  source: string;
  sourceId: string;
  name: string;
  sourceName: string;
  localName?: string;
  notes?: string;
  sportType: string;
  startTime: string;
  distanceM: number;
  movingTimeS: number;
  elapsedTimeS: number;
  elevationGainM: number;
  avgHeartRate?: number;
  maxHeartRate?: number;
  avgPaceSPKM?: number;
  avgGradeAdjustedPaceSPKM?: number;
  caloriesKcal?: number;
  originalProviderUrl?: string;
  summaryPolyline?: string;
  gear?: GearSummary[];
  samples?: ActivitySample[];
  laps?: ActivityLap[];
  climbs?: ActivityClimb[];
  media?: ActivityMedia[];
  createdAt: string;
};

export type DeleteActivityResult = {
  deleted: boolean;
  excludedFromSync: boolean;
  syncExclusionMessage?: string;
};

export type DeleteActivityMediaResult = {
  deleted: boolean;
};

export type ActivitySortBy = "date" | "duration" | "distance" | "elevation_gain" | "avg_pace" | "calories";
export type ActivitySortOrder = "asc" | "desc";

export type ActivityTypeFilters = {
  sports: string[];
  excludeSports: string[];
  search?: string;
  dateFrom?: string;
  dateTo?: string;
  sortBy?: ActivitySortBy;
  sortOrder?: ActivitySortOrder;
};

export type ActivityListPage = {
  activities: Activity[] | null;
  limit: number;
  offset: number;
  nextOffset?: number;
  hasMore: boolean;
};

export type CalendarActivitySummary = {
  id: string;
  name: string;
  startTime: string;
  sportType: string;
  source?: string;
  notes?: string;
  distanceM: number;
  movingTimeS: number;
};

export type CalendarDay = {
  date: string;
  activityCount: number;
  activities: CalendarActivitySummary[];
};

export type ActivityCalendar = {
  monthStart: string;
  monthEnd: string;
  days: CalendarDay[];
};

export type SummaryStats = {
  activityCount: number;
  distanceM: number;
  movingTimeS: number;
  elevationGainM: number;
  recent: Activity[] | null;
  weeklyDistance: Array<{ weekStart: string; distanceM: number }> | null;
};

export type ImportFile = {
  id: string;
  filename: string;
  contentType: string;
  sha256: string;
  sizeBytes: number;
  parser: string;
  status: string;
  error?: string;
  createdAt: string;
};

export type ProviderStatus = {
  configured: boolean;
  connected: boolean;
  connection: {
    provider: string;
    providerAccountId?: string;
    displayName?: string;
    scopes?: string[] | null;
    tokenExpiresAt?: string;
  };
};

export type GarminStatus = ProviderStatus;

export type GearListResponse = {
  gear: Gear[] | null;
  active: Gear[] | null;
  retired: Gear[] | null;
};

export type GearDetailResponse = {
  gear: Gear;
  activities: Activity[] | null;
};

export type SyncJob = {
  id: string;
  provider: string;
  kind: string;
  status: string;
  error?: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
  payload?: Record<string, unknown>;
};

export type DailyHealthMetric = {
  id?: string;
  provider: string;
  date: string;
  steps?: number;
  totalCaloriesKcal?: number;
  activeCaloriesKcal?: number;
  restingHeartRateBpm?: number;
  avgHeartRateBpm?: number;
  maxHeartRateBpm?: number;
  sleepDurationS?: number;
  deepSleepS?: number;
  lightSleepS?: number;
  remSleepS?: number;
  awakeSleepS?: number;
  sleepScore?: number;
  stressAvg?: number;
  stressMax?: number;
  bodyBatteryAvg?: number;
  bodyBatteryMin?: number;
  bodyBatteryMax?: number;
  bodyBatteryStart?: number;
  bodyBatteryEnd?: number;
  bodyBatteryGained?: number;
  bodyBatteryDrained?: number;
  hrvAvgMs?: number;
  hrvStatus?: string;
  weightKg?: number;
  bodyFatPct?: number;
  raw?: Record<string, unknown>;
  createdAt?: string;
  updatedAt?: string;
};
