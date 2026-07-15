export type Session = {
  authenticated: boolean;
  csrfToken?: string;
};

export type AppConfig = {
  mapTileURL: string;
  stravaConfigured: boolean;
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
};

export type Activity = {
  id: string;
  source: string;
  sourceId: string;
  name: string;
  sportType: string;
  startTime: string;
  distanceM: number;
  movingTimeS: number;
  elapsedTimeS: number;
  elevationGainM: number;
  avgHeartRate?: number;
  maxHeartRate?: number;
  avgPaceSPKM?: number;
  summaryPolyline?: string;
  samples?: ActivitySample[];
  laps?: ActivityLap[];
  createdAt: string;
};

export type DeleteActivityResult = {
  deleted: boolean;
  excludedFromSync: boolean;
  syncExclusionMessage?: string;
};

export type ActivityTypeFilters = {
  sports: string[];
  excludeSports: string[];
  search?: string;
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

export type StravaStatus = ProviderStatus;

export type IntervalsStatus = ProviderStatus;

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
