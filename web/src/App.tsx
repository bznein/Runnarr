import { Fragment, useEffect, useRef, useState } from "react";
import type { CSSProperties, ReactNode } from "react";
import { Link, NavLink, Navigate, Route, Routes, useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { QueryClient } from "@tanstack/react-query";
import { Activity as ActivityIcon, ArrowDown, ArrowUp, ArrowUpDown, BarChart3, CalendarDays, Calculator, ChevronDown, ChevronLeft, ChevronRight, Cloud, Columns3, Database, Download, ExternalLink, Filter, Flame, Footprints, HeartPulse, LogOut, Map as MapIcon, Moon, MoreVertical, Pencil, RefreshCw, Route as RouteIcon, Scale, Mountain, Timer, Settings as SettingsIcon, Square, StickyNote, Sun, Trash2, Upload, X, BatteryCharging, RotateCcw, Monitor } from "lucide-react";
import { divIcon } from "leaflet";
import { MapContainer, Marker, Polyline, TileLayer, useMap } from "react-leaflet";
import { Area, AreaChart, Bar, BarChart, CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { activityGPXURL, api, ApiError, setCsrfToken } from "./api";
import { HEALTH_CHART_Y_AXIS_WIDTH, formatHealthAxisBPM, formatHealthAxisHours, formatHealthAxisInteger, formatHealthAxisMS } from "./healthChart";
import { PACE_ROUTE_COLORS, clampPaceToScale, paceColorForPace, paceForRouteSegment, paceScaleFromPaces, paceScaleFromSpeeds, speedToPaceSPKM } from "./paceDisplay";
import type { PaceDisplayScale } from "./paceDisplay";
import type {
  Activity,
  ActivityClimb,
  ActivityInterval,
  ActivityLap,
  ActivityMedia,
  ActivitySample,
  ActivityWorkoutStep,
  ActivitySortBy,
  ActivityTypeFilters as ActivityTypeFiltersValue,
  AppConfig,
  DailyHealthMetric,
  Gear,
  GearSummary,
  ImportFile,
  PlannedActivityMatchResponse,
  TrainingSheetWritebackPreview,
  Session,
  SyncJob,
  TrainingSheetPreviewChange,
  TrainingSheetPreviewCell,
  ToolsPaceResponse,
  ToolsVdotResponse,
  UserPreference
} from "./types";

type RoutePoint = [number, number];
type ActivityDateRange = Pick<ActivityTypeFiltersValue, "dateFrom" | "dateTo">;
type ActivitySort = Required<Pick<ActivityTypeFiltersValue, "sortBy" | "sortOrder">>;
type HealthDateRange = { from: string; to: string };
type GearSortBy = "first_used" | "last_used" | "activity_count" | "distance" | "distance_percent";
type ThemePreference = "system" | "light" | "dark";
type ActivityTableColumnKey = "date" | "type" | "gear" | "distance" | "time" | "calories" | "source";
type ActivityChartSeriesKey = "elevationM" | "heartRate" | "paceSPKM" | "power" | "cadence";
type ActivityAnalysisTab = "stats" | "intervals";
type PlannedMatchDraft = { plannedActivityId: string; feedback?: string; rpe: number | null; rpeSet: boolean; overrides?: Record<string, string> };
type ActivityChartPoint = {
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
type RouteColorSource = "pace" | "gap";
type CalendarMonth = { year: number; month: number };
type ActivityChartSeries = {
  key: ActivityChartSeriesKey;
  label: string;
  color: string;
  defaultVisible: boolean;
  format: (value: number) => string;
};
type ClimbProfilePoint = {
  label: string;
  distanceKm: number;
  elevationM: number;
};
type ClimbMapSegment = {
  climb: ActivityClimb;
  points: RoutePoint[];
  start?: RoutePoint;
};
type PaceRouteSegment = {
  points: RoutePoint[];
  color: string;
};
type HealthChartPoint = {
  date: string;
  label: string;
  steps?: number;
  totalCalories?: number;
  activeCalories?: number;
  remainingCalories?: number;
  sleepHours?: number;
  restingHeartRate?: number;
  stress?: number;
  bodyBatteryGained?: number;
  bodyBatteryDrained?: number;
  bodyBatteryDrainedLoss?: number;
  bodyBatteryHighest?: number;
  hrv?: number;
  weight?: number;
};

const defaultActivitySort: ActivitySort = { sortBy: "date", sortOrder: "desc" };
const emptyActivityTypeFilters: ActivityTypeFiltersValue = { sports: [], excludeSports: [], search: "", dateFrom: "", dateTo: "", ...defaultActivitySort };
const ACTIVITY_LIST_PAGE_SIZE = 100;
const garminHealthDefaultDays = 7;
const healthBarChartMaxDays = 30;
const defaultClimbSensitivity = 50;
const vdotDistancePresets: Array<{ id: string; label: string; distanceKm: string }> = [
  { id: "marathon", label: "Marathon", distanceKm: "42.195" },
  { id: "half-marathon", label: "HM", distanceKm: "21.0975" },
  { id: "10m", label: "10M", distanceKm: "16.0934" },
  { id: "10k", label: "10K", distanceKm: "10" },
  { id: "5k", label: "5K", distanceKm: "5" }
];
const climbSensitivityPresets: Array<{ id: string; label: string; value: number }> = [
  {
    id: "conservative",
    label: "Conservative",
    value: 0
  },
  {
    id: "balanced",
    label: "Balanced",
    value: 50
  },
  {
    id: "aggressive",
    label: "Aggressive",
    value: 100
  }
];
const activityTableColumnOptions: Array<{ key: ActivityTableColumnKey; label: string }> = [
  { key: "date", label: "Date" },
  { key: "type", label: "Type" },
  { key: "gear", label: "Gear" },
  { key: "distance", label: "Distance" },
  { key: "time", label: "Time" },
  { key: "calories", label: "Calories" },
  { key: "source", label: "Source" }
];
const defaultActivityTableColumns: ActivityTableColumnKey[] = activityTableColumnOptions.map((option) => option.key);
const compactActivityTableColumns: ActivityTableColumnKey[] = ["date", "distance", "time"];
const defaultGearSortBy: GearSortBy = "distance_percent";
const gearSortByOptions: Array<{ value: GearSortBy; label: string }> = [
  { value: "distance", label: "Total distance" },
  { value: "distance_percent", label: "Percent of distance limit" },
  { value: "last_used", label: "Last used" },
  { value: "first_used", label: "First used" },
  { value: "activity_count", label: "Activity count" }
];
const preferencesQueryKey = (userID?: string) => ["preferences", userID] as const;
const ELEVATION_SMOOTHING_RADIUS_M = 150;
const ELEVATION_SMOOTHING_SAMPLE_RADIUS = 36;
const chartTooltipContentStyle: CSSProperties = {
  border: "1px solid var(--color-border-control)",
  borderRadius: 8,
  background: "var(--color-surface)",
  boxShadow: "var(--shadow-menu)",
  color: "var(--color-text)"
};
const chartTooltipLabelStyle: CSSProperties = {
  color: "var(--color-muted-strong)",
  fontWeight: 700
};
const chartTooltipCursorStyle = {
  fill: "var(--color-surface-soft)",
  opacity: 0.72
};
const calendarWeekdays = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
const activityChartSeries: ActivityChartSeries[] = [
  { key: "elevationM", label: "Elevation", color: "#4664c9", defaultVisible: true, format: (value) => `${Math.round(value).toLocaleString()} m` },
  { key: "heartRate", label: "Heart rate", color: "#c84d4d", defaultVisible: true, format: (value) => `${Math.round(value)} bpm` },
  { key: "paceSPKM", label: "Pace", color: "#2f8f83", defaultVisible: true, format: (value) => formatPace(value) },
  { key: "power", label: "Power", color: "#b7791f", defaultVisible: false, format: (value) => `${Math.round(value)} W` },
  { key: "cadence", label: "Cadence", color: "#7a4eb2", defaultVisible: false, format: (value) => `${Math.round(value)} spm` }
];

function applyThemePreference(preference: ThemePreference) {
  const root = document.documentElement;
  if (preference === "system") {
    delete root.dataset.theme;
    return;
  }
  root.dataset.theme = preference;
}

function normalizeActivityTableColumns(columns?: string[]): ActivityTableColumnKey[] {
  if (!columns || columns.length === 0) {
    return defaultActivityTableColumns;
  }
  const allowed = new Set(defaultActivityTableColumns);
  const normalized = columns.filter((item): item is ActivityTableColumnKey => allowed.has(item as ActivityTableColumnKey));
  return normalized.length > 0 ? normalized : defaultActivityTableColumns;
}

function normalizeGearSortBy(value?: string): GearSortBy {
  return isGearSortBy(value ?? null) ? value as GearSortBy : defaultGearSortBy;
}

function mergeUserPreference(current: UserPreference | undefined, updates: Partial<UserPreference>): UserPreference {
  return {
    themePreference: current?.themePreference ?? "system",
    activityTableColumns: current?.activityTableColumns ?? defaultActivityTableColumns,
    gearSortBy: current?.gearSortBy || defaultGearSortBy,
    ...updates
  };
}

function useSaveUserPreferences(userID?: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (updates: Partial<UserPreference>) => {
      if (!userID) {
        throw new Error("User session is not available");
      }
      const current = await queryClient.fetchQuery<UserPreference>({
        queryKey: preferencesQueryKey(userID),
        queryFn: api.preferences
      });
      return api.updatePreferences(mergeUserPreference(current, updates));
    },
    onSuccess: (next) => {
      if (userID) {
        queryClient.setQueryData(preferencesQueryKey(userID), next);
      }
    }
  });
}

export function App() {
  const [themePreference, setThemePreference] = useState<ThemePreference>("system");
  const queryClient = useQueryClient();
  const session = useQuery({ queryKey: ["session"], queryFn: api.session });
  const effectiveUserID = session.data?.user?.id;
  const preferences = useQuery({
    queryKey: preferencesQueryKey(effectiveUserID),
    queryFn: api.preferences,
    enabled: Boolean(session.data?.authenticated && effectiveUserID)
  });
  const savePreferences = useSaveUserPreferences(effectiveUserID);

  useEffect(() => {
    applyThemePreference(themePreference);
  }, [themePreference]);

  useEffect(() => {
    setThemePreference(preferences.data?.themePreference ?? "system");
  }, [effectiveUserID, preferences.data?.themePreference]);

  useEffect(() => {
    if (!effectiveUserID) {
      return;
    }
    queryClient.removeQueries({
      predicate: (query) => query.queryKey[0] !== "session" && query.queryKey[0] !== "preferences"
    });
  }, [effectiveUserID, queryClient]);

  useEffect(() => {
    setCsrfToken(session.data?.csrfToken);
  }, [session.data?.csrfToken]);

  if (session.isLoading) {
    return <FullScreenMessage title="Runnarr" message="Loading session" />;
  }

  if (!session.data?.authenticated) {
    return (
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    );
  }

  const onThemePreferenceChange = (preference: ThemePreference) => {
    setThemePreference(preference);
    if (session.data?.canWrite !== false && effectiveUserID) {
      savePreferences.mutate({ themePreference: preference });
    }
  };

  return (
    <AuthenticatedApp session={session.data} themePreference={themePreference} onThemePreferenceChange={onThemePreferenceChange} themePreferenceError={savePreferences.error} />
  );
}

function AuthenticatedApp({
  session,
  themePreference,
  onThemePreferenceChange,
  themePreferenceError
}: {
  session?: Session;
  themePreference: ThemePreference;
  onThemePreferenceChange: (preference: ThemePreference) => void;
  themePreferenceError?: Error | null;
}) {
  const config = useQuery({ queryKey: ["config"], queryFn: api.config });
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: async () => {
      setCsrfToken("");
      await queryClient.invalidateQueries({ queryKey: ["session"] });
      navigate("/login");
    }
  });

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <Link to="/" className="brand">
          <ActivityIcon size={24} />
          <span>Runnarr</span>
        </Link>
        <nav className="nav">
          <NavItem to="/" icon={<BarChart3 size={18} />} label="Dashboard" />
          <NavItem to="/activities" icon={<MapIcon size={18} />} label="Activities" />
          <NavItem to="/calendar" icon={<CalendarDays size={18} />} label="Calendar" />
          <NavItem to="/health" icon={<HeartPulse size={18} />} label="Health" />
          <NavItem to="/tools" icon={<Calculator size={18} />} label="Tools" />
          <NavItem to="/gear" icon={<Footprints size={18} />} label="Gear" />
        </nav>
        <div className="sidebar-bottom">
          <div className="account-chip" title={session?.user?.username}>
            <strong>{session?.user?.displayName || session?.user?.username}</strong>
            <span>{session?.user?.role === "admin" ? "Administrator" : "User"}</span>
          </div>
          <NavItem to="/settings" icon={<SettingsIcon size={18} />} label="Settings" />
          <button className="nav-button" type="button" onClick={() => logout.mutate()}>
            <LogOut size={18} />
            <span>Log out</span>
          </button>
        </div>
      </aside>
      <main className="main">
        {session?.supportMode && (
          <div className="support-banner">
            <span>Read-only support view: {session.user?.displayName || session.user?.username}</span>
            <button className="secondary-button small-button" type="button" onClick={() => {
              void api.stopSupport().then(() => window.location.reload());
            }}>Exit support view</button>
          </div>
        )}
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/activities" element={<ActivitiesPage />} />
          <Route path="/activities/:id" element={<ActivityDetailPage config={config.data} />} />
          <Route path="/calendar" element={<ActivityCalendarPage />} />
          <Route path="/health" element={<HealthPage />} />
          <Route path="/tools" element={<ToolsPage />} />
          <Route path="/gear" element={<GearPage />} />
          <Route path="/gear/:id" element={<GearDetailPage />} />
          <Route path="/imports" element={<Navigate to="/settings#import" replace />} />
          <Route path="/settings" element={<SettingsPage themePreference={themePreference} onThemePreferenceChange={onThemePreferenceChange} themePreferenceError={themePreferenceError} />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  );
}

function NavItem({ to, icon, label }: { to: string; icon: JSX.Element; label: string }) {
  return (
    <NavLink to={to} className={({ isActive }) => `nav-link ${isActive ? "active" : ""}`} end={to === "/"}>
      {icon}
      <span>{label}</span>
    </NavLink>
  );
}

function deleteActivityConfirmation(activity: Activity) {
  if (activity.source === "file") {
    return `Delete "${activity.name}" from Runnarr?`;
  }
  const source = formatSourceName(activity.source);
  return [
    `Remove "${activity.name}" from Runnarr?`,
    `Because this came from ${source}, Runnarr will remember it as ignored and will not import it again during future syncs.`,
    `This will not delete it from ${source}.`
  ].join("\n\n");
}

function formatSourceName(source: string) {
  switch (source) {
    case "garmin":
      return "Garmin Connect";
    case "file":
      return "manual upload";
    default:
      return source;
  }
}

function invalidateGearRelatedQueries(queryClient: QueryClient) {
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: ["gears"] }),
    queryClient.invalidateQueries({ queryKey: ["gear"] }),
    queryClient.invalidateQueries({ queryKey: ["activities"] }),
    queryClient.invalidateQueries({ queryKey: ["activity"] }),
    queryClient.invalidateQueries({ queryKey: ["summary"] })
  ]);
}

function LoginPage() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const session = useQuery({ queryKey: ["session"], queryFn: api.session });
  const login = useMutation({
    mutationFn: ({ username, password }: { username: string; password: string }) => api.login(username, password),
    onSuccess: async (session) => {
      setCsrfToken(session.csrfToken);
      await queryClient.invalidateQueries({ queryKey: ["session"] });
      navigate("/");
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : "Login failed")
  });

  const localLoginEnabled = session.data?.localLoginEnabled !== false;
  const googleOIDCEnabled = session.data?.googleOIDCEnabled === true;
  const callbackError = searchParams.get("error");

  return (
    <div className="login-page">
      <form
        className="login-panel"
        onSubmit={(event) => {
          event.preventDefault();
            login.mutate({ username, password });
        }}
      >
        <div className="brand login-brand">
          <ActivityIcon size={26} />
          <span>Runnarr</span>
        </div>
        {localLoginEnabled && <>
          <label className="field">
            <span>Username</span>
            <input autoFocus type="text" autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} />
          </label>
          <label className="field">
            <span>Password</span>
            <input type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} />
          </label>
          {(error || callbackError) && <div className="error">{error || "Google login was not completed."}</div>}
          <button className="primary-button" type="submit" disabled={login.isPending || username.trim().length === 0 || password.length === 0}>
            Log in
          </button>
        </>}
        {!localLoginEnabled && callbackError && <div className="error">Google login was not completed.</div>}
        {googleOIDCEnabled && <a className="secondary-button" href="/api/auth/google/login">Continue with Google</a>}
        {!localLoginEnabled && !googleOIDCEnabled && <div className="error">No login method is configured.</div>}
      </form>
    </div>
  );
}

function Dashboard() {
  const [filters, setFilters] = useState<ActivityTypeFiltersValue>(emptyActivityTypeFilters);
  const activityTypes = useQuery({ queryKey: ["activity-types"], queryFn: api.activityTypes });
  const summary = useQuery({ queryKey: ["summary", filters], queryFn: () => api.summary(filters) });

  if (summary.isLoading) {
    return <Page title="Dashboard"><LoadingRow /></Page>;
  }
  if (!summary.data) {
    return <Page title="Dashboard"><EmptyState title="No summary available" /></Page>;
  }

  const weekly = (summary.data.weeklyDistance ?? []).map((item) => ({
    week: new Date(item.weekStart).toLocaleDateString(undefined, { month: "short", day: "numeric" }),
    km: Number((item.distanceM / 1000).toFixed(1))
  }));

  return (
    <Page title="Dashboard">
      <ActivityTypeFilterPanel
        activityTypes={activityTypes.data?.activityTypes ?? []}
        filters={filters}
        onChange={setFilters}
      />
      <section className="metric-grid">
        <Metric label="Activities" value={summary.data.activityCount.toLocaleString()} icon={<ActivityIcon size={18} />} />
        <Metric label="Distance" value={formatDistance(summary.data.distanceM)} icon={<RouteIcon size={18} />} />
        <Metric label="Moving Time" value={formatDuration(summary.data.movingTimeS)} icon={<Timer size={18} />} />
        <Metric label="Elevation" value={`${Math.round(summary.data.elevationGainM).toLocaleString()} m`} icon={<Mountain size={18} />} />
      </section>

      <section className="split-layout">
        <div className="panel">
          <div className="panel-heading">Weekly distance</div>
          <div className="chart-area">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={weekly}>
                <CartesianGrid strokeDasharray="3 3" vertical={false} />
                <XAxis dataKey="week" />
                <YAxis width={42} />
                <Tooltip
                  contentStyle={chartTooltipContentStyle}
                  labelStyle={chartTooltipLabelStyle}
                  cursor={chartTooltipCursorStyle}
                  formatter={(value) => [`${value} km`, "Distance"]}
                />
                <Bar dataKey="km" fill="#2f8f83" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </div>

        <div className="panel">
          <div className="panel-heading">Recent activities</div>
          <ActivityTable activities={summary.data.recent ?? []} compact />
        </div>
      </section>
    </Page>
  );
}

function HealthPage() {
  const [range, setRange] = useState(() => healthRangeForLastDays(garminHealthDefaultDays));
  const [draftRange, setDraftRange] = useState(() => healthRangeForLastDays(garminHealthDefaultDays));
  const [syncFrom, setSyncFrom] = useState(range.from);
  const [selectedDate, setSelectedDate] = useState("");
  const queryClient = useQueryClient();
  const garminStatus = useQuery({ queryKey: ["garmin-status"], queryFn: api.garminStatus });
  const jobs = useQuery({ queryKey: ["sync-jobs"], queryFn: api.syncJobs, refetchInterval: 2000 });
  const latestHealthJob = (jobs.data?.jobs ?? []).find((job) => job.provider === "garmin" && job.kind.startsWith("health"));
  const anyGarminSyncRunning = (jobs.data?.jobs ?? []).some((job) => job.provider === "garmin" && job.status === "running");
  const healthSyncRunning = latestHealthJob?.status === "running";
  const dayDetailRef = useRef<HTMLDivElement | null>(null);
  const health = useQuery({
    queryKey: ["health-daily", range],
    queryFn: () => api.healthDaily(range),
    refetchInterval: healthSyncRunning ? 5000 : false
  });
  const garminHealthSync = useMutation({
    mutationFn: api.garminHealthSync,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["sync-jobs"] }),
        queryClient.invalidateQueries({ queryKey: ["health-daily"] })
      ]);
    }
  });
  const metrics = health.data?.metrics ?? [];
  const latestMetric = latestHealthMetric(metrics);
  const selectedMetric = metrics.find((metric) => metric.date === selectedDate);
  const chartData = healthChartData(metrics);
  const showLongRangeHealthLines = healthRangeDayCount(range) > healthBarChartMaxDays;
  const cardItems = healthMetricCards(latestMetric);
  const activePreset = healthRangePresets().find((preset) => healthRangesMatch(draftRange, healthRangeForLastDays(preset.days)));
  const draftRangeChanged = !healthRangesMatch(draftRange, range);
  const draftRangeValid = healthRangeDayCount(draftRange) > 0;
  const syncDisabled = !garminStatus.data?.connected || garminHealthSync.isPending || anyGarminSyncRunning;
  const applyHealthRange = (nextRange: HealthDateRange) => {
    setRange(nextRange);
    setDraftRange(nextRange);
    setSelectedDate("");
  };
  useEffect(() => {
    if (!selectedMetric || !dayDetailRef.current) {
      return;
    }
    dayDetailRef.current.scrollIntoView({ behavior: "smooth", block: "start" });
  }, [selectedMetric?.date]);

  return (
    <Page title="Health">
      <section className="panel health-controls-panel">
        <div className="health-range-controls">
          <div className="segmented-control health-preset-control" role="group" aria-label="Health date range">
            {healthRangePresets().map((preset) => (
              <button
                key={preset.days}
                className={activePreset?.days === preset.days ? "active" : ""}
                type="button"
                onClick={() => applyHealthRange(healthRangeForLastDays(preset.days))}
              >
                {preset.label}
              </button>
            ))}
          </div>
          <div className="date-range-grid health-date-grid">
            <label className="field">
              <span>From</span>
              <input
                type="date"
                value={draftRange.from}
                max={draftRange.to || localDateString()}
                onChange={(event) => setDraftRange({ ...draftRange, from: event.target.value })}
              />
            </label>
            <label className="field">
              <span>To</span>
              <input
                type="date"
                value={draftRange.to}
                min={draftRange.from}
                max={localDateString()}
                onChange={(event) => setDraftRange({ ...draftRange, to: event.target.value })}
              />
            </label>
          </div>
          <div className="health-range-actions">
            <button className="secondary-button small-button" type="button" disabled={!draftRangeChanged} onClick={() => setDraftRange(range)}>
              Reset
            </button>
            <button className="primary-button small-button" type="button" disabled={!draftRangeChanged || !draftRangeValid} onClick={() => applyHealthRange(draftRange)}>
              Apply
            </button>
          </div>
        </div>
        <div className="health-sync-controls">
          <label className="compact-field">
            <span>Sync from</span>
            <input type="date" value={syncFrom} max={localDateString()} onChange={(event) => setSyncFrom(event.target.value)} />
          </label>
          <button
            className="primary-button"
            type="button"
            disabled={syncDisabled}
            onClick={() => garminHealthSync.mutate({ from: syncFrom || undefined, to: localDateString() })}
          >
            <RefreshCw size={16} />
            {healthSyncRunning ? "Syncing" : "Sync health"}
          </button>
        </div>
      </section>

      <SyncProgressCard job={latestHealthJob} />
      {garminHealthSync.error && <div className="error">{garminHealthSync.error instanceof Error ? garminHealthSync.error.message : "Garmin health sync failed"}</div>}
      {health.error && <div className="error">{health.error instanceof Error ? health.error.message : "Could not load health metrics"}</div>}

      {cardItems.length > 0 && (
        <section className="metric-grid">
          {cardItems.map((item) => <Metric key={item.label} label={item.label} value={item.value} icon={item.icon} />)}
        </section>
      )}

      {health.isLoading && <LoadingRow />}
      {!health.isLoading && metrics.length === 0 && (
        <EmptyState
          title="No health metrics found"
          action={garminStatus.data?.connected ? (
            <button className="secondary-button" type="button" disabled={syncDisabled} onClick={() => garminHealthSync.mutate({ from: syncFrom || undefined, to: localDateString() })}>
              <RefreshCw size={16} />
              Sync health
            </button>
          ) : (
            <Link className="secondary-button" to="/settings">Connect Garmin</Link>
          )}
        />
      )}

      {metrics.length > 0 && (
        <>
          <section className="health-chart-grid">
            <HealthBarChart title="Steps" data={chartData} dataKey="steps" color="#2f8f83" formatter={formatHealthInteger} axisFormatter={formatHealthAxisInteger} asLine={showLongRangeHealthLines} />
            <HealthCaloriesChart data={chartData} asLine={showLongRangeHealthLines} />
            <HealthBarChart title="Sleep" data={chartData} dataKey="sleepHours" color="#4664c9" formatter={(value) => `${value.toFixed(1)} h`} axisFormatter={formatHealthAxisHours} asLine={showLongRangeHealthLines} />
            <HealthLineChart title="Resting heart rate" data={chartData} dataKey="restingHeartRate" color="#c84d4d" formatter={(value) => `${Math.round(value)} bpm`} axisFormatter={formatHealthAxisBPM} />
            <HealthLineChart title="Stress" data={chartData} dataKey="stress" color="#7a4eb2" formatter={(value) => Math.round(value).toLocaleString()} />
            <HealthBodyBatteryChart data={chartData} asLine={showLongRangeHealthLines} />
            <HealthLineChart title="HRV" data={chartData} dataKey="hrv" color="#6f8f2f" formatter={(value) => `${Math.round(value)} ms`} axisFormatter={formatHealthAxisMS} />
            <HealthWeightChart data={chartData} />
          </section>

          <section className="panel">
            <div className="filter-header">
              <div className="panel-heading">Daily metrics</div>
              {selectedMetric && (
                <button className="secondary-button small-button" type="button" onClick={() => setSelectedDate("")}>
                  Clear selection
                </button>
              )}
            </div>
            <HealthMetricsTable
              metrics={metrics}
              selectedDate={selectedDate}
              onSelect={(date) => setSelectedDate((current) => current === date ? "" : date)}
            />
          </section>

          {selectedMetric && (
            <div ref={dayDetailRef}>
              <HealthDayDetail metric={selectedMetric} />
            </div>
          )}
        </>
      )}
    </Page>
  );
}

function ToolsPage() {
  const [distanceKm, setDistanceKm] = useState("");
  const [time, setTime] = useState("");
  const [pace, setPace] = useState("");
  const [result, setResult] = useState<ToolsPaceResponse>();
  const [vdotDistanceKm, setVdotDistanceKm] = useState("");
  const [vdotTime, setVdotTime] = useState("");
  const [vdotResult, setVdotResult] = useState<ToolsVdotResponse>();
  const [error, setError] = useState("");
  const [vdotError, setVdotError] = useState("");
  const [vdotDistancePresetId, setVdotDistancePresetId] = useState("");
  const calculatePace = useMutation({
    mutationFn: api.toolsPace,
    onSuccess: (payload) => {
      setResult(payload);
      setError("");
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : "Could not calculate pace values")
  });
  const calculateVdot = useMutation({
    mutationFn: api.toolsVDOT,
    onSuccess: (payload) => {
      setVdotResult(payload);
      setVdotError("");
    },
    onError: (err) => setVdotError(err instanceof ApiError ? err.message : "Could not calculate VDOT")
  });
  const filledInputs = [distanceKm, time, pace].filter((value) => value.trim().length > 0).length;
  const canSubmit = filledInputs === 2;
  const canSubmitVDOT = vdotDistanceKm.trim().length > 0 && vdotTime.trim().length > 0;

  const clearForm = () => {
    setDistanceKm("");
    setTime("");
    setPace("");
    setResult(undefined);
    setError("");
  };
  const clearVdotForm = () => {
    setVdotDistanceKm("");
    setVdotTime("");
    setVdotDistancePresetId("");
    setVdotResult(undefined);
    setVdotError("");
  };
  const setVdotDistancePreset = (presetId: string, distanceKm: string) => {
    setVdotDistancePresetId(presetId);
    setVdotDistanceKm(distanceKm);
  };
  const clearDistancePreset = () => setVdotDistancePresetId("");

  return (
    <Page title="Tools">
      <section className="panel">
        <div className="panel-heading">Pace calculator</div>
        <p className="tools-help-text">
          Fill in exactly two fields and submit to compute the missing one. Distance is in km, time is MM:SS or HH:MM:SS, pace is MM:SS /km.
        </p>
        <div className="tools-section-spacer" />
        <form
          className="tools-form"
          onSubmit={(event) => {
            event.preventDefault();
            setError("");
            calculatePace.mutate({
              distanceKm: distanceKm.trim(),
              time: time.trim(),
              pace: pace.trim()
            });
          }}
        >
          <div className="tools-form-grid">
            <label className="field">
              <span>Distance</span>
              <input
                type="number"
                step="0.001"
                min="0"
                value={distanceKm}
                placeholder="10.0"
                onChange={(event) => setDistanceKm(event.target.value)}
              />
            </label>
            <label className="field">
              <span>Time</span>
              <input
                type="text"
                value={time}
                placeholder="45:00 or 1:45:00"
                onChange={(event) => setTime(event.target.value)}
              />
            </label>
            <label className="field">
              <span>Pace</span>
              <input
                type="text"
                value={pace}
                placeholder="4:30"
                onChange={(event) => setPace(event.target.value)}
              />
            </label>
          </div>
          <div className="tools-form-actions">
            <button className="secondary-button small-button" type="button" onClick={clearForm}>
              Clear
            </button>
            <button className="primary-button" type="submit" disabled={!canSubmit || calculatePace.isPending}>
              {calculatePace.isPending ? "Calculating..." : "Calculate"}
            </button>
          </div>
        </form>
        {error && <div className="error">{error}</div>}
      </section>

      {result && (
        <section className="panel">
          <div className="panel-heading">Result</div>
          <section className="tools-result-grid">
            <Metric label="Distance" value={result.distanceLabel} icon={<RouteIcon size={18} />} />
            <Metric label="Time" value={result.timeLabel} icon={<Timer size={18} />} />
            <Metric label="Pace" value={result.paceLabel} icon={<Scale size={18} />} />
          </section>
        </section>
      )}

      <section className="panel">
        <div className="panel-heading">VDOT calculator</div>
        <p className="tools-help-text">
          Enter a race distance and finishing time to estimate your VDOT and predicted times for common race distances.
        </p>
        <div className="tools-section-spacer" />
        <div className="tools-preset-list">
          {vdotDistancePresets.map((preset) => (
            <button
              type="button"
              key={preset.id}
              className={`filter-chip ${vdotDistancePresetId === preset.id ? "active" : ""}`}
              onClick={() => setVdotDistancePreset(preset.id, preset.distanceKm)}
            >
              {preset.label}
            </button>
          ))}
        </div>
        <div className="tools-preset-spacer" />
        <form
          className="tools-form"
          onSubmit={(event) => {
            event.preventDefault();
            setVdotError("");
            calculateVdot.mutate({
              distanceKm: vdotDistanceKm.trim(),
              time: vdotTime.trim()
            });
          }}
        >
          <div className="tools-form-grid">
            <label className="field">
              <span>Distance</span>
              <input
                type="number"
                step="0.001"
                min="0"
                value={vdotDistanceKm}
                placeholder="10.0"
                onChange={(event) => {
                  clearDistancePreset();
                  setVdotDistanceKm(event.target.value);
                }}
              />
            </label>
            <label className="field">
              <span>Time</span>
              <input
                type="text"
                value={vdotTime}
                placeholder="40:00 or 1:40:00"
                onChange={(event) => setVdotTime(event.target.value)}
              />
            </label>
            <div className="tools-form-spacer" />
          </div>
          <div className="tools-form-actions">
            <button className="secondary-button small-button" type="button" onClick={clearVdotForm}>
              Clear
            </button>
            <button className="primary-button" type="submit" disabled={!canSubmitVDOT || calculateVdot.isPending}>
              {calculateVdot.isPending ? "Calculating..." : "Calculate"}
            </button>
          </div>
        </form>
        {vdotError && <div className="error">{vdotError}</div>}
      </section>

      {vdotResult && (
        <section className="panel">
          <div className="panel-heading">VDOT result</div>
          <section className="tools-result-grid">
            <Metric label="Distance" value={vdotResult.distanceLabel} icon={<RouteIcon size={18} />} />
            <Metric label="Time" value={vdotResult.timeLabel} icon={<Timer size={18} />} />
            <Metric label="VDOT" value={vdotResult.vdotLabel} icon={<Flame size={18} />} />
          </section>
          <p className="tools-help-text tools-result-subtitle">Equivalent race predictions</p>
          <section className="tools-equivalent-grid">
            {vdotResult.equivalents.map((equivalent) => (
              <Metric key={equivalent.race} label={equivalent.race} value={equivalent.timeLabel} icon={<RouteIcon size={18} />} />
            ))}
          </section>
        </section>
      )}
    </Page>
  );
}

