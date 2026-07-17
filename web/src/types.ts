export type Session = {
  authenticated: boolean;
  csrfToken?: string;
};

export type AppConfig = {
  mapTileURL: string;
  baseURL: string;
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

export type Activity = {
  id: string;
  source: string;
  sourceId: string;
  name: string;
  sourceName: string;
  localName?: string;
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
