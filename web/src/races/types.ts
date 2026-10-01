import type { Activity, Course, ToolsVdotEquivalent } from "../types";

export type RaceStatus =
  | "wishlist"
  | "planned"
  | "registered"
  | "finished"
  | "dns"
  | "dnf"
  | "disqualified"
  | "cancelled"
  | "postponed";
export type RaceDiscipline = "road" | "track" | "trail" | "cross_country";
export type RaceKind = "race" | "parkrun" | "virtual" | "time_trial";
export type RaceCheckpoint = {
  name: string;
  distanceM: number;
  timeMs: number;
};
export type RaceGoal = { name: string; timeMs?: number; achieved?: boolean };
export type RaceChecklistItem = {
  label: string;
  done: boolean;
  dueDate?: string;
  daysBefore?: number;
};
export type RaceResult = {
  confirmed: boolean;
  chipTimeMs?: number;
  gunTimeMs?: number;
  manualTimeMs?: number;
  overallPlace?: number;
  overallTotal?: number;
  category?: string;
  categoryPlace?: number;
  categoryTotal?: number;
  excluded: boolean;
  exclusionReason?: string;
  splitBasis?: "" | "chip" | "gun" | "manual";
  checkpoints: RaceCheckpoint[];
};
export type RaceInput = {
  revision: number;
  name: string;
  date?: string;
  startTime?: string;
  timezone: string;
  status: RaceStatus;
  discipline: RaceDiscipline;
  kind: RaceKind;
  distanceM?: number;
  priority?: "" | "A" | "B" | "C";
  groupId?: string;
  activityId?: string;
  plannedActivityId?: string;
  courseId?: string;
  refreshCourse?: boolean;
  removeCourse?: boolean;
  raceOnly: boolean;
  location?: string;
  latitude?: number;
  longitude?: number;
  eventUrl?: string;
  resultsUrl?: string;
  registrationDeadline?: string;
  registrationNotes?: string;
  bib?: string;
  startDetails?: string;
  travelNotes?: string;
  fuelingNotes?: string;
  pacingNotes?: string;
  kitNotes?: string;
  notes?: string;
  ageOverride?: number;
  tableOverride?: "" | "M" | "F";
  result: RaceResult;
  goals: RaceGoal[];
  checklist: RaceChecklistItem[];
};
export type Race = RaceInput & {
  id: string;
  courseSnapshot?: Course;
  createdAt: string;
  updatedAt: string;
};
export type RaceSummary = Pick<
  Race,
  | "id"
  | "name"
  | "date"
  | "status"
  | "activityId"
  | "plannedActivityId"
  | "priority"
  | "timezone"
  | "startTime"
  | "distanceM"
>;
export type RacePage = { races: Race[]; hasMore: boolean; nextOffset: number };
export type RaceSettings = {
  revision: number;
  birthDate?: string;
  gradingTable?: "" | "M" | "F";
  vdotReferenceId?: string;
  predictionsEnabled: boolean;
  weatherEnabled: boolean;
  predictionSyncedAt?: string;
  predictionAttemptedAt?: string;
  predictionBackfilled: boolean;
  predictionError?: string;
};
export type RaceResource = {
  id: string;
  revision: number;
  name: string;
  items?: RaceChecklistItem[];
};
export type RaceMetrics = {
  eligible: boolean;
  timeMs?: number;
  basis?: string;
  vdot?: number;
  vdotReason?: string;
  ageGrade?: number;
  ageGradeReason?: string;
  age?: number;
  table?: string;
  tableVersion: string;
  equivalents?: ToolsVdotEquivalent[];
};
export type RaceHalfway = {
  source?: string;
  basis?: string;
  firstHalfMs?: number;
  secondHalfMs?: number;
  differenceMs?: number;
  watchFirstHalfMs?: number;
  scaled: boolean;
  reason?: string;
};
export type RacePrediction = {
  date: string;
  distanceM: number;
  timeMs?: number;
  fetchedAt: string;
  backfilled: boolean;
};
export type RacePerformancePoint = Race & {
  metrics: RaceMetrics;
  personalBest: boolean;
  seasonBest: boolean;
};
export type RacePerformance = {
  results: RacePerformancePoint[];
  reference?: RacePerformancePoint;
  referenceReason?: string;
  predictions: RacePrediction[];
  tableVersion: string;
};
export type RaceComparison = {
  source: string;
  date: string;
  referenceId?: string;
  timeMs: number;
  differenceMs?: number;
  backfilled: boolean;
  capturedAt?: string;
  fetchedAt?: string;
  version?: string;
};
export type RaceBuildWeek = {
  from: string;
  to: string;
  distanceM: number;
  durationS: number;
  elevationM: number;
  count: number;
  longestM: number;
};
export type RaceForecastHour = {
  time: string;
  temperatureC?: number;
  apparentTemperatureC?: number;
  humidity?: number;
  rainProbability?: number;
  windKph?: number;
  windDirection?: number;
};
export type RaceForecast = {
  hours: RaceForecastHour[];
  fetchedAt?: string;
  attemptedAt?: string;
  error?: string;
  reason?: string;
  stale: boolean;
  saved: boolean;
  attribution: string;
};
export type RaceDetail = {
  race: Race;
  activity?: Activity;
  metrics: RaceMetrics;
  halfway: RaceHalfway;
  buildUp: RaceBuildWeek[];
  comparisons: RaceComparison[];
  forecast: RaceForecast;
  planWarning?: string;
};
export type RaceCandidate = {
  id: string;
  source: string;
  sourceId: string;
  name: string;
  sport: string;
  startTime: string;
  distanceM: number;
  movingTimeS: number;
  elapsedTimeS: number;
  reasons: string[];
  confidence?: string;
};
export type RaceReport = {
  revision: number;
  sections: string[];
  training: string;
  preparation: string;
  experience: string;
  reflections: string;
};