function HealthBarChart({
  title,
  data,
  dataKey,
  color,
  formatter,
  axisFormatter = formatHealthAxisInteger,
  asLine = false
}: {
  title: string;
  data: HealthChartPoint[];
  dataKey: keyof HealthChartPoint;
  color: string;
  formatter: (value: number) => string;
  axisFormatter?: (value: number) => string;
  asLine?: boolean;
}) {
  if (!data.some((item) => isFiniteNumber(item[dataKey]))) {
    return null;
  }
  if (asLine) {
    return (
      <div className="panel">
        <div className="panel-heading">{title}</div>
        <div className="health-chart-area">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={data}>
              <CartesianGrid strokeDasharray="3 3" vertical={false} />
              <XAxis dataKey="label" minTickGap={18} />
              <YAxis width={HEALTH_CHART_Y_AXIS_WIDTH} tickFormatter={axisFormatter} />
              <Tooltip
                contentStyle={chartTooltipContentStyle}
                labelStyle={chartTooltipLabelStyle}
                formatter={(value) => [formatter(Number(value)), title]}
              />
              <Line type="monotone" dataKey={dataKey} stroke={color} strokeWidth={2} dot={false} connectNulls />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </div>
    );
  }
  return (
    <div className="panel">
      <div className="panel-heading">{title}</div>
      <div className="health-chart-area">
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={data}>
            <CartesianGrid strokeDasharray="3 3" vertical={false} />
            <XAxis dataKey="label" />
            <YAxis width={HEALTH_CHART_Y_AXIS_WIDTH} tickFormatter={axisFormatter} />
            <Tooltip
              contentStyle={chartTooltipContentStyle}
              labelStyle={chartTooltipLabelStyle}
              cursor={chartTooltipCursorStyle}
              formatter={(value) => [formatter(Number(value)), title]}
            />
            <Bar dataKey={dataKey} fill={color} radius={[4, 4, 0, 0]} />
          </BarChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}

function HealthCaloriesChart({ data, asLine = false }: { data: HealthChartPoint[]; asLine?: boolean }) {
  if (!data.some((item) => isFiniteNumber(item.activeCalories) || isFiniteNumber(item.totalCalories) || isFiniteNumber(item.remainingCalories))) {
    return null;
  }
  if (asLine) {
    return (
      <div className="panel">
        <div className="chart-header">
          <div className="panel-heading">Calories</div>
          <div className="health-chart-legend" aria-label="Calories series">
            <span><i style={{ background: "#b7791f" }} /> Active</span>
            <span><i style={{ background: "#4664c9" }} /> Total</span>
          </div>
        </div>
        <div className="health-chart-area">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={data}>
              <CartesianGrid strokeDasharray="3 3" vertical={false} />
              <XAxis dataKey="label" minTickGap={18} />
              <YAxis width={HEALTH_CHART_Y_AXIS_WIDTH} tickFormatter={formatHealthAxisInteger} />
              <Tooltip
                contentStyle={chartTooltipContentStyle}
                labelStyle={chartTooltipLabelStyle}
                formatter={(value, name) => [formatHealthCalories(Number(value)), String(name)]}
              />
              <Line type="monotone" dataKey="activeCalories" name="Active" stroke="#b7791f" strokeWidth={2} dot={false} connectNulls />
              <Line type="monotone" dataKey="totalCalories" name="Total" stroke="#4664c9" strokeWidth={2} dot={false} connectNulls />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </div>
    );
  }
  return (
    <div className="panel">
      <div className="panel-heading">Calories</div>
      <div className="health-chart-area">
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={data}>
            <CartesianGrid strokeDasharray="3 3" vertical={false} />
            <XAxis dataKey="label" />
            <YAxis width={HEALTH_CHART_Y_AXIS_WIDTH} tickFormatter={formatHealthAxisInteger} />
            <Tooltip
              contentStyle={chartTooltipContentStyle}
              labelStyle={chartTooltipLabelStyle}
              cursor={chartTooltipCursorStyle}
              formatter={(value, name, item) => formatCaloriesTooltipItem(value, name, item)}
            />
            <Bar dataKey="activeCalories" name="Active" stackId="calories" fill="#b7791f" />
            <Bar dataKey="remainingCalories" name="Remaining" stackId="calories" fill="#4664c9" radius={[4, 4, 0, 0]} />
          </BarChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}

function formatCaloriesTooltipItem(value: unknown, name: unknown, item: unknown) {
  if (String(name) !== "Remaining") {
    return [formatHealthCalories(Number(value)), String(name)];
  }
  const payload = healthTooltipPayload(item);
  const active = finiteValue(payload?.activeCalories) ?? 0;
  const remaining = finiteValue(Number(value)) ?? 0;
  return [formatHealthCalories(active + remaining), "Total"];
}

function healthTooltipPayload(item: unknown): HealthChartPoint | undefined {
  if (!item || typeof item !== "object" || !("payload" in item)) {
    return undefined;
  }
  const payload = (item as { payload?: HealthChartPoint }).payload;
  return payload && typeof payload === "object" ? payload : undefined;
}

function HealthLineChart({
  title,
  data,
  dataKey,
  color,
  formatter,
  axisFormatter = formatHealthAxisInteger
}: {
  title: string;
  data: HealthChartPoint[];
  dataKey: keyof HealthChartPoint;
  color: string;
  formatter: (value: number) => string;
  axisFormatter?: (value: number) => string;
}) {
  if (!data.some((item) => isFiniteNumber(item[dataKey]))) {
    return null;
  }
  return (
    <div className="panel">
      <div className="panel-heading">{title}</div>
      <div className="health-chart-area">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={data}>
            <CartesianGrid strokeDasharray="3 3" vertical={false} />
            <XAxis dataKey="label" />
            <YAxis width={HEALTH_CHART_Y_AXIS_WIDTH} tickFormatter={axisFormatter} />
            <Tooltip
              contentStyle={chartTooltipContentStyle}
              labelStyle={chartTooltipLabelStyle}
              formatter={(value) => [formatter(Number(value)), title]}
            />
            <Line type="monotone" dataKey={dataKey} stroke={color} strokeWidth={2} dot={false} connectNulls />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}

function HealthWeightChart({ data }: { data: HealthChartPoint[] }) {
  const points = data.filter((item): item is HealthChartPoint & { weight: number } => isFiniteNumber(item.weight));
  if (points.length === 0) {
    return null;
  }
  const measurementLabel = points.length === 1 ? "1 measurement" : `${points.length.toLocaleString()} measurements`;
  return (
    <div className="panel">
      <div className="chart-header">
        <div className="panel-heading">Weight</div>
        <div className="muted">{measurementLabel}</div>
      </div>
      <div className="health-chart-area">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={points}>
            <CartesianGrid strokeDasharray="3 3" vertical={false} />
            <XAxis dataKey="label" minTickGap={18} />
            <YAxis width={46} domain={weightYAxisDomain(points)} tickFormatter={(value) => Number(value).toFixed(1)} />
            <Tooltip
              contentStyle={chartTooltipContentStyle}
              labelStyle={chartTooltipLabelStyle}
              formatter={(value) => [formatHealthWeight(Number(value)), "Weight"]}
            />
            <Line
              type="monotone"
              dataKey="weight"
              stroke="#8b5e3c"
              strokeWidth={2}
              dot={{ r: 4, strokeWidth: 2 }}
              activeDot={{ r: 6 }}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}

function weightYAxisDomain(points: Array<HealthChartPoint & { weight: number }>): [number, number] {
  const values = points.map((point) => point.weight);
  const min = Math.min(...values);
  const max = Math.max(...values);
  const padding = min === max ? 1 : Math.max(0.5, (max - min) * 0.25);
  return [
    Math.max(0, Math.floor((min - padding) * 10) / 10),
    Math.ceil((max + padding) * 10) / 10
  ];
}

function HealthBodyBatteryChart({ data, asLine = false }: { data: HealthChartPoint[]; asLine?: boolean }) {
  const [hoveredPoint, setHoveredPoint] = useState<{ point: HealthChartPoint; x: number; y: number }>();
  const points = data.filter((item) => isFiniteNumber(item.bodyBatteryGained) || isFiniteNumber(item.bodyBatteryDrained) || isFiniteNumber(item.bodyBatteryDrainedLoss) || isFiniteNumber(item.bodyBatteryHighest));
  if (points.length === 0) {
    return null;
  }
  if (asLine) {
    return (
      <div className="panel">
        <div className="chart-header">
          <div className="panel-heading">Body battery</div>
          <div className="health-chart-legend" aria-label="Body battery series">
            <span><i style={{ background: "#2f8f83" }} /> Gained</span>
            <span><i style={{ background: "#c84d4d" }} /> Drained</span>
            <span><i style={{ background: "#b7791f" }} /> Highest</span>
          </div>
        </div>
        <div className="health-chart-area">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={points}>
              <CartesianGrid strokeDasharray="3 3" vertical={false} />
              <XAxis dataKey="label" minTickGap={18} />
              <YAxis width={42} />
              <Tooltip
                contentStyle={chartTooltipContentStyle}
                labelStyle={chartTooltipLabelStyle}
                formatter={(value, name) => [formatHealthRounded(Number(value)), String(name)]}
              />
              <Line type="monotone" dataKey="bodyBatteryGained" name="Gained" stroke="#2f8f83" strokeWidth={2} dot={false} connectNulls />
              <Line type="monotone" dataKey="bodyBatteryDrained" name="Drained" stroke="#c84d4d" strokeWidth={2} dot={false} connectNulls />
              <Line type="monotone" dataKey="bodyBatteryHighest" name="Highest" stroke="#b7791f" strokeWidth={2} dot={false} connectNulls />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </div>
    );
  }
  const width = 640;
  const height = 220;
  const margin = { top: 10, right: 18, bottom: 34, left: 42 };
  const plotWidth = width - margin.left - margin.right;
  const plotHeight = height - margin.top - margin.bottom;
  const maxMagnitude = bodyBatteryMagnitude(points);
  const yTicks = [maxMagnitude, maxMagnitude / 2, 0, -maxMagnitude / 2, -maxMagnitude];
  const labelEvery = Math.max(1, Math.ceil(points.length / 6));
  const highestPoints = points
    .map((point, index) => ({ point, x: bodyBatteryChartX(index, points.length, margin.left, plotWidth) }))
    .filter((item): item is { point: HealthChartPoint & { bodyBatteryHighest: number }; x: number } => isFiniteNumber(item.point.bodyBatteryHighest));
  const highestPath = highestPoints.map((item, index) => `${index === 0 ? "M" : "L"} ${item.x} ${bodyBatteryChartY(item.point.bodyBatteryHighest, maxMagnitude, margin.top, plotHeight)}`).join(" ");

  return (
    <div className="panel">
      <div className="chart-header">
        <div className="panel-heading">Body battery</div>
        <div className="health-chart-legend" aria-label="Body battery series">
          <span><i style={{ background: "#2f8f83" }} /> Gained</span>
          <span><i style={{ background: "#c84d4d" }} /> Drained</span>
          <span><i style={{ background: "#b7791f" }} /> Highest</span>
        </div>
      </div>
      <div className="health-chart-area body-battery-chart-wrap" onMouseLeave={() => setHoveredPoint(undefined)}>
        <svg className="body-battery-chart" viewBox={`0 0 ${width} ${height}`} role="img" aria-label="Body battery gained, drained, and highest by day">
          {yTicks.map((tick) => {
            const y = bodyBatteryChartY(tick, maxMagnitude, margin.top, plotHeight);
            return (
              <g key={tick}>
                <line className="body-battery-grid-line" x1={margin.left} x2={width - margin.right} y1={y} y2={y} />
                <text className="body-battery-axis-label" x={margin.left - 8} y={y + 4} textAnchor="end">{Math.round(tick)}</text>
              </g>
            );
          })}
          {highestPath && <path className="body-battery-highest-line" d={highestPath} />}
          {points.map((point, index) => {
            const x = bodyBatteryChartX(index, points.length, margin.left, plotWidth);
            const zeroY = bodyBatteryChartY(0, maxMagnitude, margin.top, plotHeight);
            const gainedY = isFiniteNumber(point.bodyBatteryGained) ? bodyBatteryChartY(point.bodyBatteryGained, maxMagnitude, margin.top, plotHeight) : undefined;
            const drainedValue = isFiniteNumber(point.bodyBatteryDrainedLoss) ? point.bodyBatteryDrainedLoss : undefined;
            const drainedY = drainedValue !== undefined ? bodyBatteryChartY(drainedValue, maxMagnitude, margin.top, plotHeight) : undefined;
            const highestY = isFiniteNumber(point.bodyBatteryHighest) ? bodyBatteryChartY(point.bodyBatteryHighest, maxMagnitude, margin.top, plotHeight) : undefined;
            const showLabel = index === 0 || index === points.length - 1 || index % labelEvery === 0;
            const tooltipY = Math.min(gainedY ?? zeroY, highestY ?? zeroY);
            return (
              <g key={point.date}>
                <rect
                  className="body-battery-hit-area"
                  x={x - 10}
                  y={margin.top}
                  width={20}
                  height={plotHeight}
                  onMouseEnter={() => setHoveredPoint({ point, x: (x / width) * 100, y: (tooltipY / height) * 100 })}
                  onMouseMove={() => setHoveredPoint({ point, x: (x / width) * 100, y: (tooltipY / height) * 100 })}
                  onFocus={() => setHoveredPoint({ point, x: (x / width) * 100, y: (tooltipY / height) * 100 })}
                  tabIndex={0}
                  aria-label={bodyBatteryTooltipText(point)}
                />
                {gainedY !== undefined && (
                  <rect className="body-battery-bar gained" x={x - 6} y={gainedY} width={12} height={Math.max(1, zeroY - gainedY)} rx={3} />
                )}
                {drainedY !== undefined && (
                  <rect className="body-battery-bar drained" x={x - 6} y={zeroY} width={12} height={Math.max(1, drainedY - zeroY)} rx={3} />
                )}
                {highestY !== undefined && <circle className="body-battery-highest-dot" cx={x} cy={highestY} r={3.5} />}
                {showLabel && (
                  <text className="body-battery-axis-label" x={x} y={height - 10} textAnchor="middle">{healthChartLabel(point.date)}</text>
                )}
              </g>
            );
          })}
        </svg>
        {hoveredPoint && (
          <div className="body-battery-tooltip" style={{ left: `${hoveredPoint.x}%`, top: `${hoveredPoint.y}%` }}>
            <strong>{formatHealthDate(hoveredPoint.point.date)}</strong>
            <span>Gained {formatHealthRounded(hoveredPoint.point.bodyBatteryGained)}</span>
            <span>Drained {formatHealthRounded(Math.abs(hoveredPoint.point.bodyBatteryDrainedLoss ?? 0))}</span>
            <span>Highest {formatHealthRounded(hoveredPoint.point.bodyBatteryHighest)}</span>
          </div>
        )}
      </div>
    </div>
  );
}

function bodyBatteryMagnitude(points: HealthChartPoint[]) {
  const maxValue = points.reduce((max, point) => Math.max(
    max,
    point.bodyBatteryGained ?? 0,
    Math.abs(point.bodyBatteryDrainedLoss ?? 0),
    point.bodyBatteryHighest ?? 0
  ), 100);
  return Math.ceil(maxValue / 25) * 25;
}

function bodyBatteryChartX(index: number, count: number, left: number, width: number) {
  if (count <= 1) {
    return left + width / 2;
  }
  return left + (index / (count - 1)) * width;
}

function bodyBatteryChartY(value: number, magnitude: number, top: number, height: number) {
  const clamped = Math.max(-magnitude, Math.min(magnitude, value));
  return top + ((magnitude - clamped) / (magnitude * 2)) * height;
}

function bodyBatteryTooltipText(point: HealthChartPoint) {
  return [
    formatHealthDate(point.date),
    `Gained: ${formatHealthRounded(point.bodyBatteryGained)}`,
    `Drained: ${formatHealthRounded(Math.abs(point.bodyBatteryDrainedLoss ?? 0))}`,
    `Highest: ${formatHealthRounded(point.bodyBatteryHighest)}`
  ].join("\n");
}

function HealthMetricsTable({
  metrics,
  selectedDate,
  onSelect
}: {
  metrics: DailyHealthMetric[];
  selectedDate: string;
  onSelect: (date: string) => void;
}) {
  return (
    <div className="table-wrap">
      <table className="data-table health-table">
        <thead>
          <tr>
            <th>Date</th>
            <th>Steps</th>
            <th>Calories</th>
            <th>Sleep</th>
            <th>RHR</th>
            <th>Stress</th>
            <th>Body battery</th>
            <th>HRV</th>
            <th>Weight</th>
          </tr>
        </thead>
        <tbody>
          {[...metrics].reverse().map((metric) => (
            <tr key={metric.date} className={selectedDate === metric.date ? "selected-row" : ""}>
              <td>
                <button className="table-button" type="button" onClick={() => onSelect(metric.date)}>
                  {formatHealthDate(metric.date)}
                </button>
              </td>
              <td>{formatHealthInteger(metric.steps)}</td>
              <td>{formatHealthCalories(metric.totalCaloriesKcal ?? metric.activeCaloriesKcal)}</td>
              <td>{formatHealthDuration(metric.sleepDurationS)}</td>
              <td>{formatHealthBPM(metric.restingHeartRateBpm)}</td>
              <td>{formatHealthRounded(metric.stressAvg)}</td>
              <td>{formatBodyBatteryGainDrain(metric)}</td>
              <td>{formatHealthMS(metric.hrvAvgMs)}</td>
              <td>{formatHealthWeight(metric.weightKg)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function HealthDayDetail({ metric }: { metric: DailyHealthMetric }) {
  const items = healthDetailItems(metric);
  if (items.length === 0) {
    return null;
  }
  return (
    <section className="panel health-detail-panel">
      <div>
        <div className="panel-heading">{formatHealthDate(metric.date)}</div>
        <div className="health-detail-grid">
          {items.map((item) => (
            <div className="health-detail-item" key={item.label}>
              <span>{item.label}</span>
              <strong>{item.value}</strong>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

function GearPage() {
  const queryClient = useQueryClient();
  const session = useQuery({ queryKey: ["session"], queryFn: api.session });
  const userID = session.data?.user?.id;
  const preferences = useQuery({
    queryKey: preferencesQueryKey(userID),
    queryFn: api.preferences,
    enabled: Boolean(userID)
  });
  const savePreferences = useSaveUserPreferences(userID);
  const gears = useQuery({ queryKey: ["gears"], queryFn: api.gears });
  const garminStatus = useQuery({ queryKey: ["garmin-status"], queryFn: api.garminStatus });
  const jobs = useQuery({ queryKey: ["sync-jobs"], queryFn: api.syncJobs, refetchInterval: 2000 });
  const latestGearJob = (jobs.data?.jobs ?? []).find((job) => job.provider === "garmin" && isGearSyncJob(job));
  const anyGarminSyncRunning = (jobs.data?.jobs ?? []).some((job) => job.provider === "garmin" && job.status === "running");
  const gearSyncRunning = latestGearJob?.status === "running";
  const gearSync = useMutation({
    mutationFn: api.garminGearSync,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["sync-jobs"] }),
        queryClient.invalidateQueries({ queryKey: ["gears"] }),
        queryClient.invalidateQueries({ queryKey: ["gear"] }),
        queryClient.invalidateQueries({ queryKey: ["activities"] }),
        queryClient.invalidateQueries({ queryKey: ["summary"] })
      ]);
    }
  });
  const [gearSortBy, setGearSortBy] = useState<GearSortBy>(defaultGearSortBy);
  const activeGear = gears.data?.active ?? [];
  const retiredGear = gears.data?.retired ?? [];
  const allGear = gears.data?.gear ?? [];
  const sortedActiveGear = sortGears(activeGear, gearSortBy);
  const sortedRetiredGear = sortGears(retiredGear, gearSortBy);
  const syncDisabled = !garminStatus.data?.connected || gearSync.isPending || anyGarminSyncRunning;

  useEffect(() => {
    setGearSortBy(normalizeGearSortBy(preferences.data?.gearSortBy));
  }, [preferences.data?.gearSortBy, userID]);

  useEffect(() => {
    if (!latestGearJob || latestGearJob.status === "running") {
      return;
    }
    void invalidateGearRelatedQueries(queryClient);
  }, [latestGearJob?.id, latestGearJob?.status, queryClient]);

  return (
        <Page
      title="Gear"
      actions={
        <>
          <label className="compact-field gear-sort-control" htmlFor="gear-sort-by">
            <span>Sort by</span>
            <select
              id="gear-sort-by"
              value={gearSortBy}
              onChange={(event) => {
                const next = event.target.value as GearSortBy;
                setGearSortBy(next);
                if (userID) {
                  savePreferences.mutate({ gearSortBy: next });
                }
              }}
            >
              {gearSortByOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </label>
          <button className="primary-button" type="button" disabled={syncDisabled} onClick={() => gearSync.mutate()}>
            <RefreshCw size={16} />
            {gearSyncRunning ? "Syncing" : "Sync gear"}
          </button>
        </>
      }
    >
      <SyncProgressCard job={latestGearJob} />
      {gearSync.error && <div className="error">{gearSync.error instanceof Error ? gearSync.error.message : "Garmin gear sync failed"}</div>}
      {gears.error && <div className="error">{gears.error instanceof Error ? gears.error.message : "Could not load gear"}</div>}
      {gears.isLoading && <LoadingRow />}
      {!gears.isLoading && allGear.length === 0 && (
        <EmptyState
          title="No gear found"
          action={garminStatus.data?.connected ? (
            <button className="secondary-button" type="button" disabled={syncDisabled} onClick={() => gearSync.mutate()}>
              <RefreshCw size={16} />
              Sync gear
            </button>
          ) : (
            <Link className="secondary-button" to="/settings">Connect Garmin</Link>
          )}
        />
      )}
      {sortedActiveGear.length > 0 && <GearSection title="Active gear" gear={sortedActiveGear} />}
      {sortedRetiredGear.length > 0 && <GearSection title="Retired gear" gear={sortedRetiredGear} retired />}
    </Page>
  );
}

function GearSection({ title, gear, retired = false }: { title: string; gear: Gear[]; retired?: boolean }) {
  return (
    <section className="panel gear-section">
      <div className="filter-header">
        <div className="panel-heading">{title}</div>
        <span className="muted">{gear.length.toLocaleString()}</span>
      </div>
      <div className="gear-grid">
        {gear.map((item) => <GearCard key={item.id} gear={item} retired={retired} />)}
      </div>
    </section>
  );
}

function GearCard({ gear, retired = false }: { gear: Gear; retired?: boolean }) {
  const subtitle = gearSubtitle(gear);
  return (
    <Link className={`gear-card${retired ? " retired" : ""}`} to={`/gear/${gear.id}`}>
      <div className="gear-card-header">
        <strong>{gearDisplayName(gear)}</strong>
        <span className="source-pill">{formatGearType(gear.gearType)}</span>
      </div>
      {subtitle && <div className="gear-meta">{subtitle}</div>}
      <GearDistanceBlock gear={gear} />
    </Link>
  );
}

function GearDistanceBlock({ gear }: { gear: Gear }) {
  if (!isFiniteNumber(gear.totalDistanceM)) {
    return null;
  }
  const total = gear.totalDistanceM;
  const max = isFiniteNumber(gear.maxDistanceM) && gear.maxDistanceM > 0 ? gear.maxDistanceM : undefined;
  const usagePercent = gearDistanceUsagePercentRaw(total, max);
  const usagePercentLabel = gearDistanceUsagePercent(total, max);
  return (
    <div className="gear-distance-block">
      <div className="gear-distance-label">
        <span>Total distance</span>
        <strong>{formatGearDistance(total)}</strong>
      </div>
      <div className="gear-distance-meta">
        <span>Activities</span>
        <strong>{formatGearActivityCount(gear.activityCount)}</strong>
      </div>
      {max && (
        <>
          <div className="gear-progress" aria-label={`Gear distance ${usagePercentLabel}`}>
            <span style={{ width: `${usagePercent}%` }} />
          </div>
          <div className="gear-progress-label">{formatGearDistance(total)} of {formatGearDistance(max)} · {usagePercentLabel}</div>
        </>
      )}
    </div>
  );
}

function GearChipList({ gear, compact = false }: { gear?: GearSummary[]; compact?: boolean }) {
  const items = gear ?? [];
  if (items.length === 0) {
    return null;
  }
  return (
    <div className={`gear-chip-list${compact ? " compact" : ""}`}>
      {items.map((item) => (
        <Link className={`gear-chip${item.retired ? " retired" : ""}`} key={item.id} to={`/gear/${item.id}`} title={gearDisplayLabel(item)}>
          <Footprints size={13} />
          <span>{gearDisplayLabel(item)}</span>
        </Link>
      ))}
    </div>
  );
}

function GearDetailPage() {
  const { id } = useParams();
  const gear = useQuery({ queryKey: ["gear", id], queryFn: () => api.gear(id!), enabled: Boolean(id) });

  if (gear.isLoading) {
    return <Page title="Gear"><LoadingRow /></Page>;
  }
  if (gear.error) {
    return <Page title="Gear"><div className="error">{gear.error instanceof Error ? gear.error.message : "Could not load gear"}</div></Page>;
  }
  if (!gear.data) {
    return <Page title="Gear"><EmptyState title="Gear not found" /></Page>;
  }

  const item = gear.data.gear;
  const activities = gear.data.activities ?? [];
  const detailItems = gearDetailItems(item);
  return (
    <Page
      title={gearDisplayName(item)}
      eyebrow={`${formatGearType(item.gearType)} · ${item.retired ? "Retired" : "Active"}`}
      actions={<Link className="secondary-button" to="/gear"><ChevronLeft size={16} /> All gear</Link>}
    >
      <section className="metric-grid">
        {isFiniteNumber(item.totalDistanceM) && <Metric label="Distance" value={formatGearDistance(item.totalDistanceM)} />}
        <Metric label="Activities" value={activities.length.toLocaleString()} />
        <Metric label="Type" value={formatGearType(item.gearType)} />
        <Metric label="Status" value={item.retired ? "Retired" : "Active"} />
      </section>

      {detailItems.length > 0 && (
        <section className="panel gear-detail-panel">
          <div className="panel-heading">Details</div>
          <div className="gear-detail-grid">
            {detailItems.map((detail) => (
              <div className="gear-detail-item" key={detail.label}>
                <span>{detail.label}</span>
                <strong>{detail.value}</strong>
              </div>
            ))}
          </div>
        </section>
      )}

      <section className="panel">
        <div className="filter-header">
          <div className="panel-heading">Assigned activities</div>
          <span className="muted">{activities.length.toLocaleString()}</span>
        </div>
        {activities.length > 0 ? <ActivityTable activities={activities} /> : <EmptyState title="No local activities assigned" />}
      </section>
    </Page>
  );
}

function ActivitiesPage() {
  const session = useQuery({ queryKey: ["session"], queryFn: api.session });
  const userID = session.data?.user?.id;
  const preferences = useQuery({
    queryKey: preferencesQueryKey(userID),
    queryFn: api.preferences,
    enabled: Boolean(userID)
  });
  const savePreferences = useSaveUserPreferences(userID);
  const [searchParams, setSearchParams] = useSearchParams();
  const filters = activityFiltersFromSearchParams(searchParams);
  const setFilters = (nextFilters: ActivityTypeFiltersValue) => {
    setSearchParams(activityFiltersToSearchParams(nextFilters), { replace: true });
  };
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [sortOpen, setSortOpen] = useState(false);
  const [columnsOpen, setColumnsOpen] = useState(false);
  const [visibleColumns, setVisibleColumns] = useState<ActivityTableColumnKey[]>(defaultActivityTableColumns);
  const activityTypes = useQuery({ queryKey: ["activity-types"], queryFn: api.activityTypes });
  const activities = useInfiniteQuery({
    queryKey: ["activities", filters],
    initialPageParam: 0,
    queryFn: ({ pageParam }) => api.activities(filters, { limit: ACTIVITY_LIST_PAGE_SIZE, offset: pageParam }),
    getNextPageParam: (lastPage) => (lastPage.hasMore ? lastPage.nextOffset : undefined)
  });
  const queryClient = useQueryClient();
  const deleteActivity = useMutation({
    mutationFn: api.deleteActivity,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["activities"] }),
        queryClient.invalidateQueries({ queryKey: ["summary"] }),
        queryClient.invalidateQueries({ queryKey: ["activity-types"] })
      ]);
    }
  });
  const handleDelete = (activity: Activity) => {
    if (window.confirm(deleteActivityConfirmation(activity))) {
      deleteActivity.mutate(activity.id);
    }
  };
  const activityList = activities.data?.pages.flatMap((page) => page.activities ?? []) ?? [];
  const activitiesLoaded = Boolean(activities.data);
  const dateFiltersActive = hasDateFilters(filters);
  const anyFiltersActive = hasActivityFilters(filters);
  const currentSort = normalizedActivitySort(filters);
  const sortActive = !activitySortsMatch(currentSort, defaultActivitySort);
  const hiddenColumnCount = defaultActivityTableColumns.length - visibleColumns.length;
  useEffect(() => {
    setVisibleColumns(normalizeActivityTableColumns(preferences.data?.activityTableColumns));
  }, [preferences.data?.activityTableColumns, userID]);

  const applyColumns = (columns: ActivityTableColumnKey[]) => {
    setVisibleColumns(columns);
    if (userID) {
      savePreferences.mutate({ activityTableColumns: columns });
    }
  };
  return (
    <Page
      title="Activities"
      actions={
        <>
          <button
            className={`secondary-button ${dateFiltersActive ? "active-filter-button" : ""}`}
            type="button"
            onClick={() => setFiltersOpen(true)}
          >
            <Filter size={16} />
            Filter
            {dateFiltersActive && <span className="button-badge">1</span>}
          </button>
          <button
            className={`secondary-button ${sortActive ? "active-filter-button" : ""}`}
            type="button"
            onClick={() => setSortOpen(true)}
          >
            <ArrowUpDown size={16} />
            Sort
          </button>
          <button
            className={`secondary-button ${hiddenColumnCount > 0 ? "active-filter-button" : ""}`}
            type="button"
            onClick={() => setColumnsOpen(true)}
          >
            <Columns3 size={16} />
            Columns
            {hiddenColumnCount > 0 && <span className="button-badge">{hiddenColumnCount}</span>}
          </button>
        </>
      }
    >
      {filtersOpen && (
        <ActivityFiltersDialog
          filters={filters}
          onApply={setFilters}
          onClose={() => setFiltersOpen(false)}
        />
      )}
      {sortOpen && (
        <ActivitySortDialog
          filters={filters}
          onApply={setFilters}
          onClose={() => setSortOpen(false)}
        />
      )}
      {columnsOpen && (
        <ActivityColumnsDialog
          visibleColumns={visibleColumns}
          onApply={applyColumns}
          onClose={() => setColumnsOpen(false)}
        />
      )}
      <ActivitySearchPanel
        value={filters.search ?? ""}
        onChange={(search) => setFilters({ ...filters, search })}
      />
      <ActivityTypeFilterPanel
        activityTypes={activityTypes.data?.activityTypes ?? []}
        filters={filters}
        onChange={setFilters}
      />
      {activities.isLoading && <LoadingRow />}
      {activities.error && <div className="error">{activities.error instanceof Error ? activities.error.message : "Could not load activities"}</div>}
      {deleteActivity.error && <div className="error">{deleteActivity.error instanceof Error ? deleteActivity.error.message : "Delete failed"}</div>}
      {activitiesLoaded && activityList.length > 0 && (
        <>
          <ActivityTable activities={activityList} visibleColumns={visibleColumns} onDelete={handleDelete} deletingId={deleteActivity.variables} />
          {activities.hasNextPage && (
            <div className="pagination-actions">
              <button
                className="secondary-button"
                type="button"
                disabled={activities.isFetchingNextPage}
                onClick={() => void activities.fetchNextPage()}
              >
                <ChevronDown size={16} />
                {activities.isFetchingNextPage ? "Loading" : "Load more"}
              </button>
            </div>
          )}
        </>
      )}
      {activitiesLoaded && activityList.length === 0 && (
        <EmptyState
          title={anyFiltersActive ? "No activities found" : "No activities yet"}
          action={anyFiltersActive ? undefined : <Link className="secondary-button" to="/settings#import">Import a file</Link>}
        />
      )}
    </Page>
  );
}

function ActivityCalendarPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const month = parseCalendarMonth(searchParams.get("month"));
  const monthRange = calendarMonthRange(month);
  const filters: ActivityTypeFiltersValue = {
    ...emptyActivityTypeFilters,
    dateFrom: monthRange.start,
    dateTo: monthRange.end
  };
  const calendar = useQuery({
    queryKey: ["activity-calendar", month.year, month.month],
    queryFn: () => api.activityCalendar(filters)
  });
  const monthLabel = formatCalendarMonthLabel(month);
  const dayByDate = new Map(calendar.data?.days?.map((day) => [day.date, day]) ?? []);
  const updateMonth = (nextMonth: CalendarMonth) => {
    const params = new URLSearchParams(searchParams);
    params.set("month", formatCalendarMonth(nextMonth));
    setSearchParams(params, { replace: true });
  };
  const firstDay = new Date(month.year, month.month - 1, 1);
  const startWeekday = (firstDay.getDay() + 6) % 7;
  const daysInMonth = new Date(month.year, month.month, 0).getDate();
  const totalSlots = Math.ceil((startWeekday + daysInMonth) / 7) * 7;
  const monthCells = Array.from({ length: totalSlots }, (_, index) => {
    const day = index - startWeekday + 1;
    if (day < 1 || day > daysInMonth) {
      return null;
    }
    const date = formatCalendarDate(month.year, month.month, day);
    const dayData = dayByDate.get(date);
    return {
      day,
      dayData,
      date
    };
  });

  return (
    <Page
      title="Calendar"
      actions={
        <div className="calendar-controls">
          <button
            className="secondary-button small-button"
            type="button"
            onClick={() => updateMonth(calendarMonthOffset(month, -1))}
          >
            <ChevronLeft size={16} />
            Prev
          </button>
          <span className="calendar-month-label">{monthLabel}</span>
          <button
            className="secondary-button small-button"
            type="button"
            onClick={() => updateMonth(calendarMonthOffset(month, 1))}
          >
            Next
            <ChevronRight size={16} />
          </button>
        </div>
      }
    >
      <section className="panel">
        <div className="panel-heading">Monthly activity calendar</div>
        {calendar.isLoading && <LoadingRow />}
        {calendar.error && <div className="error">{calendar.error instanceof Error ? calendar.error.message : "Could not load calendar"}</div>}
        <div className="calendar-weekday-header">
          {calendarWeekdays.map((weekday) => (
            <span key={weekday}>{weekday}</span>
          ))}
        </div>
        <div className="calendar-grid">
          {monthCells.map((entry, index) => {
            if (entry === null) {
              return <div className="calendar-day-cell empty" key={`empty-${index}`} />;
            }
            const hasActivities = entry.dayData && entry.dayData.activityCount > 0;
            return (
              <div
                className={`calendar-day-cell ${hasActivities ? "calendar-day-cell--active" : ""}`}
                key={entry.date}
              >
                <div className="calendar-day-number">{entry.day}</div>
                {hasActivities && (
                  <ul className="calendar-day-list">
                    {entry.dayData?.activities.map((activity) => (
                      <li key={activity.id} className={`calendar-day-activity${activity.source === "training_sheet" ? " calendar-day-activity--planned" : ""}`}>
                        <Link to={`/activities/${activity.id}`}>
                          {activity.name}
                        </Link>
                        <span className="calendar-day-activity-meta">
                          {activity.sportType}
                          {activity.sportType && activity.movingTimeS > 0 && ` · ${formatDuration(activity.movingTimeS)}`}
                        </span>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            );
          })}
        </div>
      </section>
    </Page>
  );
}

function ActivitySearchPanel({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  return (
    <section className="panel search-panel">
      <label className="field search-field">
        <span>Search by name</span>
        <input
          type="search"
          placeholder="Activity name"
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      </label>
      <button className="secondary-button small-button" type="button" disabled={value.length === 0} onClick={() => onChange("")}>
        Clear
      </button>
    </section>
  );
}

function ActivityFiltersDialog({
  filters,
  onApply,
  onClose
}: {
  filters: ActivityTypeFiltersValue;
  onApply: (filters: ActivityTypeFiltersValue) => void;
  onClose: () => void;
}) {
  const [draftDates, setDraftDates] = useState<ActivityDateRange>({
    dateFrom: filters.dateFrom ?? "",
    dateTo: filters.dateTo ?? ""
  });
  const presets = dateFilterPresets();
  const activePreset = presets.find((preset) => dateRangesMatch(draftDates, preset.range));
  const dateRangeInvalid = Boolean(draftDates.dateFrom && draftDates.dateTo && draftDates.dateFrom > draftDates.dateTo);
  const applyDates = () => {
    if (dateRangeInvalid) {
      return;
    }
    onApply({
      ...filters,
      dateFrom: draftDates.dateFrom ?? "",
      dateTo: draftDates.dateTo ?? ""
    });
    onClose();
  };

  return (
    <div
      className="dialog-backdrop"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <section className="filter-dialog" role="dialog" aria-modal="true" aria-labelledby="activity-filters-title">
        <div className="dialog-header">
          <div>
            <div className="eyebrow">Filters</div>
            <h2 id="activity-filters-title">Date</h2>
          </div>
          <button className="icon-button" type="button" aria-label="Close filters" onClick={onClose}>
            <X size={16} />
          </button>
        </div>

        <div className="filter-dialog-section">
          <div className="filter-label">Preset</div>
          <div className="date-preset-grid">
            {presets.map((preset) => (
              <button
                key={preset.id}
                className={`filter-chip ${activePreset?.id === preset.id ? "active" : ""}`}
                type="button"
                onClick={() => setDraftDates(preset.range)}
              >
                {preset.label}
              </button>
            ))}
          </div>
        </div>

        <div className="filter-dialog-section">
          <div className="filter-label">Custom range</div>
          <div className="date-range-grid">
            <label className="field">
              <span>From</span>
              <input
                type="date"
                value={draftDates.dateFrom ?? ""}
                max={draftDates.dateTo || undefined}
                onChange={(event) => setDraftDates({ ...draftDates, dateFrom: event.target.value })}
              />
            </label>
            <label className="field">
              <span>To</span>
              <input
                type="date"
                value={draftDates.dateTo ?? ""}
                min={draftDates.dateFrom || undefined}
                onChange={(event) => setDraftDates({ ...draftDates, dateTo: event.target.value })}
              />
            </label>
          </div>
          {dateRangeInvalid && <div className="row-error">End date must be after start date.</div>}
        </div>

        <div className="dialog-actions">
          <button className="secondary-button" type="button" onClick={() => setDraftDates({ dateFrom: "", dateTo: "" })}>
            Clear dates
          </button>
          <button className="secondary-button" type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="primary-button" type="button" disabled={dateRangeInvalid} onClick={applyDates}>
            Apply
          </button>
        </div>
      </section>
    </div>
  );
}

function ActivitySortDialog({
  filters,
  onApply,
  onClose
}: {
  filters: ActivityTypeFiltersValue;
  onApply: (filters: ActivityTypeFiltersValue) => void;
  onClose: () => void;
}) {
  const [draftSort, setDraftSort] = useState<ActivitySort>(normalizedActivitySort(filters));
  const applySort = () => {
    onApply({
      ...filters,
      sortBy: draftSort.sortBy,
      sortOrder: draftSort.sortOrder
    });
    onClose();
  };

  return (
    <div
      className="dialog-backdrop"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <section className="filter-dialog" role="dialog" aria-modal="true" aria-labelledby="activity-sort-title">
        <div className="dialog-header">
          <div>
            <div className="eyebrow">Sort</div>
            <h2 id="activity-sort-title">Activities</h2>
          </div>
          <button className="icon-button" type="button" aria-label="Close sort" onClick={onClose}>
            <X size={16} />
          </button>
        </div>

        <div className="filter-dialog-section">
          <div className="filter-label">Sort by</div>
          <div className="sort-option-grid">
            {activitySortOptions().map((option) => (
              <button
                key={option.value}
                className={`sort-choice ${draftSort.sortBy === option.value ? "active" : ""}`}
                type="button"
                onClick={() => setDraftSort({ ...draftSort, sortBy: option.value })}
              >
                <span>{option.label}</span>
              </button>
            ))}
          </div>
        </div>

        <div className="filter-dialog-section">
          <div className="filter-label">Direction</div>
          <div className="segmented-control">
            <button
              className={draftSort.sortOrder === "desc" ? "active" : ""}
              type="button"
              onClick={() => setDraftSort({ ...draftSort, sortOrder: "desc" })}
            >
              <ArrowDown size={15} />
              Desc
            </button>
            <button
              className={draftSort.sortOrder === "asc" ? "active" : ""}
              type="button"
              onClick={() => setDraftSort({ ...draftSort, sortOrder: "asc" })}
            >
              <ArrowUp size={15} />
              Asc
            </button>
          </div>
        </div>

        <div className="dialog-actions">
          <button className="secondary-button" type="button" onClick={() => setDraftSort(defaultActivitySort)}>
            Reset
          </button>
          <button className="secondary-button" type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="primary-button" type="button" onClick={applySort}>
            Apply
          </button>
        </div>
      </section>
    </div>
  );
}

function ActivityTypeFilterPanel({
  activityTypes,
  filters,
  onChange
}: {
  activityTypes: string[];
  filters: ActivityTypeFiltersValue;
  onChange: (filters: ActivityTypeFiltersValue) => void;
}) {
  const includeSet = new Set(filters.sports);
  const excludeSet = new Set(filters.excludeSports);
  if (activityTypes.length === 0) {
    return null;
  }

  const toggleInclude = (sport: string) => {
    const nextSports = includeSet.has(sport)
      ? filters.sports.filter((item) => item !== sport)
      : [...filters.sports, sport];
    onChange({
      ...filters,
      sports: nextSports,
      excludeSports: filters.excludeSports.filter((item) => item !== sport)
    });
  };
  const toggleExclude = (sport: string) => {
    const nextExcluded = excludeSet.has(sport)
      ? filters.excludeSports.filter((item) => item !== sport)
      : [...filters.excludeSports, sport];
    onChange({
      ...filters,
      sports: filters.sports.filter((item) => item !== sport),
      excludeSports: nextExcluded
    });
  };
  const clearFilters = () => onChange({ ...filters, sports: [], excludeSports: [] });
  const hasFilters = filters.sports.length > 0 || filters.excludeSports.length > 0;

  return (
    <section className="panel filter-panel">
      <div className="filter-header">
        <div className="panel-heading">Activity types</div>
        <button className="secondary-button small-button" type="button" disabled={!hasFilters} onClick={clearFilters}>Clear</button>
      </div>
      <div className="filter-grid">
        <div className="filter-group">
          <div className="filter-label">Show only</div>
          <div className="chip-list">
            {activityTypes.map((sport) => (
              <button
                key={`include-${sport}`}
                className={`filter-chip ${includeSet.has(sport) ? "active" : ""}`}
                type="button"
                onClick={() => toggleInclude(sport)}
              >
                {sport}
              </button>
            ))}
          </div>
        </div>
        <div className="filter-group">
          <div className="filter-label">Exclude</div>
          <div className="chip-list">
            {activityTypes.map((sport) => (
              <button
                key={`exclude-${sport}`}
                className={`filter-chip exclude ${excludeSet.has(sport) ? "active" : ""}`}
                type="button"
                onClick={() => toggleExclude(sport)}
              >
                {sport}
              </button>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}

function ActivityColumnsDialog({
  visibleColumns,
  onApply,
  onClose
}: {
  visibleColumns: ActivityTableColumnKey[];
  onApply: (columns: ActivityTableColumnKey[]) => void;
  onClose: () => void;
}) {
  const [draftColumns, setDraftColumns] = useState<ActivityTableColumnKey[]>(visibleColumns);
  const visibleSet = new Set(draftColumns);
  const toggleColumn = (key: ActivityTableColumnKey) => {
    setDraftColumns((current) => {
      const currentSet = new Set(current);
      if (currentSet.has(key)) {
        currentSet.delete(key);
      } else {
        currentSet.add(key);
      }
      return defaultActivityTableColumns.filter((column) => currentSet.has(column));
    });
  };
  const applyColumns = () => {
    onApply(draftColumns);
    onClose();
  };

  return (
    <div
      className="dialog-backdrop"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <section className="filter-dialog" role="dialog" aria-modal="true" aria-labelledby="activity-columns-title">
        <div className="dialog-header">
          <div>
            <div className="eyebrow">Columns</div>
            <h2 id="activity-columns-title">Activities</h2>
          </div>
          <button className="icon-button" type="button" aria-label="Close columns" onClick={onClose}>
            <X size={16} />
          </button>
        </div>

        <div className="filter-dialog-section">
          <div className="column-option-grid">
            <label className="column-option locked">
              <input type="checkbox" checked disabled />
              <span>Name</span>
            </label>
            {activityTableColumnOptions.map((option) => (
              <label className="column-option" key={option.key}>
                <input
                  type="checkbox"
                  checked={visibleSet.has(option.key)}
                  onChange={() => toggleColumn(option.key)}
                />
                <span>{option.label}</span>
              </label>
            ))}
          </div>
        </div>

        <div className="dialog-actions">
          <button className="secondary-button" type="button" onClick={() => setDraftColumns(defaultActivityTableColumns)}>
            Show all
          </button>
          <button className="secondary-button" type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="primary-button" type="button" onClick={applyColumns}>
            Apply
          </button>
        </div>
      </section>
    </div>
  );
}

function ActivityTable({
  activities,
  compact = false,
  visibleColumns,
  onDelete,
  deletingId
}: {
  activities: Activity[];
  compact?: boolean;
  visibleColumns?: ActivityTableColumnKey[];
  onDelete?: (activity: Activity) => void;
  deletingId?: string;
}) {
  if (activities.length === 0) {
    return <EmptyState title="No activities found" />;
  }
  const columns = compact ? compactActivityTableColumns : (visibleColumns ?? defaultActivityTableColumns);
  const showColumn = (column: ActivityTableColumnKey) => columns.includes(column);
  return (
    <div className="table-wrap">
      <table className="data-table activity-table">
        <thead>
          <tr>
            {showColumn("date") && <th className="activity-date-column">Date</th>}
            <th className="activity-name-column">Name</th>
            {showColumn("type") && <th className="activity-type-column">Type</th>}
            {showColumn("gear") && <th className="activity-gear-column">Gear</th>}
            {showColumn("distance") && <th className="activity-distance-column">Distance</th>}
            {showColumn("time") && <th className="activity-time-column">Time</th>}
            {showColumn("calories") && <th className="activity-calories-column">Calories</th>}
            {showColumn("source") && <th className="activity-source-column">Source</th>}
            {onDelete && <th aria-label="Actions" />}
          </tr>
        </thead>
        <tbody>
          {activities.map((activity) => (
            <tr key={activity.id}>
              {showColumn("date") && <td>{formatDate(activity.startTime)}</td>}
              <td className="activity-name-cell"><Link to={`/activities/${activity.id}`} title={activity.name}>{activity.name}</Link></td>
              {showColumn("type") && <td className="clip-cell" title={activity.sportType}>{activity.sportType}</td>}
              {showColumn("gear") && <td className="gear-table-cell"><GearChipList gear={activity.gear} compact /></td>}
              {showColumn("distance") && <td>{formatDistance(activity.distanceM)}</td>}
              {showColumn("time") && <td>{formatDuration(activity.movingTimeS || activity.elapsedTimeS)}</td>}
              {showColumn("calories") && <td>{formatCalories(activity.caloriesKcal)}</td>}
              {showColumn("source") && <td><span className="source-pill">{activity.source}</span></td>}
              {onDelete && (
                <td className="row-actions">
                  <button
                    className="icon-button danger"
                    type="button"
                    title="Delete activity"
                    aria-label={`Delete ${activity.name}`}
                    disabled={deletingId === activity.id}
                    onClick={() => onDelete(activity)}
                  >
                    <Trash2 size={16} />
                  </button>
                </td>
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ActivityDetailPage({ config }: { config?: AppConfig }) {
  const { id } = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const activityQueryKey = ["activity", id] as const;
  const [plannedMatchWindowDays, setPlannedMatchWindowDays] = useState(7);
  const activity = useQuery({ queryKey: activityQueryKey, queryFn: () => api.activity(id!), enabled: Boolean(id) });
  const plannedMatchCandidates = useQuery({
    queryKey: ["planned-match-candidates", id, plannedMatchWindowDays],
    queryFn: () => api.plannedMatchCandidates(id!, plannedMatchWindowDays),
    enabled: Boolean(id) && activity.data?.activity.source !== "training_sheet",
    refetchInterval: (query) => {
      const writeback = query.state.data?.writeback;
      if (!writeback) {
        return false;
      }
      return writeback.jobStatus === "running" || writeback.summaryStatus === "running" || writeback.intervalsStatus === "running" || writeback.feedbackStatus === "running" ? 1500 : false;
    }
  });
  const previewPlannedActivity = useMutation({
    mutationFn: (draft: PlannedMatchDraft) => api.plannedMatchPreview(id!, draft),
    onSuccess: ({ preview }) => {
      setMatchPreview(preview);
    }
  });
  const applyPlannedActivity = useMutation({
    mutationFn: (draft: PlannedMatchDraft & { fingerprint: string }) => api.applyPlannedMatchPreview(id!, draft),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["planned-match-candidates", id] }),
        queryClient.invalidateQueries({ queryKey: ["planned-activities"] }),
        queryClient.invalidateQueries({ queryKey: ["activity-calendar"] }),
        queryClient.invalidateQueries({ queryKey: ["activity", id] })
      ]);
      setMatchPreview(undefined);
      setMatchOpen(false);
    },
    onError: () => {
      setMatchPreview(undefined);
    }
  });
  const unmatchPlannedActivity = useMutation({
    mutationFn: () => api.unmatchPlannedActivity(id!),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["planned-match-candidates", id] }),
        queryClient.invalidateQueries({ queryKey: ["planned-activities"] }),
        queryClient.invalidateQueries({ queryKey: ["activity-calendar"] })
      ]);
    }
  });
  const deleteActivity = useMutation({
    mutationFn: api.deleteActivity,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["activities"] }),
        queryClient.invalidateQueries({ queryKey: ["summary"] }),
        queryClient.invalidateQueries({ queryKey: ["activity-types"] })
      ]);
      navigate("/activities");
    }
  });
  const renameActivity = useMutation({
    mutationFn: (name: string) => api.renameActivity(id!, name),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["activity", id] }),
        queryClient.invalidateQueries({ queryKey: ["activities"] }),
        queryClient.invalidateQueries({ queryKey: ["summary"] })
      ]);
      setRenameOpen(false);
    }
  });
  const updateActivityNotes = useMutation({
    mutationFn: (notes: string) => api.updateActivityNotes(id!, notes),
    onSuccess: async (result) => {
      queryClient.setQueryData<{ activity: Activity }>(activityQueryKey, result);
      await queryClient.invalidateQueries({ queryKey: ["activity", id] });
      setNotesOpen(false);
    }
  });
  const updateActivityReflection = useMutation({
    mutationFn: ({ feedback, rpe }: { feedback: string; rpe: number | null }) => api.updateActivityReflection(id!, feedback, rpe),
    onSuccess: async (result) => {
      queryClient.setQueryData<{ activity: Activity }>(activityQueryKey, result);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["activity", id] }),
        queryClient.invalidateQueries({ queryKey: ["planned-match-candidates", id] })
      ]);
      setCheckInOpen(false);
    }
  });
  const retryWriteback = useMutation({
    mutationFn: () => api.retryPlannedWriteback(id!),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["sync-jobs"] });
      await queryClient.invalidateQueries({ queryKey: ["planned-match-candidates", id] });
    }
  });
  const uploadMedia = useMutation({
    mutationFn: async (files: File[]) => {
      const uploaded: Array<{ media: ActivityMedia }> = [];
      for (const file of files) {
        uploaded.push(await api.uploadActivityMedia(id!, file));
      }
      return uploaded;
    },
    onSuccess: (uploaded) => {
      setMediaFileInputKey((key) => key + 1);
      queryClient.setQueryData<{ activity: Activity }>(activityQueryKey, (current) => {
        if (!current) {
          return current;
        }
        return {
          activity: {
            ...current.activity,
            media: mergeActivityMedia(current.activity.media ?? [], uploaded.map((item) => item.media))
          }
        };
      });
    }
  });
  const [highlightedSample, setHighlightedSample] = useState<ActivityChartPoint | undefined>();
  const [selectedClimbIndex, setSelectedClimbIndex] = useState<number | undefined>();
  const [routeColorSource, setRouteColorSource] = useState<RouteColorSource>("pace");
  const [actionsOpen, setActionsOpen] = useState(false);
  const [renameOpen, setRenameOpen] = useState(false);
  const [notesOpen, setNotesOpen] = useState(false);
  const [matchOpen, setMatchOpen] = useState(false);
  const [matchCandidateId, setMatchCandidateId] = useState<string>();
  const [matchPreview, setMatchPreview] = useState<TrainingSheetWritebackPreview>();
  const [checkInOpen, setCheckInOpen] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [mediaFileInputKey, setMediaFileInputKey] = useState(0);
  const [selectedMediaId, setSelectedMediaId] = useState<string>();
  const [analysisTab, setAnalysisTab] = useState<ActivityAnalysisTab>("stats");
  const [climbSensitivityDraft, setClimbSensitivityDraft] = useState(defaultClimbSensitivity);
  const [climbSensitivityPreview, setClimbSensitivityPreview] = useState(defaultClimbSensitivity);
  const routeUsesGap = (activity.data?.activity.laps ?? []).some((lap) => lap.avgGradeAdjustedPaceSPKM !== undefined);
  const configuredClimbSensitivity = config?.climbDetection?.sensitivity ?? defaultClimbSensitivity;
  const climbSensitivity = clampClimbSensitivity(climbSensitivityDraft);
  const climbSensitivityForPreview = clampClimbSensitivity(climbSensitivityPreview);
  const isPreviewPending = climbSensitivity !== climbSensitivityForPreview;
  const climbPreview = useQuery({
    queryKey: ["activity-climb-preview", id, climbSensitivityForPreview],
    queryFn: () => api.activityClimbPreview(id!, climbSensitivityForPreview),
    placeholderData: (previousData) => previousData,
    enabled: Boolean(activity.data)
  });
  useEffect(() => {
    if (!routeUsesGap) {
      setRouteColorSource("pace");
    }
  }, [id, routeUsesGap]);

  useEffect(() => {
    setHighlightedSample(undefined);
    setSelectedClimbIndex(undefined);
    setActionsOpen(false);
    setRenameOpen(false);
    setNotesOpen(false);
    setMatchOpen(false);
    setMatchCandidateId(undefined);
    setCheckInOpen(false);
    setPlannedMatchWindowDays(7);
    setExportOpen(false);
    setSelectedMediaId(undefined);
    setAnalysisTab("stats");
    updateActivityNotes.reset();
    uploadMedia.reset();
    setMediaFileInputKey((key) => key + 1);
    setClimbSensitivityDraft(configuredClimbSensitivity);
    setClimbSensitivityPreview(configuredClimbSensitivity);
  }, [id, configuredClimbSensitivity]);

  useEffect(() => {
    const nextValue = clampClimbSensitivity(climbSensitivity);
    const timeout = window.setTimeout(() => setClimbSensitivityPreview(nextValue), 120);
    return () => window.clearTimeout(timeout);
  }, [climbSensitivity]);

  const item = activity.data?.activity;
  const effectiveClimbs = item ? (climbPreview.data?.climbs ?? item.climbs ?? []) : [];

  useEffect(() => {
    if (selectedClimbIndex === undefined) {
      return;
    }
    if (!effectiveClimbs.some((climb) => climb.index === selectedClimbIndex)) {
      setSelectedClimbIndex(undefined);
    }
  }, [effectiveClimbs, selectedClimbIndex]);

  if (activity.isLoading) {
    return <Page title="Activity"><LoadingRow /></Page>;
  }
  if (!activity.data || !item) {
    return <Page title="Activity"><EmptyState title="Activity not found" /></Page>;
  }
  if (item.source === "training_sheet") {
    const notes = (item.notes ?? "").trim();
    return (
      <Page title={item.name}>
        {notes && (
          <section className="panel">
            <div className="panel-heading">Note</div>
            <p>{notes}</p>
          </section>
        )}
      </Page>
    );
  }

  const confirmedItem = item;
  const mediaItems = item.media ?? [];
  const locatedMedia = mediaItems.filter(hasMediaLocation);
  const routePoints = routeForActivity(confirmedItem);
  const canExportGPX = canExportActivityGPX(confirmedItem);
  const paceScale = paceScaleForActivity(confirmedItem, "pace");
  const routePaceScale = paceScaleForActivity(confirmedItem, routeColorSource);
  const paceRouteSegments = paceRouteSegmentsForActivity(confirmedItem, routePaceScale, routeColorSource);
  const chartData = chartDataFor(confirmedItem.samples ?? [], paceScale);
  const highlightedPoint = routePointForChartPoint(highlightedSample);
  const finalClimbs = effectiveClimbs;
  const selectedClimb = selectedClimbIndex === undefined ? undefined : finalClimbs.find((climb) => climb.index === selectedClimbIndex);
  const climbMapSegments = climbMapSegmentsFor(confirmedItem, finalClimbs);
  const selectedClimbProfile = climbProfileFor(confirmedItem, selectedClimb);
  const feedbackAvailable = Boolean(plannedMatchCandidates.data?.matched?.feedbackCell?.trim());

  const handleSelectClimb = (climb: ActivityClimb) => {
    setSelectedClimbIndex((current) => current === climb.index ? undefined : climb.index);
  };
  const handleDelete = () => {
    setActionsOpen(false);
    if (window.confirm(deleteActivityConfirmation(item))) {
      deleteActivity.mutate(item.id);
    }
  };
  const handleRename = (name: string) => {
    renameActivity.mutate(name);
  };
  const handleSaveNotes = (notes: string) => {
    updateActivityNotes.mutate(notes);
  };
  const handleDeleteNotes = () => {
    if (window.confirm("Delete this note?")) {
      updateActivityNotes.mutate("");
    }
  };
  const handleSaveReflection = (feedback: string, rpe: number | null) => {
    updateActivityReflection.mutate({ feedback, rpe });
  };
  const openMatchDialog = (candidateId?: string) => {
    const nextCandidateId = candidateId ?? plannedMatchCandidates.data?.suggestedId ?? plannedMatchCandidates.data?.candidates[0]?.id;
    if (!nextCandidateId) {
      return;
    }
    previewPlannedActivity.reset();
    applyPlannedActivity.reset();
    setMatchPreview(undefined);
    setMatchCandidateId(nextCandidateId);
    setMatchOpen(true);
  };
  const handlePreviewMatch = (draft: PlannedMatchDraft) => {
    applyPlannedActivity.reset();
    previewPlannedActivity.mutate(draft);
  };
  const resetMatchPreview = () => {
    setMatchPreview(undefined);
    previewPlannedActivity.reset();
    applyPlannedActivity.reset();
  };
  const handleApplyMatch = (draft: PlannedMatchDraft) => {
    if (!matchPreview) {
      return;
    }
    applyPlannedActivity.mutate({ ...draft, fingerprint: matchPreview.fingerprint });
  };
  const handleMediaFilesSelected = (files: File[]) => {
    if (files.length === 0 || uploadMedia.isPending) {
      return;
    }
    uploadMedia.reset();
    uploadMedia.mutate(files);
  };
  const handleClimbSensitivityChange = (value: number) => {
    setClimbSensitivityDraft(clampClimbSensitivity(value));
  };
  const climbSensitivityControls = (
    <details className="climb-sensitivity-details">
      <summary>
        <span>Adjust sensitivity</span>
        <strong>{climbSensitivity}</strong>
      </summary>
      <div className="climb-sensitivity-controls" role="region" aria-label="Temporary climb sensitivity controls">
        <div className="climb-sensitivity-range">
          <span>Temporary for this activity</span>
          <strong>{climbSensitivity}</strong>
        </div>
        <input
          className="climb-sensitivity-slider"
          type="range"
          min={0}
          max={100}
          step={1}
          value={climbSensitivity}
          aria-label="Climb sensitivity"
          onChange={(event) => handleClimbSensitivityChange(Number(event.target.value))}
        />
        <div className="climb-sensitivity-preset-row">
          <span className="muted">Changes apply only while this activity is open.</span>
          {isPreviewPending && <span className="muted">Recalculating…</span>}
        </div>
      </div>
    </details>
  );

  return (
    <Page
      title={confirmedItem.name}
      eyebrow={`${confirmedItem.sportType} · ${formatDate(confirmedItem.startTime)}`}
      actions={
        <>
          <ActivityMediaUploadAction
            inputKey={mediaFileInputKey}
            uploading={uploadMedia.isPending}
            onFilesSelected={handleMediaFilesSelected}
          />
          <ActivityDetailActions
            activity={item}
            open={actionsOpen}
            deleting={deleteActivity.isPending}
            canExportGPX={canExportGPX}
            canUnmatchPlanned={Boolean(plannedMatchCandidates.data?.matched)}
            unmatching={unmatchPlannedActivity.isPending}
            onToggle={() => setActionsOpen((current) => !current)}
            onRename={() => {
              renameActivity.reset();
              setRenameOpen(true);
              setActionsOpen(false);
            }}
            onNotes={() => {
              updateActivityNotes.reset();
              setNotesOpen(true);
              setActionsOpen(false);
            }}
            onExportGPX={() => {
              setExportOpen(true);
              setActionsOpen(false);
            }}
            onUnmatchPlanned={() => {
              setActionsOpen(false);
              unmatchPlannedActivity.mutate();
            }}
            onDelete={handleDelete}
          />
        </>
      }
    >
      {deleteActivity.error && <div className="error">{deleteActivity.error instanceof Error ? deleteActivity.error.message : "Delete failed"}</div>}
      {renameOpen && (
        <ActivityRenameDialog
          activity={item}
          saving={renameActivity.isPending}
          error={renameActivity.error}
          onSave={handleRename}
          onClose={() => setRenameOpen(false)}
        />
      )}
      {notesOpen && (
        <ActivityNotesDialog
          activity={item}
          saving={updateActivityNotes.isPending}
          error={updateActivityNotes.error}
          onSave={handleSaveNotes}
          onClose={() => setNotesOpen(false)}
        />
      )}
      {matchOpen && plannedMatchCandidates.data && (
        <PlannedActivityMatchDialog
          data={plannedMatchCandidates.data}
          selectedCandidateId={matchCandidateId}
          canLoadMore={plannedMatchWindowDays === 7 && plannedMatchCandidates.data.hasMore}
          matching={previewPlannedActivity.isPending || applyPlannedActivity.isPending}
          error={previewPlannedActivity.error ?? applyPlannedActivity.error}
          preview={matchPreview}
          onSelectCandidate={setMatchCandidateId}
          onPreview={handlePreviewMatch}
          onApply={handleApplyMatch}
          onPreviewReset={resetMatchPreview}
          onLoadMore={() => setPlannedMatchWindowDays(30)}
          onClose={() => setMatchOpen(false)}
        />
      )}
      {checkInOpen && (
        <ActivityReflectionDialog
          activity={item}
          feedbackAvailable={feedbackAvailable}
          saving={updateActivityReflection.isPending}
          error={updateActivityReflection.error}
          onSave={handleSaveReflection}
          onClose={() => setCheckInOpen(false)}
        />
      )}
      {exportOpen && (
        <ActivityExportGPXDialog
          activity={item}
          onClose={() => setExportOpen(false)}
        />
      )}
      <PlannedActivityMatchPanel
        data={plannedMatchCandidates.data}
        loading={plannedMatchCandidates.isLoading}
        error={plannedMatchCandidates.error ?? previewPlannedActivity.error ?? applyPlannedActivity.error ?? updateActivityReflection.error ?? unmatchPlannedActivity.error ?? retryWriteback.error}
        matching={previewPlannedActivity.isPending || applyPlannedActivity.isPending || updateActivityReflection.isPending}
        retrying={retryWriteback.isPending}
        feedbackAvailable={feedbackAvailable}
        canLoadMore={plannedMatchWindowDays === 7 && Boolean(plannedMatchCandidates.data?.hasMore)}
        windowDays={plannedMatchWindowDays}
        onMatchHint={openMatchDialog}
        onOpenMatch={() => openMatchDialog()}
        onLoadMore={() => setPlannedMatchWindowDays(30)}
        onOpenCheckIn={() => {
          updateActivityReflection.reset();
          setCheckInOpen(true);
        }}
        onRetry={() => retryWriteback.mutate()}
      />
      <section className="metric-grid">
        <Metric label="Distance" value={formatDistance(item.distanceM)} />
        <Metric label="Moving Time" value={formatDuration(item.movingTimeS || item.elapsedTimeS)} />
        <Metric label="Pace" value={formatPace(item.avgPaceSPKM)} />
        {item.avgHeartRate !== undefined && <Metric label="Avg HR" value={formatBPM(item.avgHeartRate)} />}
        {item.maxHeartRate !== undefined && <Metric label="Max HR" value={formatBPM(item.maxHeartRate)} />}
        <Metric label="Elevation" value={`${Math.round(item.elevationGainM).toLocaleString()} m`} />
        {item.avgGradeAdjustedPaceSPKM !== undefined && <Metric label="GAP" value={formatPace(item.avgGradeAdjustedPaceSPKM)} />}
        {item.caloriesKcal !== undefined && <Metric label="Calories" value={formatCalories(item.caloriesKcal)} />}
      </section>

      <ActivityNotesPanel
        notes={item.notes ?? ""}
        saving={updateActivityNotes.isPending}
        onEdit={() => {
          updateActivityNotes.reset();
          setNotesOpen(true);
        }}
        onDelete={handleDeleteNotes}
      />


      {(item.gear ?? []).length > 0 && (
        <section className="panel gear-activity-panel">
          <div className="panel-heading">Gear</div>
          <GearChipList gear={item.gear} />
        </section>
      )}

      {mediaItems.length > 0 ? (
        <ActivityMediaPanel
          activity={item}
          uploading={uploadMedia.isPending}
          uploadError={uploadMedia.error}
          selectedMediaId={selectedMediaId}
          onSelectMedia={setSelectedMediaId}
        />
      ) : (
        Boolean(uploadMedia.error) && <div className="error">{uploadMedia.error instanceof Error ? uploadMedia.error.message : "Upload failed"}</div>
      )}

      {(routePoints.length > 1 || locatedMedia.length > 0) && (
        <section className="panel">
          <div className="route-panel-header">
            <div className="panel-heading">Route</div>
          </div>
          <ActivityMap
            points={routePoints}
            paceSegments={paceRouteSegments}
            tileURL={config?.mapTileURL}
            highlightedPoint={highlightedPoint}
            climbSegments={climbMapSegments}
            selectedClimbIndex={selectedClimb?.index}
            onSelectClimb={handleSelectClimb}
            mediaMarkers={locatedMedia}
            selectedMediaId={selectedMediaId}
            onSelectMedia={setSelectedMediaId}
            routeColorSource={routeColorSource}
            onRouteColorSourceChange={setRouteColorSource}
            showRouteColorSelector={routeUsesGap}
          />
        </section>
      )}

      <ActivityClimbsPanel
        climbs={effectiveClimbs}
        selectedClimb={selectedClimb}
        profileData={selectedClimbProfile}
        sensitivityControls={climbSensitivityControls}
        onSelect={handleSelectClimb}
      />

      <div className="activity-analysis-tabs" role="tablist" aria-label="Activity analysis">
        <button
          className={analysisTab === "stats" ? "active" : ""}
          type="button"
          role="tab"
          aria-selected={analysisTab === "stats"}
          onClick={() => setAnalysisTab("stats")}
        >
          Stats
        </button>
        <button
          className={analysisTab === "intervals" ? "active" : ""}
          type="button"
          role="tab"
          aria-selected={analysisTab === "intervals"}
          onClick={() => setAnalysisTab("intervals")}
        >
          Intervals
        </button>
      </div>
      {analysisTab === "stats" ? (
        <ActivityCombinedChart key={item.id} data={chartData} onHighlight={setHighlightedSample} />
      ) : (
        <ActivityIntervalsPanel activity={confirmedItem} />
      )}
    </Page>
  );
}

