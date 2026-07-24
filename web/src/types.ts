export type Session = {
  authenticated: boolean;
  publicMode?: boolean;
  localLoginEnabled?: boolean;
  googleOIDCEnabled?: boolean;
  csrfToken?: string;
  actor?: SessionUser;
  user?: SessionUser;
  supportMode?: boolean;
  canWrite?: boolean;
};

export type SessionUser = {
  id: string;
  username: string;
  displayName: string;
  role: "admin" | "user";
};

export type User = SessionUser & {
  disabled: boolean;
  lastLoginAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type UserPreference = {
  themePreference: "system" | "light" | "dark";
  activityTableColumns?: string[];
  gearSortBy: string;
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
};

export type TrainingSheetConfig = {
  enabled: boolean;
  sheetURL: string;
  checkEveryHours: number;
  planYear?: number;
  lastSyncedAt?: string;
};

export type GoogleSheetsStatus = {
  configured: boolean;
  connected: boolean;
  writeReady: boolean;
  provider: string;
};

export type PlannedActivity = {
  id: string;
  source: string;
  sourceId: string;
  workbookId: string;
  sheetId: string;
  sheetTitle: string;
  planCell: string;
  feedbackCell?: string;
  plannedDate: string;
  name: string;
  sportType: string;
  notes?: string;
  status: string;
  sourceUrl?: string;
  matchedActivityId?: string;
  matchedAt?: string;
};

export type TrainingSheetPreviewChange = {
  range: string;
  section: "summary" | "intervals" | "feedback";
  label: string;
  currentValue: string;
  proposedValue: string;
  status: "write" | "conflict" | "unchanged" | "manual";
};

export type TrainingSheetPreviewCellStyle = {
  backgroundColor?: string;
  textColor?: string;
  bold?: boolean;
  italic?: boolean;
  fontSize?: number;
  horizontalAlignment?: string;
  verticalAlignment?: string;
  wrapStrategy?: string;
};

export type TrainingSheetPreviewColumn = {
  index: number;
  label: string;
  widthPx?: number;
  hidden?: boolean;
};

export type TrainingSheetPreviewCell = {
  ref: string;
  currentValue: string;
  displayValue: string;
  proposedValue?: string;
  status: "write" | "conflict" | "unchanged" | "manual";
  section?: string;
  label?: string;
  style?: TrainingSheetPreviewCellStyle;
  rowSpan?: number;
  columnSpan?: number;
};

export type TrainingSheetPreviewRow = {
  index: number;
  heightPx?: number;
  cells: TrainingSheetPreviewCell[];
};

export type TrainingSheetPreviewGrid = {
  startRow: number;
  endRow: number;
  startColumn: number;
  endColumn: number;
  formattingAvailable: boolean;
  columns: TrainingSheetPreviewColumn[];
  rows: TrainingSheetPreviewRow[];
};

export type TrainingSheetWritebackPreview = {
  activityId: string;
  plannedActivityId: string;
  sheetTitle: string;
  sheetUrl: string;
  fingerprint: string;
  changes: TrainingSheetPreviewChange[];
  grid: TrainingSheetPreviewGrid;
  warnings?: string[];
  writeCount: number;
  conflictCount: number;
};

export type TrainingSheetWritebackStatus = {
  plannedActivityId: string;
  activityId: string;
  jobId?: string;
  jobStatus?: string;
  cancelRequestedAt?: string;
  summaryStatus: string;
  summaryError?: string;
  summaryWrittenAt?: string;
  intervalsStatus: string;
  intervalsError?: string;
  intervalsWrittenAt?: string;
  feedbackStatus: string;
  feedbackError?: string;
  feedbackWrittenAt?: string;
  lastAttemptAt?: string;
};

export type PlannedActivityMatchResponse = {
  candidates: PlannedActivity[];
  suggestedId?: string;
  hasMore: boolean;
  matched?: PlannedActivity;
  writeback?: TrainingSheetWritebackStatus;
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

export type ActivitySeries = {
  samples: ActivitySample[];
  points: ActivitySeriesPoint[];
  totalSamples: number;
  sampled: boolean;
};

export type ActivitySeriesPoint = {
  index: number;
  label: string;
  distanceM?: number;
  latitude?: number;
  longitude?: number;
  elevationM?: number;
  heartRate?: number;
  paceSPKM?: number;
  rawPaceSPKM?: number;
  power?: number;
  cadence?: number;
};

export type ActivityLap = {
  index: number;
  startTime?: string;
  elapsedTimeS: number;
  movingTimeS: number;
  distanceM: number;
  avgPaceSPKM?: number;
  elevationGainM?: number;
  elevationLossM?: number;
  avgGradeAdjustedPaceSPKM?: number;
  avgHeartRate?: number;
  maxHeartRate?: number;
  avgPower?: number;
  maxPower?: number;
  normalizedPower?: number;
  avgRunCadence?: number;
  avgGroundContactTimeMS?: number;
  avgRespirationRate?: number;
  avgTemperatureC?: number;
  intensityType?: string;
  workoutStepIndex?: number;
  workoutRepeatIndex?: number;
  raw?: Record<string, unknown>;
};

export type ActivityWorkoutStep = {
  index: number;
  order: number;
  type?: string;
  description?: string;
  repeatCount?: number;
  endCondition?: string;
  endConditionValue?: number;
  targetType?: string;
  targetValueOne?: number;
  targetValueTwo?: number;
  targetValueUnit?: string;
  zoneNumber?: number;
  children?: ActivityWorkoutStep[];
};

export type ActivityWorkout = {
  provider: string;
  providerWorkoutId?: string;
  name?: string;
  sportType?: string;
  steps?: ActivityWorkoutStep[];
};

export type ActivityInterval = {
  index: number;
  category: string;
  providerType?: string;
  workoutStepIndex?: number;
  workoutRepeatIndex?: number;
  startTime?: string;
  endTime?: string;
  elapsedTimeS: number;
  movingTimeS: number;
  distanceM: number;
  avgPaceSPKM?: number;
  avgGradeAdjustedPaceSPKM?: number;
  avgHeartRate?: number;
  maxHeartRate?: number;
  avgPower?: number;
  maxPower?: number;
  normalizedPower?: number;
  avgRunCadence?: number;
  avgGroundContactTimeMS?: number;
  avgRespirationRate?: number;
  avgTemperatureC?: number;
  elevationGainM?: number;
  elevationLossM?: number;
  caloriesKcal?: number;
  lapIndexes?: number[];
  raw?: Record<string, unknown>;
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
  paceSPKM?: number;
  gapSPKM?: number;
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
  feedback?: string;
  rpe?: number;
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
  workout?: ActivityWorkout;
  intervals?: ActivityInterval[];
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

export type ActivityNavigation = {
  previousId?: string;
  nextId?: string;
};

export type CalendarActivitySummary = {
  id: string;
  source: string;
  name: string;
  startTime: string;
  sportType: string;
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
  distanceBuckets?: Array<{ start: string; distanceM: number }> | null;
  summaryPeriod?: "weekly" | "monthly" | "yearly";
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
  cancelRequestedAt?: string;
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

export type HealthChartPoint = {
  date: string;
  label?: string;
  steps?: number;
  totalCalories?: number;
  activeCalories?: number;
  remainingCalories?: number;
  sleepHours?: number;
  sleepScore?: number;
  restingHeartRate?: number;
  stress?: number;
  bodyBatteryGained?: number;
  bodyBatteryDrained?: number;
  bodyBatteryDrainedLoss?: number;
  bodyBatteryHighest?: number;
  hrv?: number;
  weight?: number;
};