function ActivityIntervalsPanel({ activity }: { activity: Activity }) {
  const intervals = activity.intervals ?? [];
  const laps = activity.laps ?? [];
  const [filter, setFilter] = useState("all");
  const [expanded, setExpanded] = useState<Record<number, boolean>>({});

  useEffect(() => {
    setFilter("all");
    setExpanded({});
  }, [activity.id]);

  if (intervals.length === 0) {
    return <ActivityFlatLapTable activity={activity} />;
  }

  const categories = Array.from(new Set(intervals.map((interval) => interval.category).filter(Boolean)));
  const filteredIntervals = filter === "all" ? intervals : intervals.filter((interval) => interval.category === filter);
  const lapsByIndex = new Map(laps.map((lap) => [lap.index, lap]));
  const showGap = intervals.some((interval) => interval.avgGradeAdjustedPaceSPKM !== undefined) || laps.some((lap) => lap.avgGradeAdjustedPaceSPKM !== undefined);
  const showHeartRate = intervals.some((interval) => interval.avgHeartRate !== undefined || interval.maxHeartRate !== undefined) || laps.some((lap) => lap.avgHeartRate !== undefined || lap.maxHeartRate !== undefined);
  const showElevation = intervals.some((interval) => interval.elevationGainM !== undefined || interval.elevationLossM !== undefined) || laps.some((lap) => lap.elevationGainM !== undefined || lap.elevationLossM !== undefined);
  const showCadence = intervals.some((interval) => interval.avgRunCadence !== undefined) || laps.some((lap) => lap.avgRunCadence !== undefined);
  const showGroundContact = intervals.some((interval) => interval.avgGroundContactTimeMS !== undefined) || laps.some((lap) => lap.avgGroundContactTimeMS !== undefined);
  const showPower = intervals.some((interval) => interval.avgPower !== undefined) || laps.some((lap) => lap.avgPower !== undefined);

  return (
    <section className="panel activity-intervals-panel">
      <div className="intervals-header">
        <div>
          <div className="panel-heading">Intervals</div>
          {activity.workout?.name && <div className="muted">Workout: {activity.workout.name}</div>}
        </div>
        <label className="compact-field" htmlFor="activity-interval-filter">
          <span>Step Type</span>
          <select id="activity-interval-filter" value={filter} onChange={(event) => setFilter(event.target.value)}>
            <option value="all">All</option>
            {categories.map((category) => (
              <option key={category} value={category}>{intervalCategoryLabel(category, activity.sportType)}</option>
            ))}
          </select>
        </label>
      </div>
      <div className="table-wrap">
        <table className="data-table interval-table">
          <thead>
            <tr>
              <th aria-label="Expand" />
              <th>Step</th>
              <th>Laps</th>
              <th>Time</th>
              <th>Cumulative</th>
              <th>Distance</th>
              <th>Avg Pace</th>
              {showGap && <th>Avg GAP</th>}
              {showHeartRate && <th>Avg HR</th>}
              {showHeartRate && <th>Max HR</th>}
              {showElevation && <th>Gain</th>}
              {showElevation && <th>Loss</th>}
              {showCadence && <th>Avg Cadence</th>}
              {showGroundContact && <th>Avg GCT</th>}
              {showPower && <th>Avg Power</th>}
            </tr>
          </thead>
          <tbody>
            {filteredIntervals.map((interval) => {
              const intervalLaps = interval.lapIndexes?.map((index) => lapsByIndex.get(index)).filter((lap): lap is ActivityLap => Boolean(lap)) ?? [];
              const isExpanded = Boolean(expanded[interval.index]);
              const label = intervalStepLabel(interval, activity.sportType);
              return (
                <Fragment key={`interval-group-${interval.index}`}>
                  <tr className="interval-summary-row" key={`interval-${interval.index}`}>
                    <td className="interval-expand-cell">
                      <button
                        className="table-icon-button"
                        type="button"
                        aria-label={`${isExpanded ? "Collapse" : "Expand"} ${label}`}
                        aria-expanded={isExpanded}
                        onClick={() => setExpanded((current) => ({ ...current, [interval.index]: !isExpanded }))}
                      >
                        {isExpanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
                      </button>
                    </td>
                    <td>
                      <strong>{label}</strong>
                      {intervalTargetLabel(activity.workout, interval) && <span className="interval-target">{intervalTargetLabel(activity.workout, interval)}</span>}
                    </td>
                    <td>{formatLapRange(interval.lapIndexes)}</td>
                    <td>{formatDuration(interval.elapsedTimeS)}</td>
                    <td>{formatDuration(intervalCumulativeTime(interval, intervals, activity.startTime))}</td>
                    <td>{formatDistance(interval.distanceM)}</td>
                    <td>{optionalPace(interval.avgPaceSPKM)}</td>
                    {showGap && <td>{optionalPace(interval.avgGradeAdjustedPaceSPKM)}</td>}
                    {showHeartRate && <td>{optionalBPM(interval.avgHeartRate)}</td>}
                    {showHeartRate && <td>{optionalBPM(interval.maxHeartRate)}</td>}
                    {showElevation && <td>{optionalMeters(interval.elevationGainM)}</td>}
                    {showElevation && <td>{optionalMeters(interval.elevationLossM)}</td>}
                    {showCadence && <td>{optionalCadence(interval.avgRunCadence)}</td>}
                    {showGroundContact && <td>{optionalMilliseconds(interval.avgGroundContactTimeMS)}</td>}
                    {showPower && <td>{optionalWatts(interval.avgPower)}</td>}
                  </tr>
                  {isExpanded && intervalLaps.map((lap) => (
                    <tr className="interval-lap-row" key={`interval-${interval.index}-lap-${lap.index}`}>
                      <td />
                      <td>Lap {lap.index + 1}</td>
                      <td>{lap.index + 1}</td>
                      <td>{formatDuration(lapMovingTimeS(lap, laps.length > 0 ? activity.samples ?? [] : []))}</td>
                      <td>{formatDuration(lapCumulativeTime(lap, laps, activity.startTime))}</td>
                      <td>{formatDistance(lap.distanceM)}</td>
                      <td>{optionalPace(lapPaceSPKM(lap, activity.samples ?? []))}</td>
                      {showGap && <td>{optionalPace(lap.avgGradeAdjustedPaceSPKM)}</td>}
                      {showHeartRate && <td>{optionalBPM(lap.avgHeartRate)}</td>}
                      {showHeartRate && <td>{optionalBPM(lap.maxHeartRate)}</td>}
                      {showElevation && <td>{optionalMeters(lap.elevationGainM)}</td>}
                      {showElevation && <td>{optionalMeters(lap.elevationLossM)}</td>}
                      {showCadence && <td>{optionalCadence(lap.avgRunCadence)}</td>}
                      {showGroundContact && <td>{optionalMilliseconds(lap.avgGroundContactTimeMS)}</td>}
                      {showPower && <td>{optionalWatts(lap.avgPower)}</td>}
                    </tr>
                  ))}
                </Fragment>
              );
            })}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function ActivityFlatLapTable({ activity }: { activity: Activity }) {
  const laps = activity.laps ?? [];
  if (laps.length === 0) {
    return <section className="panel"><EmptyState title="No lap or structured workout data" /></section>;
  }
  const showGap = laps.some((lap) => lap.avgGradeAdjustedPaceSPKM !== undefined);
  const showElevation = laps.some((lap) => lap.elevationGainM !== undefined || lap.elevationLossM !== undefined);
  return (
    <section className="panel">
      <div className="panel-heading">Laps</div>
      <p className="muted interval-fallback-note">No structured workout steps were provided; showing recorded laps.</p>
      <div className="table-wrap">
        <table className="data-table">
          <thead><tr><th>Lap</th><th>Distance</th><th>Time</th><th>Pace</th>{showGap && <th>GAP</th>}{showElevation && <th>Gain</th>}{showElevation && <th>Loss</th>}</tr></thead>
          <tbody>
            {laps.map((lap) => (
              <tr key={lap.index}>
                <td>{lap.index + 1}</td>
                <td>{formatDistance(lap.distanceM)}</td>
                <td>{formatDuration(lapMovingTimeS(lap, activity.samples ?? []))}</td>
                <td>{optionalPace(lapPaceSPKM(lap, activity.samples ?? []))}</td>
                {showGap && <td>{optionalPace(lap.avgGradeAdjustedPaceSPKM)}</td>}
                {showElevation && <td>{optionalMeters(lap.elevationGainM)}</td>}
                {showElevation && <td>{optionalMeters(lap.elevationLossM)}</td>}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function intervalCategoryLabel(category: string, sportType: string) {
  switch (category.toLowerCase()) {
    case "warmup": return "Warm Up";
    case "active": return isRunningSport(sportType) ? "Run" : "Active";
    case "recovery": return "Recovery";
    case "cooldown": return "Cool Down";
    default: return category.replace(/(^|[-_])([a-z])/g, (_, prefix: string, letter: string) => `${prefix ? " " : ""}${letter.toUpperCase()}`);
  }
}

function isRunningSport(sportType: string) {
  return /run|walk|hike/i.test(sportType);
}

function intervalStepLabel(interval: ActivityInterval, sportType: string) {
  const category = intervalCategoryLabel(interval.category, sportType);
  if (interval.workoutRepeatIndex !== undefined && (interval.category === "active" || interval.category === "recovery")) {
    return `${interval.workoutRepeatIndex}. ${category}`;
  }
  return category;
}

function formatLapRange(lapIndexes?: number[]) {
  if (!lapIndexes || lapIndexes.length === 0) {
    return "";
  }
  const first = lapIndexes[0] + 1;
  const last = lapIndexes[lapIndexes.length - 1] + 1;
  return first === last ? String(first) : `${first}–${last}`;
}

function intervalCumulativeTime(interval: ActivityInterval, intervals: ActivityInterval[], activityStart: string) {
  if (interval.endTime) {
    const end = Date.parse(interval.endTime);
    const start = Date.parse(activityStart);
    if (Number.isFinite(end) && Number.isFinite(start) && end >= start) {
      return Math.round((end - start) / 1000);
    }
  }
  const index = intervals.findIndex((candidate) => candidate.index === interval.index);
  return intervals.slice(0, index + 1).reduce((total, candidate) => total + candidate.elapsedTimeS, 0);
}

function lapCumulativeTime(lap: ActivityLap, laps: ActivityLap[], activityStart: string) {
  if (lap.startTime) {
    const end = Date.parse(lap.startTime) + lap.elapsedTimeS * 1000;
    const start = Date.parse(activityStart);
    if (Number.isFinite(end) && Number.isFinite(start) && end >= start) {
      return Math.round((end - start) / 1000);
    }
  }
  return laps.filter((candidate) => candidate.index <= lap.index).reduce((total, candidate) => total + candidate.elapsedTimeS, 0);
}

function intervalTargetLabel(workout: Activity["workout"], interval: ActivityInterval) {
  if (!workout?.steps) {
    return "";
  }
  const stepType = interval.category === "active" ? "interval" : interval.category;
  const step = flattenWorkoutSteps(workout.steps).find((candidate) => candidate.type?.toLowerCase() === stepType);
  if (!step) {
    return "";
  }
  if (step.targetType?.toLowerCase() === "pace.zone" && step.targetValueOne !== undefined && step.targetValueTwo !== undefined) {
    const paces = [speedToPaceSPKM(step.targetValueOne), speedToPaceSPKM(step.targetValueTwo)].filter((pace): pace is number => pace !== undefined).sort((left, right) => left - right);
    return paces.length === 2 ? `Target ${formatPace(paces[0])}–${formatPace(paces[1])}` : "";
  }
  if (step.endCondition?.toLowerCase() === "time" && step.endConditionValue !== undefined) {
    return `Target ${formatDuration(step.endConditionValue)}`;
  }
  return "";
}

function flattenWorkoutSteps(steps?: ActivityWorkoutStep[]) {
  const flattened: ActivityWorkoutStep[] = [];
  const visit = (items?: ActivityWorkoutStep[]) => {
    for (const item of items ?? []) {
      flattened.push(item);
      visit(item.children);
    }
  };
  visit(steps);
  return flattened;
}

function optionalPace(value?: number) {
  return value !== undefined ? formatPace(value) : "";
}

function optionalBPM(value?: number) {
  return value !== undefined ? formatBPM(value) : "";
}

function optionalMeters(value?: number) {
  return value !== undefined ? `${Math.round(value).toLocaleString()} m` : "";
}

function optionalCadence(value?: number) {
  return value !== undefined ? `${Math.round(value).toLocaleString()} spm` : "";
}

function optionalMilliseconds(value?: number) {
  return value !== undefined ? `${Math.round(value).toLocaleString()} ms` : "";
}

function optionalWatts(value?: number) {
  return value !== undefined ? `${Math.round(value).toLocaleString()} W` : "";
}

function PlannedActivityMatchPanel({
  data,
  loading,
  error,
  matching,
  retrying,
  feedbackAvailable,
  canLoadMore,
  windowDays,
  onMatchHint,
  onOpenMatch,
  onOpenCheckIn,
  onLoadMore,
  onRetry
}: {
  data?: PlannedActivityMatchResponse;
  loading: boolean;
  error: unknown;
  matching: boolean;
  retrying: boolean;
  feedbackAvailable: boolean;
  canLoadMore: boolean;
  windowDays: number;
  onMatchHint: (plannedActivityId: string) => void;
  onOpenMatch: () => void;
  onOpenCheckIn: () => void;
  onLoadMore: () => void;
  onRetry: () => void;
}) {
  const [expanded, setExpanded] = useState(!Boolean(data?.matched));
  const matched = Boolean(data?.matched);

  useEffect(() => {
    setExpanded(!matched);
  }, [matched]);

  if (loading) return null;
  if (error) return <div className="error">{error instanceof Error ? error.message : "Could not load planned activity matches"}</div>;
  if (!data) return null;
  if (!data.matched && data.candidates.length === 0 && !canLoadMore && windowDays === 7) return null;
  if (data.matched) {
    return (
      <section className="panel planned-match-panel">
        <div className="notes-panel-header">
          <button
            className="panel-collapse-toggle"
            type="button"
            aria-expanded={expanded}
            onClick={() => setExpanded((current) => !current)}
          >
            <ChevronDown size={17} aria-hidden="true" />
            <div>
            <div className="panel-heading">Matched planned run</div>
            <strong>{data.matched.name}</strong>
            </div>
          </button>
          <button className="secondary-button small-button" type="button" onClick={onOpenCheckIn}>
            {feedbackAvailable ? "RPE & feedback" : "RPE"}
          </button>
        </div>
        {expanded && (
          <div className="planned-match-panel-content">
            {data.matched.notes && <p className="muted">{data.matched.notes}</p>}
            {data.writeback && (
              <div className="writeback-status">
                <p className="muted">Sheet write-back</p>
                <div>Summary: {writebackStatusLabel(data.writeback.summaryStatus)}</div>
                {data.writeback.summaryError && <div className="muted">{data.writeback.summaryError}</div>}
                <div>Structured intervals: {writebackStatusLabel(data.writeback.intervalsStatus)}</div>
                {data.writeback.intervalsError && <div className="muted">{data.writeback.intervalsError}</div>}
                <div>How it felt/go: {writebackStatusLabel(data.writeback.feedbackStatus)}</div>
                {data.writeback.feedbackError && <div className="muted">{data.writeback.feedbackError}</div>}
                {data.writeback.jobId && data.writeback.jobStatus === "running" && <SyncJobCancelButton job={{ id: data.writeback.jobId, status: data.writeback.jobStatus, cancelRequestedAt: data.writeback.cancelRequestedAt }} compact />}
                {(data.writeback.summaryStatus === "failed" || data.writeback.summaryStatus === "canceled" || data.writeback.summaryStatus === "completed_with_conflicts" || data.writeback.intervalsStatus === "failed" || data.writeback.intervalsStatus === "canceled" || data.writeback.intervalsStatus === "completed_with_conflicts" || data.writeback.feedbackStatus === "failed" || data.writeback.feedbackStatus === "canceled" || data.writeback.feedbackStatus === "completed_with_conflicts") && (
                  <button className="secondary-button small-button" type="button" disabled={retrying} onClick={onRetry}>
                    {retrying ? "Retrying..." : "Retry write-back"}
                  </button>
                )}
              </div>
            )}
          </div>
        )}
      </section>
    );
  }
  const hintedCandidate = data.candidates[0];
  return (
    <section className="panel planned-match-panel">
      <div className="notes-panel-header">
        <button
          className="panel-collapse-toggle"
          type="button"
          aria-expanded={expanded}
          onClick={() => setExpanded((current) => !current)}
        >
          <ChevronDown size={17} aria-hidden="true" />
          <div>
            <div className="panel-heading">{hintedCandidate ? "Suggested planned run" : "Find planned run"}</div>
            {hintedCandidate ? <strong>{hintedCandidate.name}</strong> : <span className="muted">No planned run was found within {windowDays} days.</span>}
          </div>
        </button>
        {hintedCandidate && (
          <div className="notes-actions">
            <button className="primary-button small-button" type="button" disabled={matching} onClick={() => onMatchHint(hintedCandidate.id)}>
              {matching ? "Matching..." : "Match"}
            </button>
            {(data.candidates.length > 1 || canLoadMore) && (
              <button className="secondary-button small-button" type="button" disabled={matching} onClick={onOpenMatch}>
                Other options
              </button>
            )}
          </div>
        )}
        {!hintedCandidate && canLoadMore && (
          <button className="secondary-button small-button" type="button" onClick={onLoadMore}>
            Load more plans
          </button>
        )}
      </div>
      {expanded && hintedCandidate?.notes && <p className="muted">{hintedCandidate.notes}</p>}
    </section>
  );
}

function PlannedActivityMatchDialog({
  data,
  selectedCandidateId,
  canLoadMore,
  matching,
  error,
  preview,
  onSelectCandidate,
  onPreview,
  onApply,
  onPreviewReset,
  onLoadMore,
  onClose
}: {
  data: PlannedActivityMatchResponse;
  selectedCandidateId?: string;
  canLoadMore: boolean;
  matching: boolean;
  error: unknown;
  preview?: TrainingSheetWritebackPreview;
  onSelectCandidate: (plannedActivityId: string) => void;
  onPreview: (input: PlannedMatchDraft) => void;
  onApply: (input: PlannedMatchDraft) => void;
  onPreviewReset: () => void;
  onLoadMore: () => void;
  onClose: () => void;
}) {
  const [feedback, setFeedback] = useState("");
  const [rpe, setRPE] = useState(5);
  const [rpeTouched, setRPETouched] = useState(true);
  const [overrides, setOverrides] = useState<Record<string, string>>({});
  const trimmedFeedback = feedback.trim();
  const valid = Array.from(trimmedFeedback).length <= 5000;
  const selectedCandidate = data.candidates.find((candidate) => candidate.id === selectedCandidateId);
  const feedbackAvailable = Boolean(selectedCandidate?.feedbackCell?.trim());
  const draft = (): PlannedMatchDraft => ({
    plannedActivityId: selectedCandidateId ?? "",
    feedback: feedbackAvailable ? trimmedFeedback : undefined,
    rpe: rpeTouched ? rpe : null,
    rpeSet: rpeTouched,
    overrides: Object.keys(overrides).length > 0 ? overrides : undefined
  });

  useEffect(() => {
    setFeedback("");
    setRPE(5);
    setRPETouched(true);
    setOverrides({});
    onPreviewReset();
  }, [selectedCandidateId]);

  const resetPreview = () => {
    setOverrides({});
    onPreviewReset();
  };

  return (
    <div className="dialog-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <form
        className="filter-dialog notes-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="planned-match-title"
        onSubmit={(event) => {
          event.preventDefault();
          if (!selectedCandidateId || !valid) {
            return;
          }
          if (preview) {
            onApply(draft());
          } else {
            onPreview(draft());
          }
        }}
      >
        <div className="dialog-header">
          <div>
            <div className="eyebrow">Activity</div>
            <h2 id="planned-match-title">Match planned run</h2>
          </div>
          <button className="icon-button" type="button" aria-label="Close planned run matching" onClick={onClose}>
            <X size={16} />
          </button>
        </div>
        <p className="muted">Review the sheet changes before matching and writing them back.</p>
        <div className="planned-match-candidates">
          {(data.candidates ?? []).map((candidate) => (
            <label className="planned-match-candidate" key={candidate.id}>
              <input
                type="radio"
                name="planned-activity"
                checked={candidate.id === selectedCandidateId}
                disabled={matching}
                onChange={() => onSelectCandidate(candidate.id)}
              />
              <div>
                <div className="planned-match-candidate-title">
                  <strong>{candidate.name}</strong>
                  {candidate.id === data.suggestedId && <span className="planned-match-badge">Suggested</span>}
                </div>
                <div className="planned-match-candidate-meta">{formatDate(candidate.plannedDate)}</div>
                {candidate.notes && <p className="muted">{candidate.notes}</p>}
              </div>
            </label>
          ))}
        </div>
        {data.candidates.length === 0 && <p className="muted">No planned runs were found for this date.</p>}
        {selectedCandidate && (
          <>
            <label className="field">
              <span>RPE <strong>{rpe}/10</strong></span>
              <input
                className="rpe-slider"
                type="range"
                min={1}
                max={10}
                step={1}
                value={rpe}
                aria-label="Rate of perceived exertion"
                onChange={(event) => {
                  setRPE(Number(event.target.value));
                  setRPETouched(true);
                  resetPreview();
                }}
              />
            </label>
            {feedbackAvailable && (
              <label className="field">
                <span>How did it feel/go?</span>
                <textarea className="notes-textarea" maxLength={5000} rows={6} value={feedback} onChange={(event) => { setFeedback(event.target.value); resetPreview(); }} />
              </label>
            )}
            {!feedbackAvailable && selectedCandidate && (
              <label className="field">
                <span>How did it feel/go?</span>
                <textarea className="notes-textarea" maxLength={5000} rows={6} value="" disabled aria-describedby="planned-match-feedback-disabled" />
                <span id="planned-match-feedback-disabled" className="muted">Feedback was not requested for this planned activity.</span>
              </label>
            )}
          </>
        )}
        {preview && <TrainingSheetPreviewPanel preview={preview} overrides={overrides} onOverrideChange={(ref, value) => setOverrides((current) => ({ ...current, [ref]: value }))} />}
        {!valid && <div className="row-error">Feedback must be 5000 characters or fewer.</div>}
        {error instanceof Error && <div className="error">{error.message}</div>}
        <div className="dialog-actions">
          {canLoadMore && (
            <button className="secondary-button" type="button" disabled={matching} onClick={onLoadMore}>Load more plans</button>
          )}
          {preview && (
            <button className="secondary-button" type="button" disabled={matching} onClick={resetPreview}>Edit</button>
          )}
          <button className="secondary-button" type="button" disabled={matching} onClick={onClose}>Cancel</button>
          <button className="primary-button" type="submit" disabled={matching || !selectedCandidateId || !valid}>
            {matching ? (preview ? "Applying..." : "Building preview...") : (preview ? "Apply match & write back" : "Preview changes")}
          </button>
        </div>
      </form>
    </div>
  );
}

function TrainingSheetPreviewPanel({ preview, overrides, onOverrideChange }: { preview: TrainingSheetWritebackPreview; overrides: Record<string, string>; onOverrideChange: (ref: string, value: string) => void }) {
  const [selectedRef, setSelectedRef] = useState<string>();
  const grid = preview.grid;
  const selectedCell = grid?.rows.flatMap((row) => row.cells).find((cell) => cell.ref === selectedRef);
  const effectiveSelectedCell = selectedCell ? trainingSheetPreviewCellWithOverride(selectedCell, overrides) : undefined;
  const effectiveChanges = preview.changes.map((change) => trainingSheetPreviewChangeWithOverride(change, overrides));
  const writeCount = effectiveChanges.filter((change) => change.status === "write" || change.status === "manual").length;
  const conflictCount = effectiveChanges.filter((change) => change.status === "conflict").length;

  useEffect(() => {
    setSelectedRef(undefined);
  }, [preview.fingerprint]);

  return (
    <section className="training-sheet-preview" aria-label="Training sheet preview">
      <div className="training-sheet-preview-header">
        <div>
          <div className="panel-heading">Sheet preview</div>
          <strong>{preview.sheetTitle}</strong>
        </div>
        {preview.sheetUrl && <a href={preview.sheetUrl} target="_blank" rel="noreferrer">Open sheet</a>}
      </div>
      <div className="muted">
        {writeCount} cell{writeCount === 1 ? "" : "s"} will be written{conflictCount > 0 ? ` · ${conflictCount} existing value${conflictCount === 1 ? "" : "s"} preserved` : ""}
      </div>
      <div className="training-sheet-preview-legend" aria-label="Preview legend">
        <span><i className="training-sheet-preview-swatch write" /> Will write</span>
        <span><i className="training-sheet-preview-swatch conflict" /> Existing value preserved</span>
        <span><i className="training-sheet-preview-swatch manual" /> Manual override</span>
      </div>
      <div className="muted">Click a proposed cell to edit its value. Edited conflicts will replace the existing sheet value.</div>
      {preview.warnings?.map((warning) => <div className="training-sheet-preview-warning" key={warning}>{warning}</div>)}
      {grid?.rows.length ? (
        <>
          <div className="training-sheet-preview-grid-wrap">
            <table className="training-sheet-grid">
              <colgroup>
                <col className="training-sheet-grid-row-number-column" />
                {grid.columns.map((column) => <col key={column.label} style={column.widthPx ? { width: `${column.widthPx}px` } : undefined} />)}
              </colgroup>
              <thead>
                <tr>
                  <th className="training-sheet-grid-corner" aria-label="Sheet corner" />
                  {grid.columns.map((column) => <th key={column.label} scope="col">{column.label}</th>)}
                </tr>
              </thead>
              <tbody>
                {grid.rows.map((row) => (
                  <tr key={row.index} style={row.heightPx ? { height: `${row.heightPx}px` } : undefined}>
                    <th className="training-sheet-grid-row-number" scope="row">{row.index}</th>
                    {row.cells.map((cell) => {
                      const displayCell = trainingSheetPreviewCellWithOverride(cell, overrides);
                      const selected = cell.ref === selectedRef;
                      return (
                        <td
                          key={cell.ref}
                          className={`training-sheet-grid-cell-container ${displayCell.status} ${selected ? "selected" : ""}`}
                          rowSpan={cell.rowSpan}
                          colSpan={cell.columnSpan}
                        >
                          <button
                            className="training-sheet-grid-cell"
                            type="button"
                            style={trainingSheetCellInlineStyle(cell)}
                            aria-label={trainingSheetCellAriaLabel(displayCell)}
                            title={trainingSheetCellAriaLabel(displayCell)}
                            onClick={() => setSelectedRef(cell.ref)}
                          >
                            {displayCell.displayValue || "\u00a0"}
                          </button>
                        </td>
                      );
                    })}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {effectiveSelectedCell && <TrainingSheetPreviewCellInspector cell={effectiveSelectedCell} onOverrideChange={onOverrideChange} />}
        </>
      ) : (
        <div className="muted">No sheet values are available to preview.</div>
      )}
    </section>
  );
}

function trainingSheetPreviewCellWithOverride(cell: TrainingSheetPreviewCell, overrides: Record<string, string>): TrainingSheetPreviewCell {
  if (!Object.prototype.hasOwnProperty.call(overrides, cell.ref) || !cell.section) {
    return cell;
  }
  const value = overrides[cell.ref];
  return { ...cell, displayValue: value, proposedValue: value, status: cell.currentValue === value ? "unchanged" : "manual" };
}

function trainingSheetPreviewChangeWithOverride(change: TrainingSheetPreviewChange, overrides: Record<string, string>): TrainingSheetPreviewChange {
  const separator = change.range.lastIndexOf("!");
  const ref = (separator >= 0 ? change.range.slice(separator + 1) : change.range).replace(/\$/g, "").toUpperCase();
  if (!Object.prototype.hasOwnProperty.call(overrides, ref)) {
    return change;
  }
  const proposedValue = overrides[ref];
  return { ...change, proposedValue, status: change.currentValue === proposedValue ? "unchanged" : "manual" };
}

function TrainingSheetPreviewCellInspector({ cell, onOverrideChange }: { cell: TrainingSheetPreviewCell; onOverrideChange: (ref: string, value: string) => void }) {
  const changed = cell.status !== "unchanged";
  const editable = Boolean(cell.section);
  return (
    <div className="training-sheet-preview-inspector" aria-live="polite">
      <div className="training-sheet-preview-inspector-heading">
        <div>
          <strong>{cell.ref}</strong>
          {cell.label && <span>{cell.label}</span>}
          {cell.section && <span className="muted">{cell.section}</span>}
        </div>
        <span className={`training-sheet-preview-status-badge ${cell.status}`}>{trainingSheetPreviewStatusLabel(cell.status)}</span>
      </div>
      <div className="training-sheet-preview-inspector-values">
        <div><span>Current</span><strong>{cell.currentValue || "(blank)"}</strong></div>
        {editable ? (
          <label className="training-sheet-preview-edit-value">
            <span>Proposed</span>
            <input value={cell.proposedValue ?? cell.displayValue} onChange={(event) => onOverrideChange(cell.ref, event.target.value)} />
          </label>
        ) : changed && <div><span>Proposed</span><strong>{cell.proposedValue || "(blank)"}</strong></div>}
      </div>
    </div>
  );
}

function trainingSheetCellInlineStyle(cell: TrainingSheetPreviewCell): CSSProperties {
  const style = cell.style;
  return {
    backgroundColor: cell.status === "unchanged" ? style?.backgroundColor : undefined,
    color: style?.textColor,
    fontWeight: style?.bold ? 700 : undefined,
    fontStyle: style?.italic ? "italic" : undefined,
    fontSize: style?.fontSize ? `${style.fontSize}px` : undefined,
    textAlign: trainingSheetTextAlignment(style?.horizontalAlignment),
    verticalAlign: trainingSheetVerticalAlignment(style?.verticalAlignment),
    whiteSpace: style?.wrapStrategy === "CLIP" ? "nowrap" : "pre-wrap"
  };
}

function trainingSheetTextAlignment(value?: string): CSSProperties["textAlign"] {
  switch (value) {
    case "LEFT": return "left";
    case "CENTER": return "center";
    case "RIGHT": return "right";
    default: return undefined;
  }
}

function trainingSheetVerticalAlignment(value?: string): CSSProperties["verticalAlign"] {
  switch (value) {
    case "TOP": return "top";
    case "MIDDLE": return "middle";
    case "BOTTOM": return "bottom";
    default: return undefined;
  }
}

function trainingSheetCellAriaLabel(cell: TrainingSheetPreviewCell) {
  const current = cell.currentValue || "blank";
  if (cell.status === "unchanged") {
    return `${cell.ref}: ${current}`;
  }
  return `${cell.ref}: ${current} to ${cell.proposedValue || "blank"}; ${trainingSheetPreviewStatusLabel(cell.status)}`;
}

function trainingSheetPreviewStatusLabel(status: "write" | "conflict" | "unchanged" | "manual") {
  switch (status) {
    case "write": return "will write";
    case "conflict": return "existing value preserved";
    case "unchanged": return "unchanged";
    case "manual": return "manual override";
  }
}

function writebackStatusLabel(status: string) {
  switch (status) {
    case "completed":
      return "written";
    case "completed_with_conflicts":
      return "existing values preserved";
    case "completed_with_warnings":
      return "written with warnings";
    case "skipped":
      return "skipped; review needed";
    case "not_provided":
      return "not provided";
    case "not_applicable":
      return "not applicable";
    case "running":
      return "writing...";
    case "canceled":
      return "canceled";
    case "failed":
      return "failed";
    default:
      return status;
  }
}

function ActivityNotesPanel({
  notes,
  saving,
  onEdit,
  onDelete
}: {
  notes: string;
  saving: boolean;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const hasNotes = notes.trim().length > 0;
  if (!hasNotes) {
    return null;
  }
  return (
    <section className="panel notes-panel">
      <div className="notes-panel-header">
        <div className="panel-heading">Notes</div>
        <div className="notes-actions">
          <button className="secondary-button small-button" type="button" disabled={saving} onClick={onEdit}>
            <Pencil size={15} />
            Edit
          </button>
          <button className="secondary-button small-button danger-text-button" type="button" disabled={saving} onClick={onDelete}>
            <Trash2 size={15} />
            Delete
          </button>
        </div>
      </div>
      <div className="notes-body">{notes}</div>
    </section>
  );
}

function ActivityReflectionDialog({
  activity,
  feedbackAvailable,
  saving,
  error,
  onSave,
  onClose
}: {
  activity: Activity;
  feedbackAvailable: boolean;
  saving: boolean;
  error: unknown;
  onSave: (feedback: string, rpe: number | null) => void;
  onClose: () => void;
}) {
  const [feedback, setFeedback] = useState(activity.feedback ?? "");
  const [rpe, setRPE] = useState(activity.rpe ?? 5);
  const trimmedFeedback = feedback.trim();
  const currentFeedback = (activity.feedback ?? "").trim();
  const currentRPE = activity.rpe ?? null;
  const valid = Array.from(trimmedFeedback).length <= 5000;
  const changed = trimmedFeedback !== currentFeedback || rpe !== currentRPE;
  const message = error instanceof Error ? error.message : "";

  return (
    <div className="dialog-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <form
        className="filter-dialog notes-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="activity-reflection-title"
        onSubmit={(event) => {
          event.preventDefault();
          if (valid && changed) {
            onSave(feedbackAvailable ? trimmedFeedback : currentFeedback, rpe);
          }
        }}
      >
        <div className="dialog-header">
          <div>
            <div className="eyebrow">Activity</div>
            <h2 id="activity-reflection-title">{feedbackAvailable ? "RPE & feedback" : "RPE"}</h2>
          </div>
          <button className="icon-button" type="button" aria-label="Close RPE and feedback" onClick={onClose}>
            <X size={16} />
          </button>
        </div>
        <label className="field">
          <span>RPE <strong>{rpe}/10</strong></span>
          <input
            className="rpe-slider"
            type="range"
            min={1}
            max={10}
            step={1}
            value={rpe}
            aria-label="Rate of perceived exertion"
            onChange={(event) => setRPE(Number(event.target.value))}
          />
        </label>
        {feedbackAvailable && (
          <label className="field">
            <span>How did it feel/go?</span>
            <textarea className="notes-textarea" maxLength={5000} rows={8} value={feedback} onChange={(event) => setFeedback(event.target.value)} />
          </label>
        )}
        {!valid && <div className="row-error">Feedback must be 5000 characters or fewer.</div>}
        {message && <div className="error">{message}</div>}
        <div className="dialog-actions">
          <button className="secondary-button" type="button" disabled={saving} onClick={onClose}>Cancel</button>
          <button className="primary-button" type="submit" disabled={saving || !valid || !changed}>{saving ? "Saving..." : "Save"}</button>
        </div>
      </form>
    </div>
  );
}

function ActivityNotesDialog({
  activity,
  saving,
  error,
  onSave,
  onClose
}: {
  activity: Activity;
  saving: boolean;
  error: unknown;
  onSave: (notes: string) => void;
  onClose: () => void;
}) {
  const [notes, setNotes] = useState(activity.notes ?? "");
  const trimmedNotes = notes.trim();
  const currentNotes = (activity.notes ?? "").trim();
  const valid = Array.from(trimmedNotes).length <= 5000;
  const changed = trimmedNotes !== currentNotes;
  const message = error instanceof Error ? error.message : "";

  return (
    <div
      className="dialog-backdrop"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <form
        className="filter-dialog notes-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="activity-notes-title"
        onSubmit={(event) => {
          event.preventDefault();
          if (valid && changed) {
            onSave(trimmedNotes);
          }
        }}
      >
        <div className="dialog-header">
          <div>
            <div className="eyebrow">Activity</div>
            <h2 id="activity-notes-title">Notes</h2>
          </div>
          <button className="icon-button" type="button" aria-label="Close notes" onClick={onClose}>
            <X size={16} />
          </button>
        </div>

        <label className="field">
          <span>Notes</span>
          <textarea
            autoFocus
            className="notes-textarea"
            maxLength={5000}
            rows={8}
            value={notes}
            onChange={(event) => setNotes(event.target.value)}
          />
        </label>
        {!valid && <div className="row-error">Notes must be 5000 characters or fewer.</div>}
        {message && <div className="error">{message}</div>}

        <div className="dialog-actions">
          {currentNotes && (
            <button className="secondary-button" type="button" disabled={saving} onClick={() => onSave("")}>
              Clear note
            </button>
          )}
          <button className="secondary-button" type="button" disabled={saving} onClick={onClose}>
            Cancel
          </button>
          <button className="primary-button" type="submit" disabled={saving || !valid || !changed}>
            Save
          </button>
        </div>
      </form>
    </div>
  );
}

function ActivityExportGPXDialog({
  activity,
  onClose
}: {
  activity: Activity;
  onClose: () => void;
}) {
  const [includeSensors, setIncludeSensors] = useState(false);
  const handleDownload = () => {
    const link = document.createElement("a");
    link.href = activityGPXURL(activity.id, includeSensors);
    document.body.appendChild(link);
    link.click();
    link.remove();
    onClose();
  };

  return (
    <div
      className="dialog-backdrop"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <section className="filter-dialog export-gpx-dialog" role="dialog" aria-modal="true" aria-labelledby="activity-export-gpx-title">
        <div className="dialog-header">
          <div>
            <div className="eyebrow">Activity</div>
            <h2 id="activity-export-gpx-title">Export GPX</h2>
          </div>
          <button className="icon-button" type="button" aria-label="Close GPX export" onClick={onClose}>
            <X size={16} />
          </button>
        </div>

        <label className="checkbox-field">
          <input
            type="checkbox"
            checked={includeSensors}
            onChange={(event) => setIncludeSensors(event.target.checked)}
          />
          <span>Include sensors</span>
        </label>

        <div className="dialog-actions">
          <button className="secondary-button" type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="primary-button" type="button" onClick={handleDownload}>
            <Download size={16} />
            Download
          </button>
        </div>
      </section>
    </div>
  );
}

function ActivityMediaUploadAction({
  inputKey,
  uploading,
  onFilesSelected
}: {
  inputKey: number;
  uploading: boolean;
  onFilesSelected: (files: File[]) => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);

  return (
    <>
      <button className="secondary-button" type="button" disabled={uploading} onClick={() => inputRef.current?.click()}>
        <Upload size={16} />
        {uploading ? "Uploading" : "Add photos"}
      </button>
      <input
        key={inputKey}
        ref={inputRef}
        className="media-hidden-input"
        type="file"
        accept="image/jpeg,image/png"
        multiple
        disabled={uploading}
        onChange={(event) => {
          const files = Array.from(event.target.files ?? []);
          event.currentTarget.value = "";
          onFilesSelected(files);
        }}
      />
    </>
  );
}

function ActivityMediaPanel({
  activity,
  uploading,
  uploadError,
  selectedMediaId,
  onSelectMedia
}: {
  activity: Activity;
  uploading: boolean;
  uploadError: unknown;
  selectedMediaId?: string;
  onSelectMedia: (mediaId?: string) => void;
}) {
  const queryClient = useQueryClient();
  const media = activity.media ?? [];
  const previewMedia = media.find((item) => item.id === selectedMediaId);
  const previewMediaIndex = selectedMediaId ? media.findIndex((item) => item.id === selectedMediaId) : -1;
  const previousMedia = previewMediaIndex > 0 ? media[previewMediaIndex - 1] : undefined;
  const nextMedia = previewMediaIndex >= 0 && previewMediaIndex < media.length - 1 ? media[previewMediaIndex + 1] : undefined;
  const deleteMedia = useMutation({
    mutationFn: (mediaId: string) => api.deleteActivityMedia(activity.id, mediaId),
    onSuccess: (_result, mediaId) => {
      queryClient.setQueryData<{ activity: Activity }>(["activity", activity.id], (current) => {
        if (!current) {
          return current;
        }
        return {
          activity: {
            ...current.activity,
            media: (current.activity.media ?? []).filter((item) => item.id !== mediaId)
          }
        };
      });
    }
  });
  const mediaCountLabel = media.length === 1 ? "1 photo" : `${media.length} photos`;
  const handleDeleteMedia = (item: ActivityMedia) => {
    deleteMedia.reset();
    if (window.confirm(`Delete "${item.originalFilename}" from this activity?`)) {
      if (selectedMediaId === item.id) {
        const itemIndex = media.findIndex((candidate) => candidate.id === item.id);
        const replacement = media[itemIndex + 1] ?? media[itemIndex - 1];
        onSelectMedia(replacement?.id);
      }
      deleteMedia.mutate(item.id);
    }
  };

  return (
    <section className="panel media-panel">
      <div className="media-panel-header">
        <div className="panel-heading">Media</div>
        <span className="media-count">{mediaCountLabel}</span>
      </div>

      {uploading && <div className="media-upload-status"><Upload size={16} /> Uploading photos</div>}
      {Boolean(uploadError) && <div className="error">{uploadError instanceof Error ? uploadError.message : "Upload failed"}</div>}
      {deleteMedia.error && <div className="error">{deleteMedia.error instanceof Error ? deleteMedia.error.message : "Delete failed"}</div>}

      {media.length > 0 ? (
        <div className="media-grid">
          {media.map((item) => (
            <button className="media-thumb-button" key={item.id} type="button" aria-label={`Open ${item.originalFilename}`} onClick={() => onSelectMedia(item.id)}>
              <img src={activityMediaThumbnailURL(item.id)} alt={item.originalFilename} loading="lazy" />
            </button>
          ))}
        </div>
      ) : (
        <EmptyState title="No photos attached" />
      )}

      {previewMedia && (
        <ActivityMediaPreview
          media={previewMedia}
          deleting={deleteMedia.isPending}
          onClose={() => onSelectMedia(undefined)}
          onDelete={() => handleDeleteMedia(previewMedia)}
          onPrevious={previousMedia ? () => onSelectMedia(previousMedia.id) : undefined}
          onNext={nextMedia ? () => onSelectMedia(nextMedia.id) : undefined}
        />
      )}
    </section>
  );
}

function ActivityMediaPreview({
  media,
  deleting,
  onClose,
  onDelete,
  onPrevious,
  onNext
}: {
  media: ActivityMedia;
  deleting: boolean;
  onClose: () => void;
  onDelete: () => void;
  onPrevious?: () => void;
  onNext?: () => void;
}) {
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
        return;
      }
      if (event.key === "ArrowLeft" && onPrevious) {
        event.preventDefault();
        onPrevious();
      }
      if (event.key === "ArrowRight" && onNext) {
        event.preventDefault();
        onNext();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onClose, onNext, onPrevious]);

  return (
    <div
      className="dialog-backdrop media-preview-backdrop"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <section className="filter-dialog media-preview-dialog" role="dialog" aria-modal="true" aria-labelledby="activity-media-preview-title">
        <div className="dialog-header">
          <div>
            <div className="eyebrow">Media</div>
            <h2 id="activity-media-preview-title">{media.originalFilename}</h2>
          </div>
          <button className="icon-button" type="button" aria-label="Close media preview" onClick={onClose}>
            <X size={16} />
          </button>
        </div>

        <div className="media-preview-stage">
          <button className="icon-button media-preview-nav" type="button" aria-label="Previous photo" disabled={!onPrevious} onClick={onPrevious}>
            <ChevronLeft size={20} />
          </button>
          <div className="media-preview-image">
            <img src={activityMediaOriginalURL(media.id)} alt={media.originalFilename} />
          </div>
          <button className="icon-button media-preview-nav" type="button" aria-label="Next photo" disabled={!onNext} onClick={onNext}>
            <ChevronRight size={20} />
          </button>
        </div>
        <div className="media-preview-meta">
          <span>{formatActivityMediaMeta(media)}</span>
          {hasMediaLocation(media) && <span>{formatMediaLocation(media)}</span>}
        </div>

        <div className="dialog-actions">
          <a className="secondary-button" href={activityMediaOriginalURL(media.id)} target="_blank" rel="noreferrer">
            <ExternalLink size={16} />
            Open original
          </a>
          <button className="danger-button" type="button" disabled={deleting} onClick={onDelete}>
            <Trash2 size={16} />
            Delete
          </button>
        </div>
      </section>
    </div>
  );
}

function ActivityDetailActions({
  activity,
  open,
  deleting,
  canExportGPX,
  canUnmatchPlanned,
  unmatching,
  onToggle,
  onRename,
  onNotes,
  onExportGPX,
  onUnmatchPlanned,
  onDelete
}: {
  activity: Activity;
  open: boolean;
  deleting: boolean;
  canExportGPX: boolean;
  canUnmatchPlanned: boolean;
  unmatching: boolean;
  onToggle: () => void;
  onRename: () => void;
  onNotes: () => void;
  onExportGPX: () => void;
  onUnmatchPlanned: () => void;
  onDelete: () => void;
}) {
  return (
    <div className="action-menu-wrap">
      <button className="icon-button" type="button" aria-label="Activity actions" aria-expanded={open} onClick={onToggle}>
        <MoreVertical size={18} />
      </button>
      {open && (
        <div className="action-menu" role="menu">
          <button className="action-menu-item" type="button" role="menuitem" onClick={onRename}>
            <Pencil size={16} />
            Rename
          </button>
          <button className="action-menu-item" type="button" role="menuitem" onClick={onNotes}>
            <StickyNote size={16} />
            {(activity.notes ?? "").trim() ? "Edit note" : "Add note"}
          </button>
          <button className="action-menu-item" type="button" role="menuitem" disabled={!canExportGPX} onClick={onExportGPX}>
            <Download size={16} />
            Export GPX
          </button>
          {activity.originalProviderUrl && (
            <a className="action-menu-item" role="menuitem" href={activity.originalProviderUrl} target="_blank" rel="noreferrer">
              <ExternalLink size={16} />
              Open original
            </a>
          )}
          {canUnmatchPlanned && (
            <button className="action-menu-item" type="button" role="menuitem" disabled={unmatching} onClick={onUnmatchPlanned}>
              <RotateCcw size={16} />
              {unmatching ? "Unmatching..." : "Unmatch planned run"}
            </button>
          )}
          <button className="action-menu-item danger" type="button" role="menuitem" disabled={deleting} onClick={onDelete}>
            <Trash2 size={16} />
            Delete
          </button>
        </div>
      )}
    </div>
  );
}

function ActivityRenameDialog({
  activity,
  saving,
  error,
  onSave,
  onClose
}: {
  activity: Activity;
  saving: boolean;
  error: unknown;
  onSave: (name: string) => void;
  onClose: () => void;
}) {
  const [name, setName] = useState(activity.name);
  const trimmedName = name.trim();
  const valid = trimmedName.length > 0 && Array.from(trimmedName).length <= 160;
  const changed = trimmedName !== activity.name;
  const canRestore = Boolean(activity.localName && activity.sourceName);
  const message = error instanceof Error ? error.message : "";

  return (
    <div
      className="dialog-backdrop"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <form
        className="filter-dialog rename-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="activity-rename-title"
        onSubmit={(event) => {
          event.preventDefault();
          if (valid && changed) {
            onSave(trimmedName);
          }
        }}
      >
        <div className="dialog-header">
          <div>
            <div className="eyebrow">Activity</div>
            <h2 id="activity-rename-title">Rename</h2>
          </div>
          <button className="icon-button" type="button" aria-label="Close rename" onClick={onClose}>
            <X size={16} />
          </button>
        </div>

        <label className="field">
          <span>Name</span>
          <input autoFocus type="text" maxLength={160} value={name} onChange={(event) => setName(event.target.value)} />
        </label>
        <p className="muted rename-note">This only changes the name in Runnarr. The original provider activity will not be renamed.</p>
        {canRestore && <div className="muted">Original: {activity.sourceName}</div>}
        {!valid && <div className="row-error">Name must be between 1 and 160 characters.</div>}
        {message && <div className="error">{message}</div>}

        <div className="dialog-actions">
          {canRestore && (
            <button className="secondary-button" type="button" disabled={saving} onClick={() => onSave(activity.sourceName)}>
              <RotateCcw size={16} />
              Restore original
            </button>
          )}
          <button className="secondary-button" type="button" disabled={saving} onClick={onClose}>
            Cancel
          </button>
          <button className="primary-button" type="submit" disabled={saving || !valid || !changed}>
            Save
          </button>
        </div>
      </form>
    </div>
  );
}

function ActivityClimbsPanel({
  climbs,
  selectedClimb,
  profileData,
  sensitivityControls,
  onSelect
}: {
  climbs: ActivityClimb[];
  selectedClimb?: ActivityClimb;
  profileData: ClimbProfilePoint[];
  sensitivityControls?: ReactNode;
  onSelect: (climb: ActivityClimb) => void;
}) {
  return (
    <section className="panel climbs-panel">
      <div className="chart-header climbs-panel-header">
        <div>
          <div className="panel-heading">Climbs</div>
          <span className="muted">{climbs.length.toLocaleString()} detected</span>
        </div>
      </div>
      {sensitivityControls && <div className="climb-sensitivity-panel-controls">{sensitivityControls}</div>}
      {climbs.length === 0 ? (
        <div className="muted">No climbs detected at this sensitivity.</div>
      ) : (
        <div className={`climbs-layout ${selectedClimb ? "" : "list-only"}`}>
          <div className="climb-list">
            {climbs.map((climb) => {
              const active = selectedClimb?.index === climb.index;
              return (
                <button key={climb.index} className={`climb-item ${active ? "active" : ""}`} type="button" aria-pressed={active} onClick={() => onSelect(climb)}>
                  <span className="climb-item-header">
                    <strong>Climb {climb.index + 1}</strong>
                    <span className={`climb-difficulty ${difficultyClass(climb.difficulty)}`}>{climb.difficulty}</span>
                  </span>
                  <span className="climb-item-metrics">
                    <span>{formatGrade(climb.avgGradePct)}</span>
                    <span>{formatDistance(climb.distanceM)}</span>
                    <span>{Math.round(climb.elevationGainM).toLocaleString()} m</span>
                  </span>
                  <span className="muted">{formatDistanceRange(climb.startDistanceM, climb.endDistanceM)}</span>
                </button>
              );
            })}
          </div>
          {selectedClimb && (
            <div className="climb-detail">
              <div className="climb-detail-metrics">
                <ClimbStat label="Difficulty" value={selectedClimb.difficulty} />
                <ClimbStat label="Avg Grade" value={formatGrade(selectedClimb.avgGradePct)} />
                <ClimbStat label="Distance" value={formatDistance(selectedClimb.distanceM)} />
                <ClimbStat label="Total Ascent" value={`${Math.round(selectedClimb.elevationGainM).toLocaleString()} m`} />
              </div>
              <div className="climb-profile">
                <ResponsiveContainer width="100%" height="100%">
                  <AreaChart data={profileData}>
                    <CartesianGrid strokeDasharray="3 3" vertical={false} />
                    <XAxis dataKey="label" minTickGap={26} />
                    <YAxis width={44} domain={[0, "dataMax"]} tickFormatter={(value) => String(Math.round(Number(value)))} />
                    <Tooltip
                      contentStyle={chartTooltipContentStyle}
                      labelStyle={chartTooltipLabelStyle}
                      formatter={(value) => [`${Math.round(Number(value)).toLocaleString()} m`, "Height above start"]}
                    />
                    <Area type="monotone" dataKey="elevationM" stroke="#b7791f" fill="#f6c432" fillOpacity={0.5} dot={false} />
                  </AreaChart>
                </ResponsiveContainer>
              </div>
            </div>
          )}
        </div>
      )}
    </section>
  );
}

function ClimbStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="climb-stat">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function SettingsPage({
  themePreference,
  onThemePreferenceChange,
  themePreferenceError
}: {
  themePreference: ThemePreference;
  onThemePreferenceChange: (preference: ThemePreference) => void;
  themePreferenceError?: Error | null;
}) {
  const location = useLocation();
  const queryClient = useQueryClient();
  const garminStatus = useQuery({ queryKey: ["garmin-status"], queryFn: api.garminStatus });
  const jobs = useQuery({ queryKey: ["sync-jobs"], queryFn: api.syncJobs, refetchInterval: 2000 });
  const imports = useQuery({ queryKey: ["imports"], queryFn: api.imports });
  const [file, setFile] = useState<File | null>(null);
  const [garminEmail, setGarminEmail] = useState("");
  const [garminPassword, setGarminPassword] = useState("");
  const [garminMFACode, setGarminMFACode] = useState("");
  const [garminOldest, setGarminOldest] = useState(() => localDateString());
  const [garminAllData, setGarminAllData] = useState(false);
  const garminJobs = jobs.data?.jobs ?? [];
  const latestGearJob = garminJobs.find((job) => job.provider === "garmin" && isGearSyncJob(job));
  const latestGarminJob = garminJobs.find((job) => job.provider === "garmin" && !isGearSyncJob(job)) ?? latestGearJob;
  const anyGarminSyncRunning = garminJobs.some((job) => job.provider === "garmin" && job.status === "running");
  const garminSyncRunning = latestGarminJob?.status === "running";
  const gearSyncRunning = latestGearJob?.status === "running";
  const visibleSyncJobs = [latestGarminJob, latestGearJob]
    .filter((job): job is SyncJob => Boolean(job))
    .filter((job, index, list) => list.findIndex((item) => item.id === job.id) === index);

  useEffect(() => {
    if (location.hash !== "#import") {
      return;
    }
    const timeout = window.setTimeout(() => {
      document.getElementById("import")?.scrollIntoView({ block: "start" });
    });
    return () => window.clearTimeout(timeout);
  }, [location.hash]);

  const garminConnect = useMutation({
    mutationFn: api.garminConnect,
    onSuccess: async () => {
      setGarminPassword("");
      setGarminMFACode("");
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["garmin-status"] }),
        queryClient.invalidateQueries({ queryKey: ["sync-jobs"] })
      ]);
    }
  });
  const garminSync = useMutation({
    mutationFn: api.garminSync,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["garmin-status"] }),
        queryClient.invalidateQueries({ queryKey: ["sync-jobs"] }),
        queryClient.invalidateQueries({ queryKey: ["gears"] }),
        queryClient.invalidateQueries({ queryKey: ["gear"] }),
        queryClient.invalidateQueries({ queryKey: ["activities"] }),
        queryClient.invalidateQueries({ queryKey: ["summary"] })
      ]);
    }
  });
  const garminGearSync = useMutation({
    mutationFn: api.garminGearSync,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["sync-jobs"] }),
        queryClient.invalidateQueries({ queryKey: ["gears"] }),
        queryClient.invalidateQueries({ queryKey: ["gear"] }),
        queryClient.invalidateQueries({ queryKey: ["activities"] }),
        queryClient.invalidateQueries({ queryKey: ["summary"] })
      ]);
    }
  });

  useEffect(() => {
    if (!latestGearJob || latestGearJob.status === "running") {
      return;
    }
    void invalidateGearRelatedQueries(queryClient);
  }, [latestGearJob?.id, latestGearJob?.status, queryClient]);

  const upload = useMutation({
    mutationFn: (selected: File) => api.upload(selected),
    onSuccess: async () => {
      setFile(null);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["imports"] }),
        queryClient.invalidateQueries({ queryKey: ["activities"] }),
        queryClient.invalidateQueries({ queryKey: ["activity-types"] }),
        queryClient.invalidateQueries({ queryKey: ["summary"] })
      ]);
    }
  });

  return (
    <Page title="Settings">
      <section className="panel provider-panel">
        <div>
          <div className="panel-heading">Garmin Connect</div>
          <p className="muted">{garminStatus.data?.connected ? `Connected as ${garminStatus.data.connection.displayName}` : "Connect with your Garmin account. Credentials are used only to create Garmin Connect tokens."}</p>
        </div>
        <div className="provider-controls">
          <input
            type="email"
            placeholder="Garmin email"
            value={garminEmail}
            onChange={(event) => setGarminEmail(event.target.value)}
          />
          <input
            type="password"
            placeholder="Garmin password"
            value={garminPassword}
            onChange={(event) => setGarminPassword(event.target.value)}
          />
          <input
            type="text"
            inputMode="numeric"
            placeholder="MFA code"
            value={garminMFACode}
            onChange={(event) => setGarminMFACode(event.target.value)}
          />
          <button className="secondary-button" type="button" disabled={!garminEmail || !garminPassword || garminConnect.isPending} onClick={() => garminConnect.mutate({ email: garminEmail, password: garminPassword, mfaCode: garminMFACode })}>
            <Cloud size={16} />
            Connect
          </button>
          <label className="checkbox-field">
            <input type="checkbox" checked={garminAllData} onChange={(event) => setGarminAllData(event.target.checked)} />
            <span>All data</span>
          </label>
          <label className="compact-field">
            <span>Oldest</span>
            <input type="date" value={garminOldest} max={localDateString()} disabled={garminAllData} onChange={(event) => setGarminOldest(event.target.value)} />
          </label>
          <button className="primary-button" type="button" disabled={!garminStatus.data?.connected || garminSync.isPending || anyGarminSyncRunning} onClick={() => garminSync.mutate({ oldest: garminAllData ? undefined : garminOldest, allData: garminAllData })}>
            <RefreshCw size={16} />
            {garminSyncRunning ? "Syncing" : "Sync"}
          </button>
          <button className="secondary-button" type="button" disabled={!garminStatus.data?.connected || garminGearSync.isPending || anyGarminSyncRunning} onClick={() => garminGearSync.mutate()}>
            <Footprints size={16} />
            {gearSyncRunning ? "Syncing gear" : "Sync gear"}
          </button>
        </div>
      </section>
      {visibleSyncJobs.map((job) => <SyncProgressCard key={job.id} job={job} />)}
      {garminConnect.error && <div className="error">{garminConnect.error instanceof Error ? garminConnect.error.message : "Garmin connection failed"}</div>}
      {garminSync.error && <div className="error">{garminSync.error instanceof Error ? garminSync.error.message : "Garmin sync failed"}</div>}
      {garminGearSync.error && <div className="error">{garminGearSync.error instanceof Error ? garminGearSync.error.message : "Garmin gear sync failed"}</div>}
      <TrainingSheetSettings />
      <ClimbDetectionSettingsSection />
      <DisplaySettingsSection value={themePreference} onChange={onThemePreferenceChange} error={themePreferenceError} />
      <UserManagement />
      <section id="import" className="panel upload-panel">
        <div>
          <div className="panel-heading">Data import</div>
          <p className="muted">Upload a GPX, TCX, or FIT activity file.</p>
        </div>
        <input type="file" accept=".gpx,.tcx,.fit" onChange={(event) => setFile(event.target.files?.[0] ?? null)} />
        <button className="primary-button" type="button" disabled={!file || upload.isPending} onClick={() => file && upload.mutate(file)}>
          <Upload size={16} />
          {upload.isPending ? "Uploading" : "Upload"}
        </button>
      </section>
      {upload.error && <div className="error">{upload.error instanceof Error ? upload.error.message : "Upload failed"}</div>}
      <DiagnosticsPanel
        jobs={jobs.data?.jobs ?? []}
        jobsLoading={jobs.isLoading}
        imports={imports.data?.imports ?? []}
        importsLoading={imports.isLoading}
      />
    </Page>
  );
}

function ClimbDetectionSettingsSection() {
  const queryClient = useQueryClient();
  const config = useQuery({ queryKey: ["config"], queryFn: api.config });
  const configuredSensitivity = config.data?.climbDetection?.sensitivity ?? defaultClimbSensitivity;
  const [draftSensitivity, setDraftSensitivity] = useState(configuredSensitivity);
  const sensitivity = clampClimbSensitivity(draftSensitivity);
  const activePreset = climbSensitivityPresetForValue(sensitivity);
  const activePresetLabel = climbSensitivityPresetLabel(sensitivity);

  useEffect(() => {
    if (config.data?.climbDetection) {
      setDraftSensitivity(config.data.climbDetection.sensitivity);
    }
  }, [config.data?.climbDetection?.sensitivity]);

  const save = useMutation({
    mutationFn: (nextSensitivity: number) => api.updateClimbDetectionSettings({ sensitivity: nextSensitivity }),
    onSuccess: async (updatedConfig) => {
      setDraftSensitivity(updatedConfig.climbDetection.sensitivity);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["config"] }),
        queryClient.invalidateQueries({ queryKey: ["activity"] }),
        queryClient.invalidateQueries({ queryKey: ["activities"] }),
        queryClient.invalidateQueries({ queryKey: ["summary"] })
      ]);
    }
  });

  const updateDraft = (nextSensitivity: number) => {
    save.reset();
    setDraftSensitivity(clampClimbSensitivity(nextSensitivity));
  };

  return (
    <section className="panel climb-settings-panel">
      <div className="climb-settings-header">
        <div>
          <div className="panel-heading">Climb detection</div>
          <p className="muted">Choose the default sensitivity used when detecting climbs across activities.</p>
        </div>
        <span className="climb-sensitivity-preset-label muted">{activePresetLabel}</span>
      </div>
      <div className="climb-settings-content">
        <div className="climb-sensitivity-range">
          <span>Sensitivity</span>
          <strong>{sensitivity}</strong>
        </div>
        <input
          className="climb-sensitivity-slider"
          type="range"
          min={0}
          max={100}
          step={1}
          value={sensitivity}
          aria-label="Default climb sensitivity"
          disabled={!config.data}
          onChange={(event) => updateDraft(Number(event.target.value))}
        />
        <div className="climb-sensitivity-presets">
          {climbSensitivityPresets.map((preset) => (
            <button
              key={preset.id}
              type="button"
              className={`secondary-button small-button ${activePreset === preset.id ? "active" : ""}`}
              aria-pressed={activePreset === preset.id}
              disabled={!config.data}
              onClick={() => updateDraft(preset.value)}
            >
              {preset.label}
            </button>
          ))}
        </div>
        <div className="climb-sensitivity-actions">
          <button
            className="secondary-button small-button"
            type="button"
            disabled={!config.data || sensitivity === defaultClimbSensitivity || save.isPending}
            onClick={() => updateDraft(defaultClimbSensitivity)}
          >
            Restore defaults
          </button>
          <button
            className="primary-button small-button"
            type="button"
            disabled={!config.data || sensitivity === configuredSensitivity || save.isPending}
            onClick={() => save.mutate(sensitivity)}
          >
            {save.isPending ? "Saving..." : "Save permanently"}
          </button>
        </div>
        {config.isLoading && <div className="muted">Loading climb detection settings…</div>}
        {config.error && <div className="error">{config.error instanceof Error ? config.error.message : "Could not load climb detection settings"}</div>}
        {save.error && <div className="error">{save.error instanceof Error ? save.error.message : "Could not save climb detection settings"}</div>}
        {save.isSuccess && <div className="muted">Climb detection settings saved.</div>}
      </div>
    </section>
  );
}

function TrainingSheetSettings() {
  const queryClient = useQueryClient();
  const config = useQuery({ queryKey: ["training-sheet-config"], queryFn: api.trainingSheetConfig });
  const google = useQuery({ queryKey: ["google-sheets-status"], queryFn: api.googleSheetsStatus });
  const [enabled, setEnabled] = useState(false);
  const [sheetURL, setSheetURL] = useState("");
  const [checkEveryHours, setCheckEveryHours] = useState(24);
  const [planYear, setPlanYear] = useState(new Date().getFullYear());
  const jobs = useQuery({ queryKey: ["sync-jobs"], queryFn: api.syncJobs, refetchInterval: 2000 });
  useEffect(() => {
    if (!config.data) return;
    setEnabled(config.data.enabled);
    setSheetURL(config.data.sheetURL);
    setCheckEveryHours(config.data.checkEveryHours);
    setPlanYear(config.data.planYear ?? new Date().getFullYear());
  }, [config.data]);
  const save = useMutation({
    mutationFn: () => api.updateTrainingSheetConfig({ enabled, sheetURL: sheetURL.trim(), checkEveryHours, planYear }),
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: ["training-sheet-config"] }); }
  });
  const sync = useMutation({ mutationFn: api.trainingSheetSync, onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: ["sync-jobs"] }); } });
  const trainingSheetJob = (jobs.data?.jobs ?? []).find((job) => job.provider === "training_sheet" && job.kind !== "writeback");
  const trainingSheetSyncRunning = sync.isPending || trainingSheetJob?.status === "running";
  const syncStage = typeof trainingSheetJob?.payload?.stage === "string" ? trainingSheetJob.payload.stage : "";
  useEffect(() => {
    if (!trainingSheetJob || trainingSheetJob.status === "running") return;
    void Promise.all([
      queryClient.invalidateQueries({ queryKey: ["training-sheet-config"] }),
      queryClient.invalidateQueries({ queryKey: ["planned-activities"] }),
      queryClient.invalidateQueries({ queryKey: ["activities"] }),
      queryClient.invalidateQueries({ queryKey: ["summary"] })
    ]);
  }, [trainingSheetJob?.id, trainingSheetJob?.status, queryClient]);
  const canSync = enabled && sheetURL.trim().length > 0 && google.data?.connected === true;
  return (
    <details className="panel settings-advanced-details">
      <summary><span className="panel-heading">Training plan import (Experimental)</span></summary>
      <div className="settings-advanced-content">
        <p className="muted">Opt-in Google Sheets integration for a structured coach training workbook. Leave disabled if you do not use this workflow.</p>
        <label className="checkbox-field"><input type="checkbox" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} /> Enable training plan sync</label>
        <label className="field"><span>Google Sheet URL</span><input type="url" value={sheetURL} onChange={(event) => setSheetURL(event.target.value)} placeholder="https://docs.google.com/spreadsheets/d/..." /></label>
        <label className="field"><span>Plan year</span><input type="number" min={1900} max={9999} value={planYear} onChange={(event) => setPlanYear(Number(event.target.value))} /></label>
        <label className="field"><span>Check every (hours)</span><input type="number" min={1} max={720} value={checkEveryHours} onChange={(event) => setCheckEveryHours(Number(event.target.value))} /></label>
        <p className="muted">Google account: {google.data?.connected ? (google.data.writeReady ? "connected with write access" : "reconnect required for write access") : google.data?.configured ? "not connected" : "OAuth not configured on the server"}</p>
        <div className="training-sheet-actions">
          <a className="secondary-button small-button" href="/api/providers/google/connect">Connect Google account</a>
          <button className="primary-button small-button" type="button" disabled={save.isPending} onClick={() => save.mutate()}>{save.isPending ? "Saving..." : "Save settings"}</button>
          <button className="secondary-button small-button" type="button" disabled={!canSync || trainingSheetSyncRunning} onClick={() => sync.mutate()}>{trainingSheetSyncRunning ? "Syncing..." : "Sync now"}</button>
          <SyncJobCancelButton job={trainingSheetJob} compact />
        </div>
        {trainingSheetSyncRunning && <p className="muted">Training plan sync is running{syncStage ? `: ${syncStage}` : "..."}</p>}
        {trainingSheetJob?.status === "completed" && <p className="muted">Training plan sync completed.</p>}
        {trainingSheetJob?.status === "canceled" && <p className="muted">Training plan sync canceled.</p>}
        {trainingSheetJob?.status === "failed" && <div className="error">Training plan sync failed: {trainingSheetJob.error || "See the server logs for details."}</div>}
        {config.data?.lastSyncedAt && <p className="muted">Last synced: {config.data.lastSyncedAt}</p>}
        {save.error && <div className="error">{save.error instanceof Error ? save.error.message : "Could not save training sheet settings"}</div>}
        {sync.error && <div className="error">{sync.error instanceof Error ? sync.error.message : "Training sheet sync failed"}</div>}
      </div>
    </details>
  );
}

function DisplaySettingsSection({
  value,
  onChange,
  error
}: {
  value: ThemePreference;
  onChange: (preference: ThemePreference) => void;
  error?: Error | null;
}) {
  return (
    <section className="panel display-panel">
      <div>
        <div className="panel-heading">Display</div>
        <p className="muted">Choose how Runnarr follows your browser color settings.</p>
      </div>
      <ThemePreferenceControl value={value} onChange={onChange} />
      {error && <div className="error">{error.message || "Could not save display preferences"}</div>}
    </section>
  );
}

function UserManagement() {
  const session = useQuery({ queryKey: ["session"], queryFn: api.session });
  const queryClient = useQueryClient();
  const users = useQuery({
    queryKey: ["users"],
    queryFn: api.users,
    enabled: session.data?.actor?.role === "admin"
  });
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<"admin" | "user">("user");
  const create = useMutation({
    mutationFn: api.createUser,
    onSuccess: async () => {
      setUsername("");
      setDisplayName("");
      setPassword("");
      await queryClient.invalidateQueries({ queryKey: ["users"] });
    }
  });
  const update = useMutation({
    mutationFn: ({ id, disabled }: { id: string; disabled: boolean }) => api.updateUser(id, { disabled }),
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: ["users"] }); }
  });
  const support = useMutation({
    mutationFn: api.startSupport,
    onSuccess: () => window.location.reload()
  });

  if (session.data?.actor?.role !== "admin") {
    return null;
  }

  return (
    <section className="panel user-management-panel">
      <div className="panel-heading">Accounts</div>
      <p className="muted">Create and disable local accounts. Support view is read-only and never changes another user’s data.</p>
      <form className="user-create-form" onSubmit={(event) => {
        event.preventDefault();
        create.mutate({ username: username.trim(), displayName: displayName.trim(), password, role });
      }}>
        <input type="text" placeholder="Username" autoComplete="off" value={username} onChange={(event) => setUsername(event.target.value)} />
        <input type="text" placeholder="Display name" autoComplete="off" value={displayName} onChange={(event) => setDisplayName(event.target.value)} />
        <input type="password" placeholder="Temporary password" autoComplete="new-password" value={password} onChange={(event) => setPassword(event.target.value)} />
        <select value={role} onChange={(event) => setRole(event.target.value as "admin" | "user")}>
          <option value="user">User</option>
          <option value="admin">Administrator</option>
        </select>
        <button className="primary-button" type="submit" disabled={create.isPending || username.trim().length === 0 || password.length < 8}>Create</button>
      </form>
      {create.error && <div className="error">{create.error instanceof Error ? create.error.message : "Could not create account"}</div>}
      {users.isLoading && <LoadingRow />}
      {!users.isLoading && (users.data?.users ?? []).length > 0 && (
        <div className="table-wrap">
          <table className="data-table">
            <thead><tr><th>User</th><th>Role</th><th>Status</th><th>Actions</th></tr></thead>
            <tbody>
              {(users.data?.users ?? []).map((user) => (
                <tr key={user.id}>
                  <td><strong>{user.displayName || user.username}</strong><span className="muted row-subtext">{user.username}</span></td>
                  <td>{user.role}</td>
                  <td>{user.disabled ? "Disabled" : "Active"}</td>
                  <td className="table-actions">
                    {user.id !== session.data?.actor?.id && <button className="secondary-button small-button" type="button" disabled={support.isPending || user.disabled} onClick={() => support.mutate(user.id)}>Support view</button>}
                    {user.id !== session.data?.actor?.id && <button className="secondary-button small-button" type="button" disabled={update.isPending} onClick={() => update.mutate({ id: user.id, disabled: !user.disabled })}>{user.disabled ? "Enable" : "Disable"}</button>}
                    <button className="secondary-button small-button" type="button" onClick={() => {
                      const nextPassword = window.prompt(`New password for ${user.username} (at least 8 characters)`);
                      if (nextPassword && nextPassword.length >= 8) {
                        void api.resetUserPassword(user.id, nextPassword).then(() => queryClient.invalidateQueries({ queryKey: ["users"] }));
                      }
                    }}>Reset password</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {support.error && <div className="error">{support.error instanceof Error ? support.error.message : "Could not enter support view"}</div>}
    </section>
  );
}

function ThemePreferenceControl({
  value,
  onChange
}: {
  value: ThemePreference;
  onChange: (preference: ThemePreference) => void;
}) {
  const options: Array<{ value: ThemePreference; label: string; icon: JSX.Element }> = [
    { value: "system", label: "System", icon: <Monitor size={15} /> },
    { value: "light", label: "Light", icon: <Sun size={15} /> },
    { value: "dark", label: "Dark", icon: <Moon size={15} /> }
  ];

  return (
    <div className="segmented-control theme-control" role="group" aria-label="Theme preference">
      {options.map((option) => (
        <button
          key={option.value}
          className={value === option.value ? "active" : ""}
          type="button"
          aria-pressed={value === option.value}
          onClick={() => onChange(option.value)}
        >
          {option.icon}
          {option.label}
        </button>
      ))}
    </div>
  );
}

function DiagnosticsPanel({
  jobs,
  jobsLoading,
  imports,
  importsLoading
}: {
  jobs: SyncJob[];
  jobsLoading: boolean;
  imports: ImportFile[];
  importsLoading: boolean;
}) {
  return (
    <section className="panel diagnostics-panel">
      <details className="diagnostics-details">
        <summary>
          <span>
            <span className="panel-heading">Diagnostics</span>
            <span className="muted">Sync jobs and manual import history</span>
          </span>
        </summary>
        <div className="diagnostics-content">
          <section className="diagnostics-section">
            <div className="panel-heading">Sync jobs</div>
            {jobsLoading && <LoadingRow />}
            {!jobsLoading && jobs.length === 0 && <EmptyState title="No sync jobs yet" />}
            {!jobsLoading && jobs.length > 0 && <SyncJobsTable jobs={jobs} />}
          </section>
          <section className="diagnostics-section">
            <div className="panel-heading">Recent imports</div>
            {importsLoading && <LoadingRow />}
            {!importsLoading && imports.length === 0 && <EmptyState title="No imports yet" />}
            {!importsLoading && imports.length > 0 && <RecentImportsTable imports={imports} />}
          </section>
        </div>
      </details>
    </section>
  );
}

function SyncJobsTable({ jobs }: { jobs: SyncJob[] }) {
  return (
    <div className="table-wrap">
      <table className="data-table">
        <thead>
          <tr>
            <th>Provider</th>
            <th>Kind</th>
            <th>Status</th>
            <th>Progress</th>
            <th>Details</th>
            <th>Created</th>
            <th aria-label="Actions" />
          </tr>
        </thead>
        <tbody>
          {jobs.map((job) => (
            <tr key={job.id}>
              <td>{job.provider}</td>
              <td>{job.kind}</td>
              <td><span className={`status ${job.status}`}>{job.status}</span>{job.error && <span className="row-error">{job.error}</span>}</td>
              <td><SyncProgressBar job={job} /></td>
              <td>{formatSyncJobDetails(job)}</td>
              <td>{formatDate(job.createdAt)}</td>
              <td><SyncJobCancelButton job={job} compact /></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

type CancellableSyncJob = Pick<SyncJob, "id" | "status"> & {
  cancelRequestedAt?: string;
};

function SyncJobCancelButton({ job, compact = false }: { job?: CancellableSyncJob; compact?: boolean }) {
  const queryClient = useQueryClient();
  const cancelSync = useMutation({
    mutationFn: api.cancelSyncJob,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["sync-jobs"] }),
        queryClient.invalidateQueries({ queryKey: ["planned-match-candidates"] })
      ]);
    }
  });
  if (!job || job.status !== "running") {
    return null;
  }
  const cancelling = Boolean(job.cancelRequestedAt) || cancelSync.isPending;
  return (
    <>
      <button
        className={`secondary-button${compact ? " small-button" : ""}`}
        type="button"
        disabled={cancelling}
        onClick={() => cancelSync.mutate(job.id)}
      >
        <Square size={compact ? 14 : 16} />
        {cancelling ? "Cancelling..." : "Cancel"}
      </button>
      {cancelSync.error && <div className="error">{cancelSync.error instanceof Error ? cancelSync.error.message : "Failed to cancel sync job"}</div>}
    </>
  );
}

function RecentImportsTable({ imports }: { imports: ImportFile[] }) {
  return (
    <div className="table-wrap">
      <table className="data-table">
        <thead>
          <tr>
            <th>File</th>
            <th>Parser</th>
            <th>Status</th>
            <th>Imported</th>
          </tr>
        </thead>
        <tbody>
          {imports.map((item) => (
            <tr key={item.id}>
              <td>{item.filename}</td>
              <td>{item.parser.toUpperCase()}</td>
              <td><span className={`status ${item.status}`}>{item.status}</span>{item.error && <span className="row-error">{item.error}</span>}</td>
              <td>{formatDate(item.createdAt)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function SyncProgressCard({ job }: { job?: SyncJob }) {
  if (!job || job.provider !== "garmin") {
    return null;
  }
  const healthSync = isHealthSyncJob(job);
  const gearSync = isGearSyncJob(job);
  const payload = job.payload ?? {};
  const imported = payloadNumber(payload, "imported");
  const saved = payloadNumber(payload, "saved");
  const processed = payloadNumber(payload, "processed");
  const activities = payloadNumber(payload, "activities");
  const days = payloadNumber(payload, "days");
  const gear = payloadNumber(payload, "gear");
  const total = healthSync ? days : gearSync ? gear : activities;
  const failed = payloadNumber(payload, "failed");
  const skippedExcluded = payloadNumber(payload, "skippedExcluded");
  const assignments = payloadNumber(payload, "assignments");
  const localAssignments = payloadNumber(payload, "localAssignments");
  const stage = payloadText(payload, "stage") || job.status;
  const listing = isSyncListingStage(stage);
  const fetchedPages = payloadNumber(payload, "fetchedPages");
  const oldest = payloadText(payload, "oldest");
  const allData = payload.allData === true;
  const from = payloadText(payload, "from");
  const to = payloadText(payload, "to");
  const currentDate = payloadText(payload, "currentDate");
  const currentActivityName = payloadText(payload, "currentActivityName");
  const currentGearName = payloadText(payload, "currentGearName");
  const warnings = payloadList(payload, "warnings");
  const firstErrors = payloadList(payload, "firstErrors");
  const foundLabel = activities === 1 ? "activity" : "activities";
  const dayLabel = days === 1 ? "day" : "days";
  const gearLabel = gear === 1 ? "item" : "items";
  const detailText = syncProgressDetailText(job, stage, currentActivityName, currentGearName, currentDate, oldest, allData, from, to, total);

  return (
    <section className="panel sync-progress-panel">
      <div className="filter-header">
        <div className="panel-heading">{gearSync ? "Garmin gear sync progress" : healthSync ? "Garmin health sync progress" : "Garmin sync progress"}</div>
        <span className={`status ${job.cancelRequestedAt ? "cancelling" : job.status}`}>{job.cancelRequestedAt && job.status === "running" ? "cancelling" : job.status}</span>
        <SyncJobCancelButton job={job} />
      </div>
      <SyncProgressBar job={job} />
      <div className="sync-progress-grid">
        {gearSync ? (
          <>
            <SyncStat label="Completed" value={`${processed.toLocaleString()} / ${gear.toLocaleString()} ${gearLabel}`} />
            <SyncStat label="Saved" value={saved.toLocaleString()} />
            <SyncStat label="Garmin assignments" value={assignments.toLocaleString()} />
            <SyncStat label="Local assignments" value={localAssignments.toLocaleString()} />
          </>
        ) : healthSync ? (
          <>
            <SyncStat label="Completed" value={`${processed.toLocaleString()} / ${days.toLocaleString()} ${dayLabel}`} />
            <SyncStat label="Saved" value={saved.toLocaleString()} />
            <SyncStat label="Failed" value={failed.toLocaleString()} />
            <SyncStat label="Range" value={from && to ? `${from} to ${to}` : "Recent"} />
          </>
        ) : listing ? (
          <>
            <SyncStat label="Found" value={`${activities.toLocaleString()} ${foundLabel}`} />
            <SyncStat label="Pages" value={fetchedPages.toLocaleString()} />
            <SyncStat label="Imported" value={imported.toLocaleString()} />
            <SyncStat label="Failed" value={failed.toLocaleString()} />
          </>
        ) : (
          <>
            <SyncStat label="Completed" value={`${processed.toLocaleString()} / ${activities.toLocaleString()}`} />
            <SyncStat label="Imported" value={imported.toLocaleString()} />
            <SyncStat label="Ignored" value={skippedExcluded.toLocaleString()} />
            <SyncStat label="Failed" value={failed.toLocaleString()} />
          </>
        )}
      </div>
      <div className="sync-progress-details">
        <span>{stage}</span>
        <span>{detailText}</span>
        <span>{formatSyncJobDetails(job)}</span>
      </div>
      {(warnings.length > 0 || firstErrors.length > 0) && (
        <div className="sync-progress-messages">
          {warnings.map((message) => <span key={`warning-${message}`}>{message}</span>)}
          {firstErrors.map((message) => <span key={`error-${message}`}>{message}</span>)}
        </div>
      )}
    </section>
  );
}

function SyncStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="sync-stat">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function SyncProgressBar({ job }: { job: SyncJob }) {
  const payload = job.payload ?? {};
  const processed = payloadNumber(payload, "processed");
  const total = isGearSyncJob(job) ? payloadNumber(payload, "gear") : isHealthSyncJob(job) ? payloadNumber(payload, "days") : payloadNumber(payload, "activities");
  const stage = payloadText(payload, "stage");
  const listing = job.status === "running" && isSyncListingStage(stage);
  const hasKnownTotal = total > 0 && !listing;
  const percent = hasKnownTotal ? Math.min(100, Math.round((processed / total) * 100)) : 0;
  return (
    <div className="progress-cell">
      <div className={`progress-bar${listing ? " indeterminate" : ""}`} aria-label={listing ? "Listing Garmin data" : `Sync progress ${percent}%`}>
        <span style={listing ? undefined : { width: `${percent}%` }} />
      </div>
      <span>{listing ? "Listing" : hasKnownTotal ? `${percent}%` : job.status}</span>
    </div>
  );
}

function formatSyncJobDetails(job: SyncJob) {
  const payload = job.payload ?? {};
  const imported = payloadNumber(payload, "imported");
  const saved = payloadNumber(payload, "saved");
  const processed = payloadNumber(payload, "processed");
  const failed = payloadNumber(payload, "failed");
  const skippedExcluded = payloadNumber(payload, "skippedExcluded");
  const activities = payloadNumber(payload, "activities");
  const days = payloadNumber(payload, "days");
  const gear = payloadNumber(payload, "gear");
  const assignments = payloadNumber(payload, "assignments");
  const localAssignments = payloadNumber(payload, "localAssignments");
  const fetchedPages = payloadNumber(payload, "fetchedPages");
  const stage = payloadText(payload, "stage");
  const parts = [];
  if (isGearSyncJob(job)) {
    if (gear > 0) {
      parts.push(`${processed}/${gear} gear`);
    }
    if (saved > 0) {
      parts.push(`${saved} saved`);
    }
    if (localAssignments > 0) {
      parts.push(`${localAssignments} assigned`);
    } else if (assignments > 0) {
      parts.push(`${assignments} provider assignments`);
    }
    return parts.length > 0 ? parts.join(" · ") : "-";
  }
  if (isHealthSyncJob(job)) {
    if (days > 0) {
      parts.push(`${processed}/${days} days`);
    }
    if (saved > 0) {
      parts.push(`${saved} saved`);
    }
    if (failed > 0) {
      parts.push(`${failed} failed`);
    }
    return parts.length > 0 ? parts.join(" · ") : "-";
  }
  if (isSyncListingStage(stage)) {
    if (activities > 0) {
      parts.push(`${activities} found`);
    }
    if (fetchedPages > 0) {
      parts.push(`${fetchedPages} pages`);
    }
    return parts.length > 0 ? parts.join(" · ") : "listing";
  }
  if (activities > 0) {
    parts.push(`${processed}/${activities} processed`);
  }
  if (imported > 0 || failed > 0) {
    parts.push(`${imported}/${activities} imported`);
  }
  if (failed > 0) {
    parts.push(`${failed} failed`);
  }
  if (skippedExcluded > 0) {
    parts.push(`${skippedExcluded} ignored`);
  }
  return parts.length > 0 ? parts.join(" · ") : "-";
}

function isSyncListingStage(stage: string) {
  return stage.toLowerCase().includes("listing");
}

function isHealthSyncJob(job: SyncJob) {
  return job.kind.startsWith("health") || payloadText(job.payload ?? {}, "kind") === "health";
}

function isGearSyncJob(job: SyncJob) {
  return job.kind.startsWith("gear") || payloadText(job.payload ?? {}, "kind") === "gear";
}

function syncProgressDetailText(job: SyncJob, stage: string, currentActivityName: string, currentGearName: string, currentDate: string, oldest: string, allData: boolean, from: string, to: string, total: number) {
  if (isGearSyncJob(job)) {
    if (currentGearName) {
      return currentGearName;
    }
    if (job.status === "completed") {
      return total > 0 ? "Gear sync finished" : "No gear found";
    }
    if (job.status === "canceled") {
      return "Gear sync canceled";
    }
    return "Waiting for first gear item";
  }
  if (isHealthSyncJob(job)) {
    if (currentDate) {
      return currentDate;
    }
    if (job.status === "completed") {
      return total > 0 ? "Health sync finished" : "No days found";
    }
    if (job.status === "canceled") {
      return "Health sync canceled";
    }
    return from && to ? `${from} to ${to}` : "Waiting for first day";
  }
  if (isSyncListingStage(stage)) {
    if (allData) {
      return "Searching all available data";
    }
    return oldest ? `Searching from ${oldest}` : "Searching Garmin Connect";
  }
  if (currentActivityName) {
    return currentActivityName;
  }
  if (job.status === "completed") {
    return total > 0 ? "Sync finished" : "No activities found";
  }
  if (job.status === "canceled") {
    return "Sync canceled";
  }
  return "Waiting for first activity";
}

function payloadNumber(payload: Record<string, unknown>, key: string) {
  const value = payload[key];
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function payloadText(payload: Record<string, unknown>, key: string) {
  const value = payload[key];
  return typeof value === "string" ? value : "";
}

function payloadList(payload: Record<string, unknown>, key: string) {
  const value = payload[key];
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}

function healthRangePresets() {
  return [
    { label: "7D", days: 7 },
    { label: "30D", days: 30 },
    { label: "90D", days: 90 }
  ];
}

function healthRangeForLastDays(days: number): HealthDateRange {
  const to = new Date();
  const from = new Date(to);
  from.setDate(to.getDate() - Math.max(days - 1, 0));
  return { from: localDateString(from), to: localDateString(to) };
}

function healthRangesMatch(left: HealthDateRange, right: HealthDateRange) {
  return left.from === right.from && left.to === right.to;
}

function healthRangeDayCount(range: HealthDateRange) {
  const from = localDateFromString(range.from);
  const to = localDateFromString(range.to);
  if (!from || !to || from > to) {
    return 0;
  }
  const millisecondsPerDay = 24 * 60 * 60 * 1000;
  return Math.floor((to.getTime() - from.getTime()) / millisecondsPerDay) + 1;
}

function localDateFromString(value: string) {
  const [year, month, day] = value.split("-").map(Number);
  if (!year || !month || !day) {
    return undefined;
  }
  return new Date(year, month - 1, day);
}

function parseCalendarMonth(value: string | null): CalendarMonth {
  const now = new Date();
  if (!value) {
    return { year: now.getFullYear(), month: now.getMonth() + 1 };
  }
  const [year, month] = value.split("-").map(Number);
  if (!year || !month || month < 1 || month > 12) {
    return { year: now.getFullYear(), month: now.getMonth() + 1 };
  }
  return { year, month };
}

function formatCalendarMonth(month: CalendarMonth) {
  return `${month.year}-${String(month.month).padStart(2, "0")}`;
}

function formatCalendarMonthLabel(month: CalendarMonth) {
  return new Date(month.year, month.month - 1, 1).toLocaleDateString(undefined, { month: "long", year: "numeric" });
}

function formatCalendarDate(year: number, month: number, day: number) {
  return `${year}-${String(month).padStart(2, "0")}-${String(day).padStart(2, "0")}`;
}

function calendarMonthRange(month: CalendarMonth) {
  return {
    start: formatCalendarDate(month.year, month.month, 1),
    end: formatCalendarDate(month.year, month.month, new Date(month.year, month.month, 0).getDate())
  };
}

function calendarMonthOffset(month: CalendarMonth, offset: number): CalendarMonth {
  const date = new Date(month.year, month.month - 1, 1);
  date.setMonth(date.getMonth() + offset);
  return {
    year: date.getFullYear(),
    month: date.getMonth() + 1
  };
}

function localDateString(value = new Date()) {
  const year = value.getFullYear();
  const month = String(value.getMonth() + 1).padStart(2, "0");
  const day = String(value.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function latestHealthMetric(metrics: DailyHealthMetric[]) {
  return [...metrics].reverse().find(hasAnyHealthMetric);
}

function hasAnyHealthMetric(metric: DailyHealthMetric) {
  return [
    metric.steps,
    metric.totalCaloriesKcal,
    metric.activeCaloriesKcal,
    metric.restingHeartRateBpm,
    metric.sleepDurationS,
    metric.stressAvg,
    metric.bodyBatteryGained,
    metric.bodyBatteryDrained,
    metric.bodyBatteryMax,
    metric.hrvAvgMs,
    metric.weightKg
  ].some(isFiniteNumber);
}

function healthChartData(metrics: DailyHealthMetric[]): HealthChartPoint[] {
  return metrics.map((metric) => {
    const totalCalories = finiteValue(metric.totalCaloriesKcal);
    const activeCalories = finiteValue(metric.activeCaloriesKcal);
    const remainingCalories = isFiniteNumber(totalCalories) ? Math.max(0, totalCalories - (activeCalories ?? 0)) : undefined;
    return {
      date: metric.date,
      label: healthChartLabel(metric.date),
      steps: finiteValue(metric.steps),
      totalCalories,
      activeCalories,
      remainingCalories,
      sleepHours: isFiniteNumber(metric.sleepDurationS) ? metric.sleepDurationS / 3600 : undefined,
      restingHeartRate: finiteValue(metric.restingHeartRateBpm),
      stress: finiteValue(metric.stressAvg),
      bodyBatteryGained: finiteValue(metric.bodyBatteryGained),
      bodyBatteryDrained: finiteValue(metric.bodyBatteryDrained),
      bodyBatteryDrainedLoss: isFiniteNumber(metric.bodyBatteryDrained) ? -metric.bodyBatteryDrained : undefined,
      bodyBatteryHighest: finiteValue(metric.bodyBatteryMax),
      hrv: finiteValue(metric.hrvAvgMs),
      weight: finiteValue(metric.weightKg)
    };
  });
}

function healthMetricCards(metric?: DailyHealthMetric) {
  if (!metric) {
    return [];
  }
  return [
    { label: "Steps", value: formatHealthInteger(metric.steps), icon: <Footprints size={18} /> },
    { label: "Calories", value: formatHealthCalories(metric.totalCaloriesKcal ?? metric.activeCaloriesKcal), icon: <Flame size={18} /> },
    { label: "Sleep", value: formatHealthDuration(metric.sleepDurationS), icon: <Moon size={18} /> },
    { label: "Resting HR", value: formatHealthBPM(metric.restingHeartRateBpm), icon: <HeartPulse size={18} /> },
    { label: "Body battery", value: formatBodyBatteryGainDrain(metric), icon: <BatteryCharging size={18} /> },
    { label: "HRV", value: formatHealthMS(metric.hrvAvgMs), icon: <ActivityIcon size={18} /> },
    { label: "Weight", value: formatHealthWeight(metric.weightKg), icon: <Scale size={18} /> }
  ].filter((item) => item.value !== "");
}

function healthDetailItems(metric: DailyHealthMetric) {
  return [
    { label: "Steps", value: formatHealthInteger(metric.steps) },
    { label: "Total calories", value: formatHealthCalories(metric.totalCaloriesKcal) },
    { label: "Active calories", value: formatHealthCalories(metric.activeCaloriesKcal) },
    { label: "Resting heart rate", value: formatHealthBPM(metric.restingHeartRateBpm) },
    { label: "Average heart rate", value: formatHealthBPM(metric.avgHeartRateBpm) },
    { label: "Maximum heart rate", value: formatHealthBPM(metric.maxHeartRateBpm) },
    { label: "Sleep", value: formatHealthDuration(metric.sleepDurationS) },
    { label: "Deep sleep", value: formatHealthDuration(metric.deepSleepS) },
    { label: "Light sleep", value: formatHealthDuration(metric.lightSleepS) },
    { label: "REM sleep", value: formatHealthDuration(metric.remSleepS) },
    { label: "Awake", value: formatHealthDuration(metric.awakeSleepS) },
    { label: "Sleep score", value: formatHealthRounded(metric.sleepScore) },
    { label: "Average stress", value: formatHealthRounded(metric.stressAvg) },
    { label: "Maximum stress", value: formatHealthRounded(metric.stressMax) },
    { label: "Body battery gained", value: formatHealthRounded(metric.bodyBatteryGained) },
    { label: "Body battery drained", value: formatHealthRounded(metric.bodyBatteryDrained) },
    { label: "Body battery highest", value: formatHealthRounded(metric.bodyBatteryMax) },
    { label: "HRV average", value: formatHealthMS(metric.hrvAvgMs) },
    { label: "HRV status", value: metric.hrvStatus ?? "" },
    { label: "Weight", value: formatHealthWeight(metric.weightKg) },
    { label: "Body fat", value: formatHealthPercent(metric.bodyFatPct) }
  ].filter((item) => item.value !== "");
}

function healthChartLabel(date: string) {
  const [year, month, day] = date.split("-").map(Number);
  if (!year || !month || !day) {
    return date;
  }
  return new Date(year, month - 1, day).toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

function formatHealthDate(date: string) {
  const [year, month, day] = date.split("-").map(Number);
  if (!year || !month || !day) {
    return date;
  }
  return new Date(year, month - 1, day).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
}

function finiteValue(value?: number) {
  return isFiniteNumber(value) ? value : undefined;
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function formatHealthInteger(value?: number) {
  return isFiniteNumber(value) ? Math.round(value).toLocaleString() : "";
}

function formatHealthRounded(value?: number) {
  return isFiniteNumber(value) ? Math.round(value).toLocaleString() : "";
}

function formatBodyBatteryGainDrain(metric: DailyHealthMetric) {
  const gained = formatHealthRounded(metric.bodyBatteryGained);
  const drained = formatHealthRounded(metric.bodyBatteryDrained);
  if (gained && drained) {
    return `+${gained} / -${drained}`;
  }
  if (gained) {
    return `+${gained}`;
  }
  if (drained) {
    return `-${drained}`;
  }
  return formatHealthRounded(metric.bodyBatteryMax);
}

function formatHealthCalories(value?: number) {
  return isFiniteNumber(value) ? `${Math.round(value).toLocaleString()} kcal` : "";
}

function formatHealthBPM(value?: number) {
  return isFiniteNumber(value) ? `${Math.round(value)} bpm` : "";
}

function formatHealthMS(value?: number) {
  return isFiniteNumber(value) ? `${Math.round(value)} ms` : "";
}

function formatHealthWeight(value?: number) {
  return isFiniteNumber(value) ? `${value.toFixed(1)} kg` : "";
}

function formatHealthPercent(value?: number) {
  return isFiniteNumber(value) ? `${value.toFixed(1).replace(/\.0$/, "")}%` : "";
}

function formatHealthDuration(totalSeconds?: number) {
  if (!isFiniteNumber(totalSeconds) || totalSeconds <= 0) {
    return "";
  }
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.round((totalSeconds % 3600) / 60);
  if (hours <= 0) {
    return `${minutes}m`;
  }
  return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`;
}

function Page({ title, eyebrow, actions, children }: { title: string; eyebrow?: string; actions?: ReactNode; children: ReactNode }) {
  return (
    <div className="page">
      <header className="page-header">
        <div>
          {eyebrow && <div className="eyebrow">{eyebrow}</div>}
          <h1>{title}</h1>
        </div>
        {actions && <div className="actions">{actions}</div>}
      </header>
      {children}
    </div>
  );
}

function Metric({ label, value, icon }: { label: string; value: string; icon?: JSX.Element }) {
  return (
    <div className="metric">
      {icon && <span className="metric-icon" aria-hidden>{icon}</span>}
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function ActivityCombinedChart({ data, onHighlight }: { data: ActivityChartPoint[]; onHighlight: (point?: ActivityChartPoint) => void }) {
  const availableSeries = activityChartSeries.filter((series) => data.some((item) => typeof item[series.key] === "number"));
  const defaultVisible = availableSeries.filter((series) => series.defaultVisible).map((series) => series.key);
  const initialVisible = defaultVisible.length > 0 ? defaultVisible : availableSeries.slice(0, 1).map((series) => series.key);
  const [visibleSeries, setVisibleSeries] = useState<ActivityChartSeriesKey[]>(initialVisible);
  const activeSeries = availableSeries.filter((series) => visibleSeries.includes(series.key));
  const toggleSeries = (key: ActivityChartSeriesKey) => {
    setVisibleSeries((current) => {
      if (current.includes(key)) {
        return current.length === 1 ? current : current.filter((item) => item !== key);
      }
      return [...current, key];
    });
  };

  return (
    <section className="panel">
      <div className="chart-header">
        <div className="panel-heading">Activity graph</div>
        {availableSeries.length > 0 && (
          <div className="chart-toggle-list">
            {availableSeries.map((series) => {
              const active = visibleSeries.includes(series.key);
              return (
                <button
                  key={series.key}
                  className={`chart-toggle ${active ? "active" : ""}`}
                  type="button"
                  style={active ? { borderColor: series.color, backgroundColor: series.color } : { borderColor: series.color, color: series.color }}
                  aria-pressed={active}
                  disabled={active && visibleSeries.length === 1}
                  onClick={() => toggleSeries(series.key)}
                >
                  {series.label}
                </button>
              );
            })}
          </div>
        )}
      </div>
      {activeSeries.length > 0 ? (
        <div className="chart-area">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart
              data={data}
              onMouseMove={(state) => onHighlight(chartPointFromMouseState(state, data))}
              onMouseLeave={() => onHighlight(undefined)}
            >
              <CartesianGrid strokeDasharray="3 3" vertical={false} />
              <XAxis dataKey="label" minTickGap={26} />
              {activeSeries.map((series, index) => (
                <YAxis
                  key={series.key}
                  yAxisId={series.key}
                  orientation={index === 0 ? "left" : "right"}
                  width={series.key === "paceSPKM" ? 58 : 46}
                  domain={["auto", "auto"]}
                  reversed={series.key === "paceSPKM"}
                  tickFormatter={(value) => formatChartTick(Number(value), series)}
                />
              ))}
              <Tooltip
                contentStyle={chartTooltipContentStyle}
                labelStyle={chartTooltipLabelStyle}
                formatter={(value, name, item) => formatChartTooltip(value, String(name), activeSeries, item)}
              />
              {activeSeries.map((series) => (
                <Line
                  key={series.key}
                  type={series.key === "elevationM" ? "basis" : "monotone"}
                  dataKey={series.key}
                  name={series.label}
                  yAxisId={series.key}
                  stroke={series.color}
                  dot={false}
                  strokeWidth={2}
                  connectNulls={series.key !== "paceSPKM"}
                />
              ))}
            </LineChart>
          </ResponsiveContainer>
        </div>
      ) : (
        <EmptyState title="No samples for this chart" />
      )}
    </section>
  );
}

function ActivityMap({
  points,
  paceSegments = [],
  tileURL,
  highlightedPoint,
  climbSegments = [],
  selectedClimbIndex,
  onSelectClimb,
  mediaMarkers = [],
  selectedMediaId,
  onSelectMedia,
  routeColorSource,
  onRouteColorSourceChange,
  showRouteColorSelector
}: {
  points: RoutePoint[];
  paceSegments?: PaceRouteSegment[];
  tileURL?: string;
  highlightedPoint?: RoutePoint;
  climbSegments?: ClimbMapSegment[];
  selectedClimbIndex?: number;
  onSelectClimb?: (climb: ActivityClimb) => void;
  mediaMarkers?: ActivityMedia[];
  selectedMediaId?: string;
  onSelectMedia?: (mediaId: string) => void;
  routeColorSource?: RouteColorSource;
  onRouteColorSourceChange?: (next: RouteColorSource) => void;
  showRouteColorSelector?: boolean;
}) {
  const mediaPoints = mediaMarkers.map(mediaRoutePoint).filter((point): point is RoutePoint => Boolean(point));
  const mapPoints = [...points, ...mediaPoints];
  const center = points[0] ?? mediaPoints[0] ?? [53.3498, -6.2603];
  const start = points[0];
  const end = points.length > 1 ? points[points.length - 1] : undefined;
  return (
    <div className="map-frame">
      <MapContainer center={center} zoom={13} scrollWheelZoom className="route-map">
        <TileLayer attribution="&copy; OpenStreetMap contributors" url={tileURL || "https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"} />
        {points.length > 1 && <Polyline pathOptions={paceSegments.length > 0 ? { color: "#53635f", weight: 4, opacity: 0.18 } : { color: "#d85c41", weight: 4 }} positions={points} />}
        {paceSegments.map((segment, index) => (
          <Polyline key={`${index}-${segment.color}`} positions={segment.points} pathOptions={{ color: segment.color, weight: 6, opacity: 0.98 }} interactive={false} />
        ))}
        <ActivityClimbMapSegments climbSegments={climbSegments} selectedClimbIndex={selectedClimbIndex} onSelectClimb={onSelectClimb} />
        {start && <Marker position={start} icon={routeEndpointIcon("start")} interactive={false} keyboard={false} />}
        {end && <Marker position={end} icon={routeEndpointIcon("end")} interactive={false} keyboard={false} />}
        {highlightedPoint && <Marker position={highlightedPoint} icon={routeHighlightIcon()} interactive={false} keyboard={false} zIndexOffset={1000} />}
        <ActivityMediaMapMarkers mediaMarkers={mediaMarkers} selectedMediaId={selectedMediaId} onSelectMedia={onSelectMedia} />
        <FitMapContent points={mapPoints} />
      </MapContainer>
      {showRouteColorSelector && onRouteColorSourceChange && (
        <ActivityRouteColorSourceControl
          source={routeColorSource ?? "pace"}
          onSelect={onRouteColorSourceChange}
        />
      )}
      {paceSegments.length > 0 && <ActivityPaceRouteLegend source={routeColorSource ?? "pace"} />}
      {(!tileURL || tileURL.includes("tile.openstreetmap.org")) && <p className="muted map-privacy-warning">Map tiles are loaded from OpenStreetMap; your browser and approximate route location are visible to that provider.</p>}
    </div>
  );
}

function ActivityPaceRouteLegend({ source }: { source: RouteColorSource }) {
  const label = source === "gap" ? "GAP" : "pace";
  return (
    <div className="pace-route-legend" aria-label="Route pace color legend">
      <span>slowest {label}</span>
      <span className="pace-route-legend-gradient" style={{ background: `linear-gradient(to right, ${PACE_ROUTE_COLORS.join(", ")})` }} />
      <span>fastest {label}</span>
    </div>
  );
}

function ActivityRouteColorSourceControl({
  source,
  onSelect
}: {
  source: RouteColorSource;
  onSelect: (source: RouteColorSource) => void;
}) {
  return (
    <div className={`route-color-source-slider${source === "gap" ? " gap" : ""}`} role="radiogroup" aria-label="Route color source">
      <span className="route-color-source-slider-thumb" aria-hidden="true" />
      <button
        type="button"
        className={source === "pace" ? "active" : ""}
        aria-pressed={source === "pace"}
        onClick={() => onSelect("pace")}
      >
        Pace
      </button>
      <button
        type="button"
        className={source === "gap" ? "active" : ""}
        aria-pressed={source === "gap"}
        onClick={() => onSelect("gap")}
      >
        GAP
      </button>
    </div>
  );
}

function ActivityClimbMapSegments({
  climbSegments,
  selectedClimbIndex,
  onSelectClimb
}: {
  climbSegments: ClimbMapSegment[];
  selectedClimbIndex?: number;
  onSelectClimb?: (climb: ActivityClimb) => void;
}) {
  const selectedSegment = climbSegments.find((segment) => segment.climb.index === selectedClimbIndex);
  return (
    <>
      {climbSegments.map((segment) => (
        <ClimbStartMarkerLayer
          key={segment.climb.index}
          segment={segment}
          selected={segment.climb.index === selectedClimbIndex}
          onSelectClimb={onSelectClimb}
        />
      ))}
      {selectedSegment && <SelectedClimbMapSegmentLayer segment={selectedSegment} onSelectClimb={onSelectClimb} />}
    </>
  );
}

function ClimbStartMarkerLayer({
  segment,
  selected,
  onSelectClimb
}: {
  segment: ClimbMapSegment;
  selected: boolean;
  onSelectClimb?: (climb: ActivityClimb) => void;
}) {
  const eventHandlers = onSelectClimb ? { click: () => onSelectClimb(segment.climb) } : undefined;
  if (!segment.start) {
    return null;
  }
  return (
    <Marker
      position={segment.start}
      icon={climbStartMarkerIcon(selected)}
      zIndexOffset={selected ? 1100 : 700}
      title={`Climb ${segment.climb.index + 1}`}
      eventHandlers={eventHandlers}
    />
  );
}

function SelectedClimbMapSegmentLayer({
  segment,
  onSelectClimb
}: {
  segment: ClimbMapSegment;
  onSelectClimb?: (climb: ActivityClimb) => void;
}) {
  const eventHandlers = onSelectClimb ? { click: () => onSelectClimb(segment.climb) } : undefined;
  if (segment.points.length <= 1) {
    return null;
  }
  return (
    <Polyline
      positions={segment.points}
      pathOptions={{ color: "#f6c432", weight: 8, opacity: 0.98 }}
      eventHandlers={eventHandlers}
    />
  );
}

function ActivityMediaMapMarkers({
  mediaMarkers,
  selectedMediaId,
  onSelectMedia
}: {
  mediaMarkers: ActivityMedia[];
  selectedMediaId?: string;
  onSelectMedia?: (mediaId: string) => void;
}) {
  return (
    <>
      {mediaMarkers.map((item) => {
        const point = mediaRoutePoint(item);
        if (!point) {
          return null;
        }
        const selected = item.id === selectedMediaId;
        return (
          <Marker
            key={item.id}
            position={point}
            icon={mediaMapMarkerIcon(item, selected)}
            zIndexOffset={selected ? 1200 : 800}
            title={item.originalFilename}
            eventHandlers={onSelectMedia ? { click: () => onSelectMedia(item.id) } : undefined}
          />
        );
      })}
    </>
  );
}

function FitMapContent({ points }: { points: RoutePoint[] }) {
  const map = useMap();
  const pointsKey = routePointsKey(points);
  useEffect(() => {
    if (points.length > 1) {
      map.fitBounds(points, { padding: [24, 24] });
    } else if (points.length === 1) {
      map.setView(points[0], 15);
    }
  }, [map, pointsKey]);
  return null;
}

function mediaRoutePoint(media: ActivityMedia): RoutePoint | undefined {
  if (!hasMediaLocation(media)) {
    return undefined;
  }
  return [media.latitude!, media.longitude!];
}

function routeEndpointIcon(kind: "start" | "end") {
  const label = kind === "start" ? "Start" : "End";
  return divIcon({
    className: "route-endpoint-marker-icon",
    html: `<span class="route-endpoint-marker ${kind}">${label}</span>`,
    iconSize: [56, 26],
    iconAnchor: [28, 13]
  });
}

function routeHighlightIcon() {
  return divIcon({
    className: "route-highlight-marker-icon",
    html: `<span class="route-highlight-marker"></span>`,
    iconSize: [18, 18],
    iconAnchor: [9, 9]
  });
}

function climbStartMarkerIcon(selected: boolean) {
  const size = selected ? 34 : 28;
  return divIcon({
    className: `climb-start-marker-icon${selected ? " selected" : ""}`,
    html: `<span class="climb-start-marker" style="--climb-marker-size:${size}px"><span class="climb-start-marker-peak"></span></span>`,
    iconSize: [size, size],
    iconAnchor: [size / 2, size / 2]
  });
}

function mediaMapMarkerIcon(media: ActivityMedia, selected: boolean) {
  const size = selected ? 52 : 44;
  return divIcon({
    className: `media-map-marker-icon${selected ? " selected" : ""}`,
    html: `<span class="media-map-marker" style="--media-marker-size:${size}px"><span class="media-map-marker-image" style="background-image:url('${activityMediaThumbnailURL(media.id)}')"></span></span>`,
    iconSize: [size, size],
    iconAnchor: [size / 2, size / 2]
  });
}

function routePointsKey(points: RoutePoint[]) {
  return points.map(([latitude, longitude]) => `${latitude.toFixed(6)},${longitude.toFixed(6)}`).join("|");
}

function routeForActivity(activity: Activity): RoutePoint[] {
  const samplePoints = (activity.samples ?? [])
    .filter((sample) => typeof sample.latitude === "number" && typeof sample.longitude === "number")
    .map((sample) => [sample.latitude!, sample.longitude!] as RoutePoint);
  if (samplePoints.length > 1) {
    return samplePoints;
  }
  if (activity.summaryPolyline) {
    return decodePolyline(activity.summaryPolyline);
  }
  return [];
}

function canExportActivityGPX(activity: Activity) {
  return (activity.samples ?? []).filter((sample) => typeof sample.latitude === "number" && typeof sample.longitude === "number").length > 1;
}

function paceScaleForActivity(activity: Activity, source: RouteColorSource = "pace") {
  if (source === "gap") {
    const gapPaces = (activity.samples ?? [])
      .map((sample) => lapGapPaceForSample(activity.laps ?? [], sample))
      .filter((pace): pace is number => typeof pace === "number" && Number.isFinite(pace) && pace > 0);
    if (gapPaces.length > 0) {
      return paceScaleFromPaces(gapPaces);
    }
  }
  return paceScaleFromSpeeds((activity.samples ?? []).map((sample) => sample.speedMPS));
}

function paceRouteSegmentsForActivity(
  activity: Activity,
  paceScale?: PaceDisplayScale,
  source: RouteColorSource = "pace"
): PaceRouteSegment[] {
  const samples = (activity.samples ?? [])
    .filter((sample) => typeof sample.latitude === "number" && typeof sample.longitude === "number")
    .map((sample) => ({
      point: [sample.latitude!, sample.longitude!] as RoutePoint,
      speedMPS: typeof sample.speedMPS === "number" && Number.isFinite(sample.speedMPS) && sample.speedMPS > 0 ? sample.speedMPS : undefined,
      gapPaceSPKM: lapGapPaceForSample(activity.laps ?? [], sample)
    }));
  if (samples.length < 2) {
    return [];
  }

  const segments: Array<{ start: RoutePoint; end: RoutePoint; paceSPKM: number }> = [];
  for (let index = 1; index < samples.length; index += 1) {
    const paceSPKM = source === "gap" ? (
      samples[index].gapPaceSPKM ?? samples[index - 1].gapPaceSPKM ?? paceForRouteSegment(samples[index - 1].speedMPS, samples[index].speedMPS)
    ) : paceForRouteSegment(samples[index - 1].speedMPS, samples[index].speedMPS);
    if (paceSPKM === undefined) {
      continue;
    }
    segments.push({ start: samples[index - 1].point, end: samples[index].point, paceSPKM });
  }
  if (segments.length === 0) {
    return [];
  }

  return segments.reduce<PaceRouteSegment[]>((grouped, segment) => {
    const color = paceScale ? paceColorForPace(segment.paceSPKM, paceScale) : PACE_ROUTE_COLORS[Math.floor(PACE_ROUTE_COLORS.length / 2)];
    const previous = grouped[grouped.length - 1];
    if (previous?.color === color && routePointsEqual(previous.points[previous.points.length - 1], segment.start)) {
      previous.points.push(segment.end);
      return grouped;
    }
    grouped.push({ color, points: [segment.start, segment.end] });
    return grouped;
  }, []);
}

function lapGapPaceForSample(laps: ActivityLap[], sample: ActivitySample): number | undefined {
  if (typeof sample.distanceM !== "number" || !Number.isFinite(sample.distanceM)) {
    return undefined;
  }
  let lapStartDistance = 0;
  const sortedLaps = laps.slice().sort((left, right) => left.index - right.index);
  for (const lap of sortedLaps) {
    const lapEndDistance = lapStartDistance + (typeof lap.distanceM === "number" ? lap.distanceM : 0);
    if (sample.distanceM >= lapStartDistance && sample.distanceM <= lapEndDistance) {
      return lap.avgGradeAdjustedPaceSPKM;
    }
    lapStartDistance = lapEndDistance;
  }
  const fallbackLap = sortedLaps.find((lap) => lap.avgGradeAdjustedPaceSPKM !== undefined);
  return fallbackLap?.avgGradeAdjustedPaceSPKM;
}

function routePointsEqual(left?: RoutePoint, right?: RoutePoint) {
  return Boolean(left && right && left[0] === right[0] && left[1] === right[1]);
}

function routeForClimb(activity: Activity, climb?: ActivityClimb): RoutePoint[] {
  return samplesForClimb(activity, climb)
    .filter((sample) => typeof sample.latitude === "number" && typeof sample.longitude === "number")
    .map((sample) => [sample.latitude!, sample.longitude!] as RoutePoint);
}

function climbMapSegmentsFor(activity: Activity, climbs: ActivityClimb[]): ClimbMapSegment[] {
  return climbs
    .map((climb) => {
      const points = routeForClimb(activity, climb);
      return { climb, points, start: points[0] };
    })
    .filter((segment) => segment.points.length > 1 || segment.start);
}

function climbProfileFor(activity: Activity, climb?: ActivityClimb): ClimbProfilePoint[] {
  if (!climb) {
    return [];
  }
  const points = chartDataFor(samplesForClimb(activity, climb))
    .filter((sample) => typeof sample.distanceM === "number" && typeof sample.elevationM === "number")
    .map((sample) => {
      const distanceKm = Math.max(0, (sample.distanceM! - climb.startDistanceM) / 1000);
      return {
        label: `${distanceKm.toFixed(1)} km`,
        distanceKm,
        elevationM: sample.elevationM!
      };
    });
  return normalizeClimbProfileElevation(points);
}

function normalizeClimbProfileElevation(points: ClimbProfilePoint[]): ClimbProfilePoint[] {
  const baseline = points[0]?.elevationM;
  if (baseline === undefined) {
    return points;
  }
  return points.map((point) => ({
    ...point,
    elevationM: Math.max(0, point.elevationM - baseline)
  }));
}

function samplesForClimb(activity: Activity, climb?: ActivityClimb) {
  if (!climb) {
    return [];
  }
  return (activity.samples ?? []).filter((sample) => sample.index >= climb.startSampleIndex && sample.index <= climb.endSampleIndex);
}

function hasActivityFilters(filters: ActivityTypeFiltersValue) {
  return Boolean(
    filters.search?.trim() ||
    filters.dateFrom ||
    filters.dateTo ||
    filters.sports.length > 0 ||
    filters.excludeSports.length > 0
  );
}

function hasDateFilters(filters: ActivityTypeFiltersValue) {
  return Boolean(filters.dateFrom || filters.dateTo);
}

function activityFiltersFromSearchParams(params: URLSearchParams): ActivityTypeFiltersValue {
  const filters: ActivityTypeFiltersValue = {
    ...emptyActivityTypeFilters,
    sports: compactSearchParamValues(params, "sport", "sports"),
    excludeSports: compactSearchParamValues(params, "excludeSport", "excludeSports"),
    search: params.get("search")?.trim() ?? "",
    dateFrom: params.get("dateFrom") ?? "",
    dateTo: params.get("dateTo") ?? "",
    sortBy: parseActivitySortBy(params.get("sortBy")),
    sortOrder: parseActivitySortOrder(params.get("sortOrder"))
  };
  return {
    ...filters,
    ...normalizedActivitySort(filters)
  };
}

function activityFiltersToSearchParams(filters: ActivityTypeFiltersValue) {
  const params = new URLSearchParams();
  for (const sport of filters.sports) {
    params.append("sport", sport);
  }
  for (const sport of filters.excludeSports) {
    params.append("excludeSport", sport);
  }
  if (filters.search?.trim()) {
    params.set("search", filters.search.trim());
  }
  if (filters.dateFrom) {
    params.set("dateFrom", filters.dateFrom);
  }
  if (filters.dateTo) {
    params.set("dateTo", filters.dateTo);
  }

  const sort = normalizedActivitySort(filters);
  if (!activitySortsMatch(sort, defaultActivitySort)) {
    params.set("sortBy", sort.sortBy);
    params.set("sortOrder", sort.sortOrder);
  }
  return params;
}

function compactSearchParamValues(params: URLSearchParams, ...keys: string[]) {
  const seen = new Set<string>();
  const values: string[] = [];
  for (const key of keys) {
    for (const raw of params.getAll(key)) {
      for (const part of raw.split(",")) {
        const value = part.trim();
        if (!value || seen.has(value)) {
          continue;
        }
        seen.add(value);
        values.push(value);
      }
    }
  }
  return values;
}

function parseActivitySortBy(value: string | null): ActivitySortBy {
  return activitySortOptions().some((option) => option.value === value) ? (value as ActivitySortBy) : defaultActivitySort.sortBy;
}

function isGearSortBy(value: string | null): value is GearSortBy {
  return gearSortByOptions.some((option) => option.value === value);
}

function sortGears(gears: Gear[], sortBy: GearSortBy) {
  return [...gears].sort((left, right) => {
    const leftValue = gearSortValue(left, sortBy);
    const rightValue = gearSortValue(right, sortBy);
    if ((leftValue ?? Number.NEGATIVE_INFINITY) > (rightValue ?? Number.NEGATIVE_INFINITY)) {
      return -1;
    }
    if ((leftValue ?? Number.NEGATIVE_INFINITY) < (rightValue ?? Number.NEGATIVE_INFINITY)) {
      return 1;
    }
    const leftName = gearDisplayName(left);
    const rightName = gearDisplayName(right);
    return leftName.localeCompare(rightName);
  });
}

function gearSortValue(gear: Gear, sortBy: GearSortBy): number {
  switch (sortBy) {
    case "activity_count":
      return typeof gear.activityCount === "number" && Number.isFinite(gear.activityCount) ? gear.activityCount : Number.NEGATIVE_INFINITY;
    case "first_used":
      return parseGearDate(gear.firstUsedAt);
    case "last_used":
      return parseGearDate(gear.lastUsedAt);
    case "distance_percent": {
      const percent = gearDistanceUsagePercentRaw(gear.totalDistanceM, gear.maxDistanceM);
      return Number.isFinite(percent) ? percent : Number.NEGATIVE_INFINITY;
    }
    case "distance":
    default:
      return isFiniteNumber(gear.totalDistanceM) ? gear.totalDistanceM : Number.NEGATIVE_INFINITY;
  }
}

function parseGearDate(value?: string): number {
  if (!value) {
    return Number.NEGATIVE_INFINITY;
  }
  const time = Date.parse(value);
  return Number.isFinite(time) ? time : Number.NEGATIVE_INFINITY;
}

function parseActivitySortOrder(value: string | null) {
  return value === "asc" || value === "desc" ? value : defaultActivitySort.sortOrder;
}

function activitySortOptions(): Array<{ value: ActivitySortBy; label: string }> {
  return [
    { value: "date", label: "Date" },
    { value: "duration", label: "Duration" },
    { value: "distance", label: "Distance" },
    { value: "elevation_gain", label: "Elevation gain" },
    { value: "avg_pace", label: "Avg pace" },
    { value: "calories", label: "Calories" }
  ];
}

function normalizedActivitySort(filters: ActivityTypeFiltersValue): ActivitySort {
  return {
    sortBy: filters.sortBy && activitySortOptions().some((option) => option.value === filters.sortBy) ? filters.sortBy : defaultActivitySort.sortBy,
    sortOrder: filters.sortOrder === "asc" || filters.sortOrder === "desc" ? filters.sortOrder : defaultActivitySort.sortOrder
  };
}

function activitySortsMatch(left: ActivitySort, right: ActivitySort) {
  return left.sortBy === right.sortBy && left.sortOrder === right.sortOrder;
}

function dateFilterPresets(): Array<{ id: string; label: string; range: ActivityDateRange }> {
  const today = new Date();
  const currentYear = today.getFullYear();
  const currentMonth = today.getMonth();
  return [
    { id: "last-7-days", label: "Last 7 days", range: dateRange(addDays(today, -6), today) },
    { id: "last-30-days", label: "Last 30 days", range: dateRange(addDays(today, -29), today) },
    { id: "last-90-days", label: "Last 90 days", range: dateRange(addDays(today, -89), today) },
    { id: "this-month", label: "This month", range: dateRange(new Date(currentYear, currentMonth, 1), today) },
    { id: "last-month", label: "Last month", range: dateRange(new Date(currentYear, currentMonth - 1, 1), new Date(currentYear, currentMonth, 0)) },
    { id: "this-year", label: "This year", range: dateRange(new Date(currentYear, 0, 1), today) },
    { id: "last-year", label: "Last year", range: dateRange(new Date(currentYear - 1, 0, 1), new Date(currentYear - 1, 11, 31)) }
  ];
}

function dateRange(start: Date, end: Date): ActivityDateRange {
  return {
    dateFrom: dateInputValue(start),
    dateTo: dateInputValue(end)
  };
}

function addDays(date: Date, days: number) {
  const next = new Date(date);
  next.setDate(next.getDate() + days);
  return next;
}

function dateInputValue(date: Date) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function dateRangesMatch(left: ActivityDateRange, right: ActivityDateRange) {
  return (left.dateFrom ?? "") === (right.dateFrom ?? "") && (left.dateTo ?? "") === (right.dateTo ?? "");
}

function chartDataFor(samples: ActivitySample[], paceScale = paceScaleFromSpeeds(samples.map((sample) => sample.speedMPS))): ActivityChartPoint[] {
  const points = samples.map((sample, index) => {
    const rawPaceSPKM = speedToPaceSPKM(sample.speedMPS);
    return {
      index: sample.index ?? index,
      label: sample.distanceM !== undefined ? `${(sample.distanceM / 1000).toFixed(1)} km` : String(index + 1),
      distanceM: typeof sample.distanceM === "number" ? sample.distanceM : undefined,
      latitude: typeof sample.latitude === "number" ? sample.latitude : undefined,
      longitude: typeof sample.longitude === "number" ? sample.longitude : undefined,
      elevationM: typeof sample.elevationM === "number" ? sample.elevationM : undefined,
      heartRate: sample.heartRate,
      paceSPKM: clampPaceToScale(rawPaceSPKM, paceScale),
      rawPaceSPKM,
      power: sample.power,
      cadence: sample.cadence
    };
  });
  return smoothElevationSeries(points);
}

function routePointForChartPoint(point?: ActivityChartPoint): RoutePoint | undefined {
  if (typeof point?.latitude === "number" && Number.isFinite(point.latitude) && typeof point.longitude === "number" && Number.isFinite(point.longitude)) {
    return [point.latitude, point.longitude];
  }
  return undefined;
}

function smoothElevationSeries(points: ActivityChartPoint[]): ActivityChartPoint[] {
  if (points.length < 3 || !points.some((point) => typeof point.elevationM === "number")) {
    return points;
  }
  if (hasMonotonicDistances(points)) {
    return smoothElevationByDistance(points);
  }
  return smoothElevationBySampleWindow(points);
}

function hasMonotonicDistances(points: ActivityChartPoint[]) {
  let previousDistance = -Infinity;
  let seenDistance = false;
  for (const point of points) {
    if (typeof point.distanceM !== "number" || !Number.isFinite(point.distanceM)) {
      return false;
    }
    if (point.distanceM < previousDistance) {
      return false;
    }
    previousDistance = point.distanceM;
    seenDistance = true;
  }
  return seenDistance;
}

function smoothElevationByDistance(points: ActivityChartPoint[]) {
  let left = 0;
  let right = 0;
  let sum = 0;
  let count = 0;
  return points.map((point) => {
    const center = point.distanceM!;
    while (right < points.length) {
      const rightPoint = points[right];
      if (!rightPoint || rightPoint.distanceM! > center + ELEVATION_SMOOTHING_RADIUS_M) {
        break;
      }
      if (typeof rightPoint.elevationM === "number") {
        sum += rightPoint.elevationM;
        count++;
      }
      right++;
    }
    while (left < points.length) {
      const leftPoint = points[left];
      if (!leftPoint || leftPoint.distanceM! >= center - ELEVATION_SMOOTHING_RADIUS_M) {
        break;
      }
      if (typeof leftPoint.elevationM === "number") {
        sum -= leftPoint.elevationM;
        count--;
      }
      left++;
    }
    if (typeof point.elevationM !== "number" || count === 0) {
      return point;
    }
    return { ...point, elevationM: sum / count };
  });
}

function smoothElevationBySampleWindow(points: ActivityChartPoint[]) {
  return points.map((point, index) => {
    if (typeof point.elevationM !== "number") {
      return point;
    }
    let sum = 0;
    let count = 0;
    const start = Math.max(0, index - ELEVATION_SMOOTHING_SAMPLE_RADIUS);
    const end = Math.min(points.length - 1, index + ELEVATION_SMOOTHING_SAMPLE_RADIUS);
    for (let i = start; i <= end; i++) {
      const sample = points[i];
      if (typeof sample?.elevationM === "number") {
        sum += sample.elevationM;
        count++;
      }
    }
    return count > 0 ? { ...point, elevationM: sum / count } : point;
  });
}

function chartPointFromMouseState(state: unknown, data: ActivityChartPoint[]): ActivityChartPoint | undefined {
  if (!state || typeof state !== "object" || !("activeTooltipIndex" in state)) {
    return undefined;
  }
  const tooltipIndex = (state as { activeTooltipIndex?: unknown }).activeTooltipIndex;
  const index = typeof tooltipIndex === "number" ? tooltipIndex : typeof tooltipIndex === "string" ? Number(tooltipIndex) : NaN;
  if (!Number.isInteger(index) || index < 0 || index >= data.length) {
    return undefined;
  }
  return data[index];
}

function formatChartTick(value: number, series: ActivityChartSeries) {
  if (!Number.isFinite(value)) {
    return "";
  }
  if (series.key === "paceSPKM") {
    const minutes = Math.floor(value / 60);
    const seconds = Math.round(value % 60);
    return `${minutes}:${String(seconds).padStart(2, "0")}`;
  }
  return String(Math.round(value));
}

function formatChartTooltip(value: unknown, name: string, seriesList: ActivityChartSeries[], item?: unknown) {
  const series = seriesList.find((item) => item.label === name);
  const numericValue = typeof value === "number" ? value : Number(value);
  if (!series || !Number.isFinite(numericValue)) {
    return [String(value), name];
  }
  const rawPace = series.key === "paceSPKM" ? chartPayloadNumber(item, "rawPaceSPKM") : undefined;
  return [series.format(rawPace ?? numericValue), name];
}

function chartPayloadNumber(item: unknown, key: keyof ActivityChartPoint) {
  const payload = chartPayload(item);
  const value = payload?.[key];
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function chartPayload(item: unknown): Partial<Record<keyof ActivityChartPoint, unknown>> | undefined {
  if (!item || typeof item !== "object" || !("payload" in item)) {
    return undefined;
  }
  const payload = (item as { payload?: Partial<Record<keyof ActivityChartPoint, unknown>> }).payload;
  return payload && typeof payload === "object" ? payload : undefined;
}

function decodePolyline(encoded: string): RoutePoint[] {
  let index = 0;
  let lat = 0;
  let lng = 0;
  const coordinates: RoutePoint[] = [];

  while (index < encoded.length) {
    let result = 0;
    let shift = 0;
    let byte = 0;
    do {
      byte = encoded.charCodeAt(index++) - 63;
      result |= (byte & 0x1f) << shift;
      shift += 5;
    } while (byte >= 0x20);
    lat += result & 1 ? ~(result >> 1) : result >> 1;

    result = 0;
    shift = 0;
    do {
      byte = encoded.charCodeAt(index++) - 63;
      result |= (byte & 0x1f) << shift;
      shift += 5;
    } while (byte >= 0x20);
    lng += result & 1 ? ~(result >> 1) : result >> 1;
    coordinates.push([lat / 1e5, lng / 1e5]);
  }
  return coordinates;
}

function activityMediaThumbnailURL(mediaId: string) {
  return `/api/activity-media/${encodeURIComponent(mediaId)}/thumbnail`;
}

function activityMediaOriginalURL(mediaId: string) {
  return `/api/activity-media/${encodeURIComponent(mediaId)}/original`;
}

function mergeActivityMedia(current: ActivityMedia[], uploaded: ActivityMedia[]) {
  const byId = new Map(current.map((item) => [item.id, item]));
  for (const item of uploaded) {
    byId.set(item.id, item);
  }
  return Array.from(byId.values()).sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime());
}

function formatActivityMediaMeta(media: ActivityMedia) {
  const parts: string[] = [];
  if (media.width > 0 && media.height > 0) {
    parts.push(`${media.width} x ${media.height}`);
  }
  parts.push(formatFileSize(media.sizeBytes));
  if (media.captureTime) {
    parts.push(formatDate(media.captureTime));
  }
  if (hasMediaLocation(media)) {
    parts.push("GPS");
  }
  return parts.join(" · ");
}

function hasMediaLocation(media: ActivityMedia) {
  return typeof media.latitude === "number" && Number.isFinite(media.latitude) && typeof media.longitude === "number" && Number.isFinite(media.longitude);
}

function formatMediaLocation(media: ActivityMedia) {
  const latitude = media.latitude;
  const longitude = media.longitude;
  if (typeof latitude !== "number" || typeof longitude !== "number") {
    return "";
  }
  return `${latitude.toFixed(5)}, ${longitude.toFixed(5)}`;
}

function formatFileSize(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return "0 B";
  }
  const units = ["B", "KB", "MB", "GB"];
  let size = bytes;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  const precision = unitIndex === 0 || size >= 10 ? 0 : 1;
  return `${size.toFixed(precision)} ${units[unitIndex]}`;
}

function LoadingRow() {
  return <div className="loading"><Database size={18} /> Loading</div>;
}

function EmptyState({ title, action }: { title: string; action?: ReactNode }) {
  return (
    <div className="empty-state">
      <span>{title}</span>
      {action}
    </div>
  );
}

function FullScreenMessage({ title, message }: { title: string; message: string }) {
  return (
    <div className="login-page">
      <div className="login-panel">
        <div className="brand login-brand"><ActivityIcon size={26} /><span>{title}</span></div>
        <p className="muted">{message}</p>
      </div>
    </div>
  );
}

function formatDate(value: string) {
  return new Date(value).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
}

function gearDisplayName(gear: GearSummary) {
  const name = gear.name?.trim();
  if (name) {
    return name;
  }
  const fallback = [gear.brand, gear.model].map((part) => part?.trim()).filter(Boolean).join(" ");
  return fallback || "Garmin gear";
}

function gearDisplayLabel(gear: GearSummary) {
  const name = gearDisplayName(gear);
  const subtitle = gearSubtitle(gear);
  return subtitle && subtitle !== name ? `${name} · ${subtitle}` : name;
}

function gearSubtitle(gear: GearSummary) {
  const model = [gear.brand, gear.model].map((part) => part?.trim()).filter(Boolean).join(" ");
  if (model && model !== gearDisplayName(gear)) {
    return model;
  }
  return formatGearDefaults(gear.defaultActivityTypes);
}

function gearDetailItems(gear: Gear) {
  return [
    { label: "Brand", value: gear.brand?.trim() ?? "" },
    { label: "Model", value: gear.model?.trim() ?? "" },
    { label: "Garmin distance", value: formatOptionalGearDistance(gear.totalDistanceM) },
    { label: "Activity count", value: formatGearActivityCount(gear.activityCount) },
    { label: "Distance limit", value: formatOptionalGearDistance(gear.maxDistanceM) },
    { label: "First used", value: formatOptionalDate(gear.firstUsedAt) },
    { label: "Last used", value: formatOptionalDate(gear.lastUsedAt) },
    { label: "Default activity types", value: formatGearDefaults(gear.defaultActivityTypes) },
    { label: "Provider", value: formatSourceName(gear.provider) },
    { label: "Provider gear ID", value: gear.providerGearId }
  ].filter((item) => item.value !== "");
}

function formatGearType(value?: string) {
  const cleaned = value?.trim().replace(/[_-]+/g, " ");
  if (!cleaned) {
    return "Gear";
  }
  return cleaned.toLowerCase().replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function formatGearDefaults(values?: string[]) {
  const formatted = (values ?? []).map(formatGearType).filter((value) => value !== "Gear");
  return formatted.length > 0 ? formatted.join(", ") : "";
}

function formatOptionalDate(value?: string) {
  return value ? formatDate(value) : "";
}

function formatOptionalGearDistance(value?: number) {
  return isFiniteNumber(value) ? formatGearDistance(value) : "";
}

function formatGearDistance(value: number) {
  const kilometers = value / 1000;
  const precision = kilometers >= 100 ? 0 : 1;
  return `${kilometers.toFixed(precision)} km`;
}

function formatGearActivityCount(value?: number) {
  if (value === undefined || !Number.isFinite(value)) {
    return "0";
  }
  return value.toLocaleString();
}

function gearDistanceUsagePercent(totalDistanceM?: number, maxDistanceM?: number) {
  const raw = gearDistanceUsagePercentRaw(totalDistanceM, maxDistanceM);
  return `${Math.max(0, raw)}%`;
}

function gearDistanceUsagePercentRaw(totalDistanceM?: number, maxDistanceM?: number) {
  if (totalDistanceM === undefined || maxDistanceM === undefined || !Number.isFinite(totalDistanceM) || !Number.isFinite(maxDistanceM) || maxDistanceM <= 0) {
    return Number.NaN;
  }
  const ratio = totalDistanceM / maxDistanceM;
  const percent = ratio * 100;
  return percent >= 0 ? Math.min(100, Math.round(percent)) : 0;
}

function formatDistance(value: number) {
  return `${(value / 1000).toFixed(value >= 100000 ? 0 : 1)} km`;
}

function formatDistanceRange(startM: number, endM: number) {
  return `${(startM / 1000).toFixed(1)}-${(endM / 1000).toFixed(1)} km`;
}

function formatGrade(value: number) {
  if (!Number.isFinite(value)) {
    return "-";
  }
  return `${value.toFixed(1).replace(/\.0$/, "")}%`;
}

function difficultyClass(value: string) {
  return value.toLowerCase().replace(/\s+/g, "-");
}

function formatDuration(totalSeconds: number) {
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (hours > 0) {
    return `${hours}:${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
  }
  return `${minutes}:${String(seconds).padStart(2, "0")}`;
}

function lapPaceSPKM(lap: NonNullable<Activity["laps"]>[number], samples: ActivitySample[]) {
  if (lap.avgPaceSPKM !== undefined && Number.isFinite(lap.avgPaceSPKM) && lap.avgPaceSPKM > 0) {
    return lap.avgPaceSPKM;
  }
  if (lap.distanceM <= 0) {
    return undefined;
  }
  const movingTimeS = lapMovingTimeS(lap, samples);
  if (movingTimeS <= 0) {
    return undefined;
  }
  return movingTimeS / (lap.distanceM / 1000);
}

function lapMovingTimeS(lap: NonNullable<Activity["laps"]>[number], samples: ActivitySample[]) {
  if (lap.movingTimeS > 0) {
    return lap.movingTimeS;
  }
  return movingLapTimeFromSamples(lap, samples);
}

function movingLapTimeFromSamples(lap: NonNullable<Activity["laps"]>[number], samples: ActivitySample[]) {
  if (!lap.startTime || lap.elapsedTimeS <= 0 || samples.length < 2) {
    return lap.elapsedTimeS;
  }
  const startMs = Date.parse(lap.startTime);
  if (!Number.isFinite(startMs)) {
    return lap.elapsedTimeS;
  }
  const endMs = startMs + lap.elapsedTimeS * 1000;
  let movingMs = 0;
  for (let index = 1; index < samples.length; index += 1) {
    const previous = samples[index - 1];
    const current = samples[index];
    if (!previous.timestamp || !current.timestamp) {
      continue;
    }
    const previousMs = Date.parse(previous.timestamp);
    const currentMs = Date.parse(current.timestamp);
    const segmentStart = Math.max(startMs, previousMs);
    const segmentEnd = Math.min(endMs, currentMs);
    if (!Number.isFinite(previousMs) || !Number.isFinite(currentMs) || segmentEnd <= segmentStart) {
      continue;
    }
    const distanceDelta = (current.distanceM ?? 0) - (previous.distanceM ?? 0);
    const moving = (previous.speedMPS ?? 0) > 0.5 || (current.speedMPS ?? 0) > 0.5 || distanceDelta > 0.5;
    if (moving) {
      movingMs += segmentEnd - segmentStart;
    }
  }
  return movingMs > 0 ? Math.round(movingMs / 1000) : lap.elapsedTimeS;
}

function formatPace(secondsPerKm?: number) {
  if (!secondsPerKm || !Number.isFinite(secondsPerKm)) {
    return "-";
  }
  const minutes = Math.floor(secondsPerKm / 60);
  const seconds = Math.round(secondsPerKm % 60);
  return `${minutes}:${String(seconds).padStart(2, "0")} /km`;
}

function formatBPM(value?: number) {
  if (value === undefined || !Number.isFinite(value)) {
    return "-";
  }
  return `${Math.round(value).toLocaleString()} bpm`;
}

function climbSensitivityPresetForValue(value: number) {
  return climbSensitivityPresets.find((preset) => preset.value === value)?.id ?? "custom";
}

function climbSensitivityPresetLabel(value: number) {
  const preset = climbSensitivityPresets.find((candidate) => candidate.id === climbSensitivityPresetForValue(value));
  return preset ? preset.label : "Custom";
}

function clampClimbSensitivity(value: number) {
  if (!Number.isFinite(value)) {
    return defaultClimbSensitivity;
  }
  return Math.max(0, Math.min(100, Math.round(value)));
}

function formatCalories(value?: number) {
  if (value === undefined || !Number.isFinite(value)) {
    return "-";
  }
  return `${Math.round(value).toLocaleString()} kcal`;
}
