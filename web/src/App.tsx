import { Fragment, useEffect, useRef, useState } from "react";
import type { CSSProperties, ReactNode } from "react";
import { Link, NavLink, Navigate, Route, Routes, useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { QueryClient } from "@tanstack/react-query";
import { Activity as ActivityIcon, ArrowDown, ArrowUp, ArrowUpDown, BarChart3, CalendarDays, Calculator, ChevronDown, ChevronLeft, ChevronRight, Cloud, Columns3, Copy, Database, Download, ExternalLink, FileUp, Filter, Flame, Footprints, GripVertical, HeartPulse, LocateFixed, LogOut, Map as MapIcon, Maximize2, Menu, Minimize2, Moon, MoreHorizontal, MoreVertical, Pencil, RefreshCw, Route as RouteIcon, Scale, Mountain, Star, Timer, Settings as SettingsIcon, Square, StickyNote, Trash2, Upload, X, BatteryCharging, RotateCcw } from "lucide-react";
import { divIcon } from "leaflet";
import { Circle, MapContainer, Marker, Polyline, TileLayer, Tooltip as LeafletTooltip, useMap, useMapEvents } from "react-leaflet";
import { Area, AreaChart, Bar, BarChart, CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { activityGPXURL, courseGPXURL, api, ApiError, setCsrfToken } from "./api";
import { HEALTH_CHART_Y_AXIS_WIDTH, formatHealthAxisBPM, formatHealthAxisHours, formatHealthAxisInteger, formatHealthAxisMS } from "./healthChart";
import { PACE_ROUTE_COLORS, clampPaceToScale, formatPaceMinutesSeconds, paceColorForPace, paceForRouteSegment, paceScaleFromPaces, paceScaleFromSpeeds, speedToPaceSPKM } from "./paceDisplay";
import type { PaceDisplayScale } from "./paceDisplay";
import { reconcileVisibleActivitySeries } from "./activityChartSeries";
import { climbPerformanceFor, gapPaceForSample, samplesForClimbPerformance } from "./climbPerformance";
import type { ClimbPerformance } from "./climbPerformance";
import { plannedMatchResponseForDialog, PlannedActivityMatchAgenda } from "./plannedMatchAgenda";
import { plannedMatchPreviewForActivity, plannedMatchRequestIsCurrent } from "./plannedMatchPreview";
import { calendarPlanMatchDescription } from "./calendarPlanMatch";
import { applyThemePreference, parseThemePreference, themeOptions, themePreferenceForAccount } from "./theme";
import type { ThemePreference } from "./theme";
import { chartDisplayDomain } from "./activityChartBounds";
import { supportsRouteMetrics } from "./activityMetrics";
import { NotificationBell, NotificationSettingsSection, NotificationsPage, unregisterCurrentPushDevice } from "./notifications";
import { hasIntervalAnalysis, resolveActivityAnalysisTab } from "./activityAnalysis";
import type { ActivityAnalysisTab } from "./activityAnalysis";
import { fullPathForSimplePath, normalizeSimpleMatchFilter, shouldRedirectToSimple, simpleIntervalSummary, simpleMatchStatusLabel } from "./simpleMode";
import type { SimpleMatchFilter } from "./simpleMode";
import { trainingSheetWritebackStatusLabel } from "./trainingSheetWriteback";
import { trainingSheetSourceURL } from "./trainingSheetLink";
import type {
  Activity,
  ActivityClimb,
  ActivityInterval,
  ActivityLap,
  ActivityMedia,
  ActivityNavigation as ActivityNavigationData,
  ActivitySample,
  ActivityWorkoutStep,
  ActivitySortBy,
  ActivityTypeFilters as ActivityTypeFiltersValue,
  AppConfig,
  CalendarActivitySummary,
  Course,
  CourseImportCandidate,
  CourseImportPreview,
  CourseImportSelection,
  CourseLeg,
  CourseProfilePoint,
  CourseRoutingLeg,
  CourseSport,
  CourseSummary,
  CourseWaypoint,
  DailyHealthMetric,
  HealthChartPoint,
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
  UserPreference,
  Workout,
  WorkoutDefinition,
  WorkoutMutation,
  WorkoutStep
} from "./types";

type RoutePoint = [number, number];
type ActivityDateRange = Pick<ActivityTypeFiltersValue, "dateFrom" | "dateTo">;
type ActivitySort = Required<Pick<ActivityTypeFiltersValue, "sortBy" | "sortOrder">>;
type HealthDateRange = { from: string; to: string };
type GearSortBy = "first_used" | "last_used" | "activity_count" | "distance" | "distance_percent";
type ActivityTableColumnKey = "date" | "type" | "gear" | "distance" | "time" | "calories" | "source";
type ActivityChartSeriesKey = "elevationM" | "heartRate" | "paceSPKM" | "power" | "cadence";
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
  paceSPKM?: number;
  gapSPKM?: number;
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
const defaultActivitySort: ActivitySort = { sortBy: "date", sortOrder: "desc" };
const emptyActivityTypeFilters: ActivityTypeFiltersValue = { sports: [], excludeSports: [], search: "", dateFrom: "", dateTo: "", ...defaultActivitySort };
const ACTIVITY_LIST_PAGE_SIZE = 100;
const garminHealthDefaultDays = 7;
const healthBarChartMaxDays = 30;
const defaultClimbSensitivity = 50;
const vdotDistancePresets: Array<{ id: string; label: string; distanceKm: string }> = [
  { id: "marathon", label: "Marathon", distanceKm: "42.195" },
  { id: "half-marathon", label: "HM", distanceKm: "21.098" },
  { id: "10m", label: "10M", distanceKm: "16.093" },
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

type PwaInstallPromptEvent = Event & {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: "accepted" | "dismissed" }>;
};

function usePwaInstallPrompt() {
  const [installPrompt, setInstallPrompt] = useState<PwaInstallPromptEvent>();
  const [installed, setInstalled] = useState(false);

  useEffect(() => {
    const displayMode = window.matchMedia("(display-mode: standalone)");
    const updateInstalled = () => setInstalled(displayMode.matches || Boolean((navigator as Navigator & { standalone?: boolean }).standalone));
    const handleInstallPrompt = (event: Event) => {
      event.preventDefault();
      setInstallPrompt(event as PwaInstallPromptEvent);
    };
    const handleInstalled = () => {
      setInstalled(true);
      setInstallPrompt(undefined);
    };

    updateInstalled();
    displayMode.addEventListener("change", updateInstalled);
    window.addEventListener("beforeinstallprompt", handleInstallPrompt);
    window.addEventListener("appinstalled", handleInstalled);
    return () => {
      displayMode.removeEventListener("change", updateInstalled);
      window.removeEventListener("beforeinstallprompt", handleInstallPrompt);
      window.removeEventListener("appinstalled", handleInstalled);
    };
  }, []);

  const install = async () => {
    if (!installPrompt) {
      return;
    }
    await installPrompt.prompt();
    await installPrompt.userChoice;
    setInstallPrompt(undefined);
  };

  return { canInstall: Boolean(installPrompt) && !installed, install };
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
    defaultExperience: current?.defaultExperience ?? "full",
    ...updates
  };
}

function safeNextPath(value: string) {
  try {
    const parsed = new URL(value, window.location.origin);
    if (parsed.origin !== window.location.origin || !value.startsWith("/") || value.startsWith("//")) return "/";
    return `${parsed.pathname}${parsed.search}${parsed.hash}`;
  } catch {
    return "/";
  }
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
  const [themePreferenceUserID, setThemePreferenceUserID] = useState<string>();
  const pwa = usePwaInstallPrompt();
  const queryClient = useQueryClient();
  const location = useLocation();
  const session = useQuery({ queryKey: ["session"], queryFn: api.session });
  const effectiveUserID = session.data?.user?.id;
  const preferences = useQuery({
    queryKey: preferencesQueryKey(effectiveUserID),
    queryFn: api.preferences,
    enabled: Boolean(session.data?.authenticated && effectiveUserID)
  });
  const savePreferences = useSaveUserPreferences(effectiveUserID);

  useEffect(() => {
    applyThemePreference(themePreferenceForAccount(themePreference, themePreferenceUserID, effectiveUserID));
  }, [effectiveUserID, themePreference, themePreferenceUserID]);

  useEffect(() => {
    if (!effectiveUserID || !preferences.data) {
      setThemePreference("system");
      setThemePreferenceUserID(undefined);
      return;
    }
    setThemePreference(parseThemePreference(preferences.data.themePreference));
    setThemePreferenceUserID(effectiveUserID);
  }, [effectiveUserID, preferences.data]);

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
    const next = safeNextPath(`${location.pathname}${location.search}${location.hash}`);
    const loginPath = next === "/" ? "/login" : `/login?next=${encodeURIComponent(next)}`;
    return (
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="*" element={<Navigate to={loginPath} replace />} />
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
    <AuthenticatedApp
      session={session.data}
      preferences={preferences.data}
      preferencesLoading={preferences.isLoading}
      themePreference={themePreference}
      onThemePreferenceChange={onThemePreferenceChange}
      onDefaultExperienceChange={(value) => savePreferences.mutateAsync({ defaultExperience: value })}
      themePreferenceError={savePreferences.error}
      pwa={pwa}
    />
  );
}

function AuthenticatedApp({
  session,
  preferences,
  preferencesLoading,
  themePreference,
  onThemePreferenceChange,
  onDefaultExperienceChange,
  themePreferenceError,
  pwa
}: {
  session?: Session;
  preferences?: UserPreference;
  preferencesLoading: boolean;
  themePreference: ThemePreference;
  onThemePreferenceChange: (preference: ThemePreference) => void;
  onDefaultExperienceChange: (value: "full" | "simple") => Promise<UserPreference>;
  themePreferenceError?: Error | null;
  pwa: { canInstall: boolean; install: () => Promise<void> };
}) {
  const config = useQuery({ queryKey: ["config"], queryFn: api.config });
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const location = useLocation();

  useEffect(() => {
    const storedNext = window.sessionStorage.getItem("runnarrLoginNext");
    if (!storedNext) return;
    window.sessionStorage.removeItem("runnarrLoginNext");
    navigate(safeNextPath(storedNext), { replace: true });
  }, [navigate]);
  useEffect(() => {
    const params = new URLSearchParams(location.search);
    const notificationID = params.get("runnarrNotification");
    if (!notificationID) return;
    params.delete("runnarrNotification");
    const cleanPath = `${location.pathname}${params.size ? `?${params}` : ""}${location.hash}`;
    const timeout = window.setTimeout(() => {
      const markRead = session?.canWrite === false ? Promise.resolve() : api.setNotificationRead(notificationID, true).catch(() => undefined);
      void markRead.finally(() => {
        void queryClient.invalidateQueries({ queryKey: ["notifications"] });
        navigate(cleanPath, { replace: true });
      });
    });
    return () => window.clearTimeout(timeout);
  }, [location.hash, location.pathname, location.search, navigate, queryClient, session?.canWrite]);

  const logout = useMutation({
    mutationFn: async () => {
      await unregisterCurrentPushDevice();
      return api.logout();
    },
    onSuccess: async () => {
      setCsrfToken("");
      await queryClient.invalidateQueries({ queryKey: ["session"] });
      navigate("/login");
    }
  });

  if (preferencesLoading && location.pathname === "/") {
    return <FullScreenMessage title="Runnarr" message="Loading preferences" />;
  }

  if (location.pathname.startsWith("/simple")) {
    return (
      <SimpleAppShell
        session={session}
        onDefaultExperienceChange={onDefaultExperienceChange}
        onLogout={() => logout.mutate()}
        loggingOut={logout.isPending}
      />
    );
  }

  if (shouldRedirectToSimple(location.pathname, preferences?.defaultExperience, session?.supportMode)) {
    return <Navigate to="/simple" replace />;
  }

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
          <NavItem to="/courses" icon={<RouteIcon size={18} />} label="Courses" />
          <NavItem to="/workouts" icon={<Timer size={18} />} label="Workouts" />
          <NavItem to="/health" icon={<HeartPulse size={18} />} label="Health" />
          <NavItem to="/tools" icon={<Calculator size={18} />} label="Tools" />
          <NavItem to="/gear" icon={<Footprints size={18} />} label="Gear" />
        </nav>
        <div className="sidebar-bottom">
          <div className="account-chip" title={session?.user?.username}>
            <strong>{session?.user?.displayName || session?.user?.username}</strong>
            <span>{session?.user?.role === "admin" ? "Administrator" : "User"}</span>
          </div>
          <NotificationBell canWrite={session?.canWrite !== false} />
          <NavItem to="/settings" icon={<SettingsIcon size={18} />} label="Settings" />
          <button className="nav-button" type="button" onClick={() => logout.mutate()}>
            <LogOut size={18} />
            <span>Log out</span>
          </button>
        </div>
      </aside>
      <main className="main">
        <MobileNavigation session={session} onLogout={() => logout.mutate()} loggingOut={logout.isPending} pwa={pwa} />
        {session?.supportMode && (
          <div className="support-banner">
            <span>Read-only support view: {session.user?.displayName || session.user?.username}</span>
            <button className="secondary-button small-button" type="button" onClick={() => {
              void api.stopSupport().then(() => window.location.replace("/"));
            }}>Exit support view</button>
          </div>
        )}
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/activities" element={<ActivitiesPage />} />
          <Route path="/activities/:id" element={<ActivityDetailPage config={config.data} canWrite={session?.canWrite !== false} />} />
          <Route path="/calendar" element={<ActivityCalendarPage />} />
          <Route path="/calendar/day/:date" element={<CalendarDayPage />} />
          <Route path="/workouts" element={<WorkoutsPage />} />
          <Route path="/workouts/new" element={<WorkoutEditorPage />} />
          <Route path="/workouts/:id" element={<WorkoutEditorPage />} />
          <Route path="/courses" element={<CoursesPage canWrite={session?.canWrite !== false} />} />
          <Route path="/courses/new" element={<CoursePlannerPage canWrite={session?.canWrite !== false} mapTileURL={config.data?.mapTileURL} routingEnabled={config.data?.courseRoutingEnabled === true} />} />
          <Route path="/courses/import" element={<CourseImportPage canWrite={session?.canWrite !== false} mapTileURL={config.data?.mapTileURL} />} />
          <Route path="/courses/imports/:id" element={<CourseImportResultPage />} />
          <Route path="/courses/:id/plan" element={<CoursePlannerPage canWrite={session?.canWrite !== false} mapTileURL={config.data?.mapTileURL} routingEnabled={config.data?.courseRoutingEnabled === true} />} />
          <Route path="/courses/:id" element={<CourseDetailPage canWrite={session?.canWrite !== false} mapTileURL={config.data?.mapTileURL} />} />
          <Route path="/notifications" element={<NotificationsPage canWrite={session?.canWrite !== false} />} />
          <Route path="/health" element={<HealthPage />} />
          <Route path="/tools" element={<ToolsPage />} />
          <Route path="/gear" element={<GearPage />} />
          <Route path="/gear/:id" element={<GearDetailPage />} />
          <Route path="/imports" element={<Navigate to="/settings#import" replace />} />
          <Route path="/settings" element={<SettingsPage canWrite={session?.canWrite !== false} defaultExperience={preferences?.defaultExperience ?? "full"} onDefaultExperienceChange={onDefaultExperienceChange} themePreference={themePreference} onThemePreferenceChange={onThemePreferenceChange} themePreferenceError={themePreferenceError} />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  );
}

function SimpleAppShell({
  session,
  onDefaultExperienceChange,
  onLogout,
  loggingOut
}: {
  session?: Session;
  onDefaultExperienceChange: (value: "full" | "simple") => Promise<UserPreference>;
  onLogout: () => void;
  loggingOut: boolean;
}) {
  const location = useLocation();
  const navigate = useNavigate();
  const [exitError, setExitError] = useState("");
  const fullPath = fullPathForSimplePath(location.pathname);
  const exitSimpleMode = async () => {
    setExitError("");
    if (session?.canWrite !== false) {
      try {
        await onDefaultExperienceChange("full");
      } catch (error) {
        setExitError(error instanceof Error ? error.message : "Could not change the default experience");
        return;
      }
    }
    navigate(fullPath);
  };

  return (
    <div className="simple-app-shell">
      <header className="simple-header">
        <Link to="/simple" className="brand simple-brand" aria-label="Simple matching home">
          <ActivityIcon size={22} />
          <span>Runnarr</span>
        </Link>
        <div className="simple-account">
          <span>{session?.user?.displayName || session?.user?.username}</span>
          <button className="secondary-button small-button" type="button" disabled={loggingOut} onClick={onLogout}>
            <LogOut size={15} />
            {loggingOut ? "Logging out…" : "Log out"}
          </button>
        </div>
      </header>
      {session?.supportMode && (
        <div className="support-banner">
          <span>Read-only support view: {session.user?.displayName || session.user?.username}</span>
          <button className="secondary-button small-button" type="button" onClick={() => {
            void api.stopSupport().then(() => window.location.replace("/"));
          }}>Exit support view</button>
        </div>
      )}
      <div className="simple-mode-banner">
        <span><strong>Simple matching mode</strong> · Training-sheet matching only</span>
        <button className="secondary-button small-button" type="button" onClick={() => void exitSimpleMode()}>Exit simple mode</button>
      </div>
      {exitError && (
        <div className="simple-exit-error error" role="alert">
          {exitError}. <Link to={fullPath}>Open full Runnarr for now</Link>.
        </div>
      )}
      <main className="simple-main">
        <Routes>
          <Route path="/simple" element={<SimpleActivitiesPage />} />
          <Route path="/simple/activities/:id" element={<ActivityDetailPage simple canWrite={session?.canWrite !== false} />} />
          <Route path="*" element={<Navigate to="/simple" replace />} />
        </Routes>
      </main>
    </div>
  );
}

function SimpleActivitiesPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const matchState = normalizeSimpleMatchFilter(searchParams.get("matchState"));
  const config = useQuery({ queryKey: ["training-sheet-config"], queryFn: api.trainingSheetConfig });
  const google = useQuery({ queryKey: ["google-sheets-status"], queryFn: api.googleSheetsStatus });
  const activities = useInfiniteQuery({
    queryKey: ["simple-activities", matchState],
    initialPageParam: 0,
    queryFn: ({ pageParam }) => api.activities(undefined, {
      limit: ACTIVITY_LIST_PAGE_SIZE,
      offset: pageParam,
      view: "training-sheet-matching",
      matchState
    }),
    getNextPageParam: (lastPage) => lastPage.hasMore ? lastPage.nextOffset : undefined,
    refetchInterval: (query) => {
      const data = query.state.data as { pages?: Array<{ activities: Activity[] | null }> } | undefined;
      return data?.pages?.some((page) => page.activities?.some((activity) => activity.trainingSheetMatch?.state === "writing")) ? 1500 : false;
    }
  });
  const items = activities.data?.pages.flatMap((page) => page.activities ?? []) ?? [];
  const filters: Array<{ value: SimpleMatchFilter; label: string }> = [
    { value: "all", label: "All" },
    { value: "unmatched", label: "Unmatched" },
    { value: "matched", label: "Matched" },
    { value: "attention", label: "Needs attention" }
  ];
  const setMatchState = (value: SimpleMatchFilter) => {
    const next = new URLSearchParams(searchParams);
    if (value === "all") next.delete("matchState"); else next.set("matchState", value);
    setSearchParams(next, { replace: true });
  };

  return (
    <section className="simple-page" aria-labelledby="simple-activities-title">
      <div className="simple-page-heading">
        <div>
          <div className="eyebrow">Training sheet</div>
          <h1 id="simple-activities-title">Match completed runs</h1>
          <p className="muted">Choose a run, review the proposed sheet changes, and write them back.</p>
        </div>
        <div className="segmented-control simple-match-filters" aria-label="Matching status">
          {filters.map((filter) => (
            <button key={filter.value} type="button" className={matchState === filter.value ? "active" : ""} aria-pressed={matchState === filter.value} onClick={() => setMatchState(filter.value)}>
              {filter.label}
            </button>
          ))}
        </div>
      </div>
      {config.data && !config.data.enabled && (
        <div className="simple-readiness-note">Automatic training-sheet sync is disabled. Existing imported plans remain available. <Link to="/settings#training-sheet">Open full settings</Link>.</div>
      )}
      {google.data && !google.data.writeReady && (
        <div className="simple-readiness-note warning">Google Sheets write access is unavailable. You can inspect matches, but preview and apply require reconnecting in <Link to="/settings#training-sheet">full settings</Link>.</div>
      )}
      {(config.error || google.error) && <div className="error">Could not check training-sheet readiness.</div>}
      {activities.isLoading && <LoadingRow />}
      {activities.error && <div className="error">{activities.error instanceof Error ? activities.error.message : "Could not load runs"}</div>}
      {!activities.isLoading && !activities.error && items.length === 0 && <EmptyState title="No runs in this view" message="Try another matching-status filter." />}
      {items.length > 0 && (
        <div className="simple-activity-list">
          {items.map((activity) => {
            const query = matchState === "all" ? "" : `?matchState=${matchState}`;
            return (
              <Link className="simple-activity-row" to={`/simple/activities/${encodeURIComponent(activity.id)}${query}`} key={activity.id}>
                <div className="simple-activity-date">{formatDate(activity.startTime)}</div>
                <div className="simple-activity-name">
                  <strong>{activity.name}</strong>
                  <span>{activity.sportType}</span>
                </div>
                <div className="simple-activity-metrics">
                  {supportsRouteMetrics(activity.sportType) && <span>{formatDistance(activity.distanceM)}</span>}
                  <span>{formatDuration(activity.movingTimeS || activity.elapsedTimeS)}</span>
                </div>
                <span className={`simple-match-status simple-match-status--${activity.trainingSheetMatch?.state ?? "unmatched"}`}>{simpleMatchStatusLabel(activity)}</span>
                <ChevronRight size={17} aria-hidden="true" />
              </Link>
            );
          })}
        </div>
      )}
      {activities.hasNextPage && (
        <div className="pagination-actions">
          <button className="secondary-button" type="button" disabled={activities.isFetchingNextPage} onClick={() => void activities.fetchNextPage()}>
            <ChevronDown size={16} />
            {activities.isFetchingNextPage ? "Loading" : "Load more"}
          </button>
        </div>
      )}
    </section>
  );
}

function MobileNavigation({
  session,
  onLogout,
  loggingOut,
  pwa
}: {
  session?: Session;
  onLogout: () => void;
  loggingOut: boolean;
  pwa: { canInstall: boolean; install: () => Promise<void> };
}) {
  const location = useLocation();
  const [moreOpen, setMoreOpen] = useState(false);

  useEffect(() => {
    setMoreOpen(false);
  }, [location.pathname]);

  const currentLabel = location.pathname.startsWith("/activities/")
    ? "Activity"
    : location.pathname.startsWith("/activities")
      ? "Activities"
      : location.pathname.startsWith("/calendar/day/")
        ? "Day view"
          : location.pathname.startsWith("/calendar")
            ? "Calendar"
          : location.pathname.startsWith("/courses")
            ? "Courses"
          : location.pathname.startsWith("/workouts")
            ? "Workouts"
          : location.pathname.startsWith("/notifications")
            ? "Notifications"
          : location.pathname.startsWith("/health")
            ? "Health"
            : location.pathname.startsWith("/tools")
              ? "Tools"
              : location.pathname.startsWith("/gear")
                ? "Gear"
                : location.pathname.startsWith("/settings")
                  ? "Settings"
                  : "Dashboard";

  return (
    <>
      <header className="mobile-header">
        <Link to="/" className="mobile-header-brand" aria-label="Runnarr dashboard">
          <ActivityIcon size={21} />
          <span>Runnarr</span>
        </Link>
        <span className="mobile-header-title">{currentLabel}</span>
        <div className="mobile-header-actions">
          <NotificationBell mobile canWrite={session?.canWrite !== false} />
          <button className="icon-button mobile-menu-button" type="button" aria-label="Open navigation" onClick={() => setMoreOpen(true)}>
            <Menu size={19} />
          </button>
        </div>
      </header>
      <nav className="mobile-bottom-nav" aria-label="Primary navigation">
        <NavLink to="/" className={({ isActive }) => `mobile-nav-item ${isActive ? "active" : ""}`} end>
          <BarChart3 size={19} />
          <span>Home</span>
        </NavLink>
        <NavLink to="/activities" className={({ isActive }) => `mobile-nav-item ${isActive ? "active" : ""}`}>
          <MapIcon size={19} />
          <span>Activities</span>
        </NavLink>
        <NavLink to="/calendar" className={({ isActive }) => `mobile-nav-item ${isActive ? "active" : ""}`}>
          <CalendarDays size={19} />
          <span>Calendar</span>
        </NavLink>
        <NavLink to="/health" className={({ isActive }) => `mobile-nav-item ${isActive ? "active" : ""}`}>
          <HeartPulse size={19} />
          <span>Health</span>
        </NavLink>
        <button className={`mobile-nav-item ${moreOpen ? "active" : ""}`} type="button" onClick={() => setMoreOpen(true)}>
          <MoreHorizontal size={19} />
          <span>More</span>
        </button>
      </nav>
      {moreOpen && (
        <div className="mobile-menu-backdrop" role="presentation" onMouseDown={(event) => {
          if (event.target === event.currentTarget) {
            setMoreOpen(false);
          }
        }}>
          <section className="mobile-menu-sheet" role="dialog" aria-modal="true" aria-label="More navigation">
            <div className="mobile-menu-heading">
              <div>
                <div className="eyebrow">Navigation</div>
                <strong>{session?.user?.displayName || session?.user?.username || "Runnarr"}</strong>
              </div>
              <button className="icon-button" type="button" aria-label="Close navigation" onClick={() => setMoreOpen(false)}>
                <X size={18} />
              </button>
            </div>
            <div className="mobile-menu-links">
              {pwa.canInstall && (
                <button className="nav-button mobile-install-button" type="button" onClick={() => void pwa.install()}>
                  <Download size={18} />
                  <span>Install Runnarr</span>
                </button>
              )}
              <NavItem to="/tools" icon={<Calculator size={18} />} label="Tools" />
              <NavItem to="/courses" icon={<RouteIcon size={18} />} label="Courses" />
              <NavItem to="/workouts" icon={<Timer size={18} />} label="Workouts" />
              <NavItem to="/gear" icon={<Footprints size={18} />} label="Gear" />
              <NavItem to="/settings" icon={<SettingsIcon size={18} />} label="Settings" />
            </div>
            <button className="nav-button mobile-logout-button" type="button" disabled={loggingOut} onClick={onLogout}>
              <LogOut size={18} />
              <span>{loggingOut ? "Logging out…" : "Log out"}</span>
            </button>
          </section>
        </div>
      )}
    </>
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
  const next = safeNextPath(searchParams.get("next") || window.sessionStorage.getItem("runnarrLoginNext") || "/");
  const login = useMutation({
    mutationFn: ({ username, password }: { username: string; password: string }) => api.login(username, password),
    onSuccess: async (session) => {
      setCsrfToken(session.csrfToken);
      await queryClient.invalidateQueries({ queryKey: ["session"] });
      window.sessionStorage.removeItem("runnarrLoginNext");
      navigate(next);
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : "Login failed")
  });

  const localLoginEnabled = session.data?.localLoginEnabled !== false;
  const googleOIDCEnabled = session.data?.googleOIDCEnabled === true;
  const callbackError = searchParams.get("error");
  useEffect(() => {
    const next = searchParams.get("next");
    if (next) window.sessionStorage.setItem("runnarrLoginNext", safeNextPath(next));
  }, [searchParams]);

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
        {googleOIDCEnabled && <a className="secondary-button" href="/api/auth/google/login" onClick={() => window.sessionStorage.setItem("runnarrLoginNext", next)}>Continue with Google</a>}
        {!localLoginEnabled && !googleOIDCEnabled && <div className="error">No login method is configured.</div>}
      </form>
    </div>
  );
}

function Dashboard() {
  const [period, setPeriod] = useState<"weekly" | "monthly" | "yearly">("weekly");
  const summary = useQuery({ queryKey: ["summary", period], queryFn: () => api.summary(undefined, period) });

  if (summary.isLoading) {
    return <Page title="Dashboard"><LoadingRow /></Page>;
  }
  if (!summary.data) {
    return <Page title="Dashboard"><EmptyState title="No summary available" /></Page>;
  }

  const buckets = (summary.data.distanceBuckets ?? summary.data.weeklyDistance ?? []).map((item) => ({
    period: new Date("start" in item ? item.start : item.weekStart).toLocaleDateString(
      undefined,
      period === "yearly" ? { year: "numeric" } : { month: "short", day: "numeric" }
    ),
    km: Number((item.distanceM / 1000).toFixed(1))
  }));

  return (
    <Page title="Dashboard">
      <div className="segmented-control" aria-label="Dashboard time scale">
        {(["weekly", "monthly", "yearly"] as const).map((value) => (
          <button key={value} type="button" className={period === value ? "active" : ""} onClick={() => setPeriod(value)}>
            {value === "weekly" ? "Weeks" : value === "monthly" ? "Months" : "Years"}
          </button>
        ))}
      </div>
      <section className="metric-grid">
        <Metric label="Activities" value={summary.data.activityCount.toLocaleString()} icon={<ActivityIcon size={18} />} />
        <Metric label="Distance" value={formatDistance(summary.data.distanceM)} icon={<RouteIcon size={18} />} />
        <Metric label="Moving Time" value={formatDuration(summary.data.movingTimeS)} icon={<Timer size={18} />} />
        <Metric label="Elevation" value={`${Math.round(summary.data.elevationGainM).toLocaleString()} m`} icon={<Mountain size={18} />} />
      </section>

      <section className="split-layout">
        <div className="panel">
          <div className="panel-heading">{period === "weekly" ? "Weekly" : period === "monthly" ? "Monthly" : "Yearly"} distance</div>
          <div className="chart-area">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={buckets}>
                <CartesianGrid strokeDasharray="3 3" vertical={false} />
                <XAxis dataKey="period" />
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
  const [selectedDate, setSelectedDate] = useState("");
  const dayDetailRef = useRef<HTMLDivElement | null>(null);
  const today = localDateString();
  const health = useQuery({
    queryKey: ["health-daily", range],
    queryFn: () => api.healthDaily(range),
    refetchInterval: false
  });
  const todayHealth = useQuery({
    queryKey: ["health-daily", "today", today],
    queryFn: () => api.healthDaily({ from: today, to: today }),
    staleTime: Infinity,
    refetchInterval: false
  });
  const healthError = health.error ?? todayHealth.error;
  const metrics = health.data?.metrics ?? [];
  const todayMetric = latestHealthMetric(todayHealth.data?.metrics ?? []);
  const selectedMetric = metrics.find((metric) => metric.date === selectedDate);
  const chartData = (health.data?.chart ?? healthChartData(metrics)).map((point) => ({
    ...point,
    label: point.label ?? healthChartLabel(point.date)
  }));
  const showLongRangeHealthLines = healthRangeDayCount(range) > healthBarChartMaxDays;
  const cardItems = healthMetricCards(todayMetric);
  const activePreset = healthRangePresets().find((preset) => healthRangesMatch(draftRange, healthRangeForLastDays(preset.days)));
  const draftRangeChanged = !healthRangesMatch(draftRange, range);
  const draftRangeValid = healthRangeDayCount(draftRange) > 0;
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

  const healthRangeControls = (
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
    </section>
  );

  return (
    <Page title="Health">
      {healthError && <div className="error">{healthError instanceof Error ? healthError.message : "Could not load health metrics"}</div>}

      {cardItems.length > 0 && (
        <section className="health-summary" aria-label="Health summary">
          <div className="health-summary-header">
            <div className="panel-heading">Summary</div>
            <span className="muted">Data for {healthSummaryDateLabel(todayMetric)}</span>
          </div>
          <div className="metric-grid">
            {cardItems.map((item) => <Metric key={item.label} label={item.label} value={item.value} icon={item.icon} />)}
          </div>
          {healthRangeControls}
        </section>
      )}

      {cardItems.length === 0 && healthRangeControls}

      {(health.isLoading || todayHealth.isLoading) && <LoadingRow />}
      {!health.isLoading && metrics.length === 0 && (
        <EmptyState
          title="No health metrics found"
          action={<Link className="secondary-button" to="/settings">Open settings</Link>}
        />
      )}

      {metrics.length > 0 && (
        <>
          <section className="health-chart-grid">
            <HealthBarChart title="Steps" data={chartData} dataKey="steps" color="#2f8f83" formatter={formatHealthInteger} axisFormatter={formatHealthAxisInteger} asLine={showLongRangeHealthLines} />
            <HealthCaloriesChart data={chartData} asLine={showLongRangeHealthLines} />
            <HealthBarChart title="Sleep" data={chartData} dataKey="sleepHours" color="#4664c9" formatter={(value) => `${value.toFixed(1)} h`} axisFormatter={formatHealthAxisHours} asLine={showLongRangeHealthLines} />
            <HealthLineChart title="Sleep score" data={chartData} dataKey="sleepScore" color="#8b5e3c" formatter={(value) => Math.round(value).toLocaleString()} axisFormatter={formatHealthAxisInteger} />
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
    <>
    <div className="table-wrap health-table-desktop">
      <table className="data-table health-table">
        <thead>
          <tr>
            <th>Date</th>
            <th>Steps</th>
            <th>Calories</th>
            <th>Sleep</th>
            <th>Sleep score</th>
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
              <td>{formatHealthRounded(metric.sleepScore)}</td>
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
    <div className="health-card-list">
      {[...metrics].reverse().map((metric) => (
        <button
          className={`health-card ${selectedDate === metric.date ? "selected" : ""}`}
          type="button"
          key={metric.date}
          onClick={() => onSelect(metric.date)}
        >
          <span className="health-card-header">
            <strong>{formatHealthDate(metric.date)}</strong>
            <span>{selectedDate === metric.date ? "Selected" : "View details"}</span>
          </span>
          <span className="health-card-metrics">
            <span><strong>{formatHealthInteger(metric.steps) || "—"}</strong><small>Steps</small></span>
            <span><strong>{formatHealthDuration(metric.sleepDurationS) || "—"}</strong><small>Sleep</small></span>
            <span><strong>{formatHealthBPM(metric.restingHeartRateBpm) || "—"}</strong><small>RHR</small></span>
            <span><strong>{formatBodyBatteryGainDrain(metric) || "—"}</strong><small>Battery</small></span>
          </span>
        </button>
      ))}
    </div>
    </>
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
          <div className={`gear-progress gear-progress--${gearUsageTone(usagePercent)}`} aria-label={`Gear distance ${usagePercentLabel}`}>
            <span style={{ width: `${usagePercent}%` }} />
          </div>
          <div className="gear-progress-label">{formatGearDistance(total)} of {formatGearDistance(max)} · {usagePercentLabel}</div>
        </>
      )}
    </div>
  );
}

function GearChipList({ gear, compact = false, className }: { gear?: GearSummary[]; compact?: boolean; className?: string }) {
  const items = gear ?? [];
  if (items.length === 0) {
    return null;
  }
  return (
    <div className={`gear-chip-list${compact ? " compact" : ""}${className ? ` ${className}` : ""}`}>
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
  const anyFiltersActive = hasActivityFilters(filters);
  const activeFilterCount = activityFilterCount(filters);
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
            className={`secondary-button ${activeFilterCount > 0 ? "active-filter-button" : ""}`}
            type="button"
            onClick={() => setFiltersOpen(true)}
          >
            <Filter size={16} />
            Filter
            {activeFilterCount > 0 && <span className="button-badge">{activeFilterCount}</span>}
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
          activityTypes={activityTypes.data?.activityTypes ?? []}
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
      {activities.isLoading && <LoadingRow />}
      {activities.error && <div className="error">{activities.error instanceof Error ? activities.error.message : "Could not load activities"}</div>}
      {deleteActivity.error && <div className="error">{deleteActivity.error instanceof Error ? deleteActivity.error.message : "Delete failed"}</div>}
      {activitiesLoaded && activityList.length > 0 && (
        <>
          <ActivityTable
            activities={activityList}
            visibleColumns={visibleColumns}
            activityListSearch={searchParams.toString()}
            onDelete={handleDelete}
            deletingId={deleteActivity.variables}
          />
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
          title={anyFiltersActive ? "No activities match these filters" : "No activities yet"}
          message={anyFiltersActive ? "Try broadening your search, date range, or selected activity types." : undefined}
          action={anyFiltersActive ? undefined : <Link className="secondary-button" to="/settings#import">Import a file</Link>}
        />
      )}
    </Page>
  );
}

function ActivityCalendarPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const month = parseCalendarMonth(searchParams.get("month"));
  const timezone = browserCalendarTimezone();
  const monthRange = calendarMonthRange(month);
  const filters: ActivityTypeFiltersValue = {
    ...emptyActivityTypeFilters,
    dateFrom: monthRange.start,
    dateTo: monthRange.end
  };
  const calendar = useQuery({
    queryKey: ["activity-calendar", month.year, month.month, timezone],
    queryFn: () => api.activityCalendar(filters, timezone)
  });
  const monthLabel = formatCalendarMonthLabel(month);
  const dayByDate = new Map(calendar.data?.days?.map((day) => [day.date, day]) ?? []);
  const today = localDateString();
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
        <div className="calendar-top-bar">
          <div>
            <div className="panel-heading">Monthly activity calendar</div>
            <span className="muted">Open a day to inspect its health data and activities.</span>
          </div>
          <Link className="secondary-button small-button" to={`/calendar/day/${today}`}>
            <CalendarDays size={16} />
            View today
          </Link>
        </div>
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
            const hasDayView = Boolean(hasActivities || entry.dayData?.hasHealthData);
            return (
              <div
                className={`calendar-day-cell ${hasDayView ? "calendar-day-cell--active" : ""}`}
                key={entry.date}
              >
                {hasDayView ? (
                  <Link className="calendar-day-number calendar-day-link" to={`/calendar/day/${entry.date}`} aria-label={`View ${formatCalendarAgendaDate(entry.date)}`}>
                    {entry.day}
                  </Link>
                ) : (
                  <div className="calendar-day-number">{entry.day}</div>
                )}
                {hasActivities && (
                  <ul className="calendar-day-list">
                    {entry.dayData?.activities.map((activity) => (
                      <li key={activity.id} className={`calendar-day-activity${activity.source === "training_sheet" ? " calendar-day-activity--planned" : ""}`}>
                        <Link to={calendarActivityPath(activity)}>
                          {activity.name}
                        </Link>
                        <span className="calendar-day-activity-meta">
                          {activity.sportType}
                          {activity.sportType && activity.movingTimeS > 0 && ` · ${formatDuration(activity.movingTimeS)}`}
                        </span>
                        {activity.matchedPlan && (
                          <span className="calendar-day-match-meta">
                            {calendarPlanMatchDescription(activity.matchedPlan, entry.date, formatCalendarAgendaDate)}
                          </span>
                        )}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            );
          })}
        </div>
        <div className="mobile-calendar-agenda">
          {monthCells.filter((entry): entry is NonNullable<typeof entry> => entry !== null).map((entry) => (
            <div className="calendar-agenda-day" key={`agenda-${entry.date}`}>
              <div className="calendar-agenda-day-header">
                {(entry.dayData?.activityCount || entry.dayData?.hasHealthData) ? (
                  <Link className="calendar-day-link" to={`/calendar/day/${entry.date}`}>
                    <strong>{formatCalendarAgendaDate(entry.date)}</strong>
                  </Link>
                ) : (
                  <strong>{formatCalendarAgendaDate(entry.date)}</strong>
                )}
                <span>{entry.dayData?.activityCount ? `${entry.dayData.activityCount} activit${entry.dayData.activityCount === 1 ? "y" : "ies"}` : "No activities"}</span>
              </div>
              {entry.dayData && entry.dayData.activityCount > 0 ? (
                <ul className="calendar-day-list">
                  {entry.dayData.activities.map((activity) => (
                    <li key={activity.id} className={`calendar-day-activity${activity.source === "training_sheet" ? " calendar-day-activity--planned" : ""}`}>
                      <Link to={calendarActivityPath(activity)}>{activity.name}</Link>
                      <span className="calendar-day-activity-meta">
                        {activity.sportType}
                        {activity.sportType && activity.movingTimeS > 0 && ` · ${formatDuration(activity.movingTimeS)}`}
                      </span>
                      {activity.matchedPlan && (
                        <span className="calendar-day-match-meta">
                          {calendarPlanMatchDescription(activity.matchedPlan, entry.date, formatCalendarAgendaDate)}
                        </span>
                      )}
                    </li>
                  ))}
                </ul>
              ) : (
                <span className="muted">Rest day</span>
              )}
            </div>
          ))}
        </div>
      </section>
    </Page>
  );
}

function CalendarDayPage() {
  const { date: routeDate } = useParams();
  const date = isCalendarDate(routeDate) ? routeDate : "";
  const timezone = browserCalendarTimezone();
  const day = useQuery({
    queryKey: ["calendar-day", date, timezone],
    queryFn: () => api.calendarDay(date, timezone),
    enabled: Boolean(date)
  });
  const health = day.data?.health;
  const activities = day.data?.activities ?? [];
  const future = date > localDateString();

  return (
    <Page
      title="Day view"
      eyebrow={date ? formatCalendarDayLongDate(date) : undefined}
      actions={
        <div className="calendar-day-controls">
          <Link className="secondary-button small-button" to={date ? `/calendar?month=${date.slice(0, 7)}` : "/calendar"}>
            <ChevronLeft size={16} />
            Back to calendar
          </Link>
        </div>
      }
    >
      {!date && <div className="error">That day is not valid.</div>}
      {day.isLoading && <LoadingRow />}
      {day.error && <div className="error">{day.error instanceof Error ? day.error.message : "Could not load day view"}</div>}

      {date && !day.isLoading && !day.error && (
        <>
          {health ? (
            <>
              <section className="health-summary" aria-label="Daily health">
                <div className="health-summary-header">
                  <div className="panel-heading">Health</div>
                  <span className="muted">Daily Garmin metrics</span>
                </div>
                <div className="metric-grid">
                  {healthMetricCards(health).map((item) => <Metric key={item.label} label={item.label} value={item.value} icon={item.icon} />)}
                </div>
              </section>
              <HealthDayDetail metric={health} />
            </>
          ) : (
            <section className="panel calendar-day-empty-health">
              <div className="panel-heading">Health</div>
              <span className="muted">{future ? "Health data is not available for future days." : "No health data recorded for this day."}</span>
            </section>
          )}

          <section className="panel calendar-day-activities-panel">
            <div className="filter-header">
              <div className="panel-heading">Activities</div>
              <span className="muted">{activities.length.toLocaleString()}</span>
            </div>
            {activities.length > 0 ? (
              <CalendarDayActivityList activities={activities} date={date} />
            ) : (
              <div className="calendar-day-empty-activities muted">
                {future ? "No planned activities for this day." : "No activities recorded for this day."}
              </div>
            )}
          </section>
        </>
      )}
    </Page>
  );
}

function CalendarDayActivityList({ activities, date }: { activities: CalendarActivitySummary[]; date: string }) {
  return (
    <ul className="calendar-day-activity-list">
      {activities.map((activity) => {
        const metadata = [
          activity.sportType,
          supportsRouteMetrics(activity.sportType) && activity.distanceM > 0 ? formatDistance(activity.distanceM) : "",
          activity.movingTimeS > 0 ? formatDuration(activity.movingTimeS) : ""
        ].filter(Boolean).join(" · ");
        return (
          <li key={activity.id} className={`calendar-day-activity-row${activity.source === "training_sheet" ? " calendar-day-activity-row--planned" : ""}`}>
            <div>
              <Link to={calendarActivityPath(activity)}>{activity.name}</Link>
              {metadata && <span className="calendar-day-activity-meta">{metadata}</span>}
              {activity.matchedPlan && (
                <span className="calendar-day-match-meta">
                  {calendarPlanMatchDescription(activity.matchedPlan, date, formatCalendarAgendaDate)}
                </span>
              )}
            </div>
            {activity.source === "training_sheet" && <span className="calendar-day-planned-label">Planned</span>}
          </li>
        );
      })}
    </ul>
  );
}

function calendarActivityPath(activity: CalendarActivitySummary) {
  if (activity.workoutId && (activity.source === "training_sheet" || activity.source === "manual_workout")) {
    return `/workouts/${activity.workoutId}`;
  }
  return `/activities/${activity.id}`;
}

function ActivityFiltersDialog({
  activityTypes,
  filters,
  onApply,
  onClose
}: {
  activityTypes: string[];
  filters: ActivityTypeFiltersValue;
  onApply: (filters: ActivityTypeFiltersValue) => void;
  onClose: () => void;
}) {
  const [draftFilters, setDraftFilters] = useState<ActivityTypeFiltersValue>(filters);
  const presets = dateFilterPresets();
  const draftDates: ActivityDateRange = {
    dateFrom: draftFilters.dateFrom ?? "",
    dateTo: draftFilters.dateTo ?? ""
  };
  const activePreset = presets.find((preset) => dateRangesMatch(draftDates, preset.range));
  const dateRangeInvalid = Boolean(draftDates.dateFrom && draftDates.dateTo && draftDates.dateFrom > draftDates.dateTo);
  const selectedTypes = selectedActivityTypes(draftFilters, activityTypes);
  const selectedSet = new Set(selectedTypes);
  const allTypesSelected = selectedTypes.length === activityTypes.length;
  const noTypesSelected = selectedTypes.length === 0;
  const updateDates = (nextDates: ActivityDateRange) => {
    setDraftFilters({
      ...draftFilters,
      dateFrom: nextDates.dateFrom ?? "",
      dateTo: nextDates.dateTo ?? ""
    });
  };
  const toggleActivityType = (sport: string) => {
    const nextSelectedTypes = selectedSet.has(sport)
      ? selectedTypes.filter((item) => item !== sport)
      : [...selectedTypes, sport];
    setDraftFilters(activityTypeFiltersForSelection(draftFilters, activityTypes, nextSelectedTypes));
  };
  const clearFilters = () => {
    setDraftFilters({
      ...draftFilters,
      search: "",
      sports: [],
      excludeSports: [],
      dateFrom: "",
      dateTo: ""
    });
  };
  const applyFilters = () => {
    if (dateRangeInvalid) {
      return;
    }
    onApply(draftFilters);
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
      <section className="filter-dialog activity-filters-dialog" role="dialog" aria-modal="true" aria-labelledby="activity-filters-title">
        <div className="dialog-header">
          <div>
            <div className="eyebrow">Filters</div>
            <h2 id="activity-filters-title">Activities</h2>
          </div>
          <button className="icon-button" type="button" aria-label="Close filters" onClick={onClose}>
            <X size={16} />
          </button>
        </div>

        <div className="filter-dialog-section">
          <div className="filter-label">Search</div>
          <label className="field">
            <span>Search by name</span>
            <input
              type="search"
              placeholder="Activity name"
              value={draftFilters.search ?? ""}
              onChange={(event) => setDraftFilters({ ...draftFilters, search: event.target.value })}
            />
          </label>
        </div>

        {activityTypes.length > 0 && (
          <div className="filter-dialog-section">
            <div className="filter-label">Activity types</div>
            <div className="activity-type-filter-menu">
              <div className="activity-type-filter-group" role="group" aria-labelledby="activity-filter-types-label">
                <div className="activity-type-filter-group-header">
                  <div id="activity-filter-types-label" className="filter-label">Select types</div>
                  <span>
                    <button type="button" disabled={allTypesSelected} onClick={() => setDraftFilters(activityTypeFiltersForSelection(draftFilters, activityTypes, activityTypes))}>Select all</button>
                    <button type="button" disabled={noTypesSelected} onClick={() => setDraftFilters(activityTypeFiltersForSelection(draftFilters, activityTypes, []))}>Clear all</button>
                  </span>
                </div>
                <div className="activity-type-options">
                  {activityTypes.map((sport) => (
                    <label key={`dialog-${sport}`} className="activity-type-option">
                      <input type="checkbox" checked={selectedSet.has(sport)} onChange={() => toggleActivityType(sport)} />
                      <span>{sport}</span>
                    </label>
                  ))}
                </div>
              </div>
            </div>
          </div>
        )}

        <div className="filter-dialog-section">
          <div className="filter-label">Date</div>
          <div className="date-preset-grid">
            {presets.map((preset) => (
              <button
                key={preset.id}
                className={`filter-chip ${activePreset?.id === preset.id ? "active" : ""}`}
                type="button"
                onClick={() => updateDates(preset.range)}
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
                onChange={(event) => updateDates({ ...draftDates, dateFrom: event.target.value })}
              />
            </label>
            <label className="field">
              <span>To</span>
              <input
                type="date"
                value={draftDates.dateTo ?? ""}
                min={draftDates.dateFrom || undefined}
                onChange={(event) => updateDates({ ...draftDates, dateTo: event.target.value })}
              />
            </label>
          </div>
          {dateRangeInvalid && <div className="row-error">End date must be after start date.</div>}
        </div>

        <div className="dialog-actions">
          <button className="secondary-button" type="button" onClick={clearFilters}>
            Clear filters
          </button>
          <button className="secondary-button" type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="primary-button" type="button" disabled={dateRangeInvalid} onClick={applyFilters}>
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
  activityListSearch,
  onDelete,
  deletingId
}: {
  activities: Activity[];
  compact?: boolean;
  visibleColumns?: ActivityTableColumnKey[];
  activityListSearch?: string;
  onDelete?: (activity: Activity) => void;
  deletingId?: string;
}) {
  if (activities.length === 0) {
    return <EmptyState title="No activities found" />;
  }
  const columns = compact ? compactActivityTableColumns : (visibleColumns ?? defaultActivityTableColumns);
  const showColumn = (column: ActivityTableColumnKey) => columns.includes(column);
  return (
    <>
    <div className="table-wrap activity-table-desktop">
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
              <td className="activity-name-cell"><Link to={activityDetailPath(activity.id, activityListSearch)} title={activity.name}>{activity.name}</Link></td>
              {showColumn("type") && <td className="clip-cell" title={activity.sportType}>{activity.sportType}</td>}
              {showColumn("gear") && <td className="gear-table-cell"><GearChipList gear={activity.gear} compact /></td>}
              {showColumn("distance") && <td>{supportsRouteMetrics(activity.sportType) ? formatDistance(activity.distanceM) : ""}</td>}
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
    <ActivityCardList activities={activities} compact={compact} activityListSearch={activityListSearch} onDelete={onDelete} deletingId={deletingId} />
    </>
  );
}

function ActivityCardList({
  activities,
  compact = false,
  activityListSearch,
  onDelete,
  deletingId
}: {
  activities: Activity[];
  compact?: boolean;
  activityListSearch?: string;
  onDelete?: (activity: Activity) => void;
  deletingId?: string;
}) {
  return (
    <div className={`activity-card-list${compact ? " compact" : ""}`}>
      {activities.map((activity) => {
        const showRouteMetrics = supportsRouteMetrics(activity.sportType);
        return <article className="activity-card" key={activity.id}>
          <div className="activity-card-header">
            <div className="activity-card-title">
              <Link to={activityDetailPath(activity.id, activityListSearch)} title={activity.name}>{activity.name}</Link>
              <span>{formatDate(activity.startTime)} · {activity.sportType}</span>
            </div>
            {onDelete && (
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
            )}
          </div>
          <div className={`activity-card-metrics${showRouteMetrics ? "" : " activity-card-metrics--without-distance"}`}>
            {showRouteMetrics && <span><strong>{formatDistance(activity.distanceM)}</strong><small>Distance</small></span>}
            <span><strong>{formatDuration(activity.movingTimeS || activity.elapsedTimeS)}</strong><small>Time</small></span>
            {!compact && <span><strong>{formatCalories(activity.caloriesKcal) || "—"}</strong><small>Calories</small></span>}
          </div>
          <div className="activity-card-footer">
            <GearChipList gear={activity.gear} compact />
            <span className="source-pill">{activity.source}</span>
          </div>
        </article>;
      })}
    </div>
  );
}

function activityDetailPath(id: string, activityListSearch?: string) {
  const query = activityListSearch?.replace(/^\?/, "");
  return `/activities/${encodeURIComponent(id)}${query ? `?${query}` : ""}`;
}

function ActivityNavigation({
  previousId,
  nextId,
  loading,
  onNavigate
}: ActivityNavigationData & { loading: boolean; onNavigate: (id: string) => void }) {
  return (
    <div className="activity-navigation" role="group" aria-label="Activity navigation" aria-busy={loading}>
      <button
        className="icon-button activity-navigation-button"
        type="button"
        title="Previous activity"
        aria-label="Previous activity"
        disabled={loading || !previousId}
        onClick={() => {
          if (previousId) {
            onNavigate(previousId);
          }
        }}
      >
        <ChevronLeft size={18} />
      </button>
      <button
        className="icon-button activity-navigation-button"
        type="button"
        title="Next activity"
        aria-label="Next activity"
        disabled={loading || !nextId}
        onClick={() => {
          if (nextId) {
            onNavigate(nextId);
          }
        }}
      >
        <ChevronRight size={18} />
      </button>
    </div>
  );
}

function ActivityDetailPage({ config, simple = false, canWrite = true }: { config?: AppConfig; simple?: boolean; canWrite?: boolean }) {
  const { id } = useParams();
  const location = useLocation();
  const activityIdRef = useRef(id);
  const reflectionPromptLocationRef = useRef<string | undefined>(undefined);
  activityIdRef.current = id;
  const activityViewRef = useRef({ id, generation: 0 });
  if (activityViewRef.current.id !== id) {
    activityViewRef.current = { id, generation: activityViewRef.current.generation + 1 };
  }
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const activityQueryKey = ["activity", id] as const;
  const [searchParams] = useSearchParams();
  const activityListSearch = searchParams.toString();
  const activityFilters = activityFiltersFromSearchParams(searchParams);
  const [plannedMatchWindowDays, setPlannedMatchWindowDays] = useState(7);
  const [retryingPlannedMatchCandidates, setRetryingPlannedMatchCandidates] = useState(false);
  const plannedMatchInteractionGenerationRef = useRef(0);
  const plannedMatchRetryGenerationRef = useRef(0);
  const invalidatePlannedMatchInteraction = () => {
    plannedMatchInteractionGenerationRef.current += 1;
    return plannedMatchInteractionGenerationRef.current;
  };
  const invalidatePlannedMatchRetry = () => {
    plannedMatchRetryGenerationRef.current += 1;
    return plannedMatchRetryGenerationRef.current;
  };
  const activity = useQuery({ queryKey: activityQueryKey, queryFn: () => api.activity(id!), enabled: Boolean(id) });
  const activityNavigation = useQuery({
    queryKey: ["activity-navigation", id, activityListSearch],
    queryFn: () => api.activityNavigation(id!, activityFilters),
    enabled: !simple && Boolean(id) && activity.data?.activity.source !== "training_sheet"
  });
  const activitySeries = useQuery({
    queryKey: ["activity-series", id],
    queryFn: () => api.activitySeries(id!, 1200),
    enabled: !simple && Boolean(id) && activity.data?.activity.source !== "training_sheet"
  });
  const plannedMatchCandidates = useQuery({
    queryKey: ["planned-match-candidates", id, plannedMatchWindowDays],
    queryFn: () => api.plannedMatchCandidates(id!, plannedMatchWindowDays),
    enabled: Boolean(id) && activity.data?.activity.source !== "training_sheet",
    initialData: plannedMatchWindowDays === 30
      ? () => queryClient.getQueryData<PlannedActivityMatchResponse>(["planned-match-candidates", id, 7])
      : undefined,
    refetchInterval: (query) => {
      const writeback = query.state.data?.writeback;
      if (!writeback) {
        return false;
      }
      return writeback.jobStatus === "running" || writeback.summaryStatus === "running" || writeback.intervalsStatus === "running" || writeback.feedbackStatus === "running" ? 1500 : false;
    }
  });
  const simpleTrainingSheetConfig = useQuery({ queryKey: ["training-sheet-config"], queryFn: api.trainingSheetConfig, enabled: simple });
  const simpleGoogleStatus = useQuery({ queryKey: ["google-sheets-status"], queryFn: api.googleSheetsStatus, enabled: simple });
  const previewPlannedActivity = useMutation({
    mutationFn: ({ activityId, draft }: { activityId: string; activityViewGeneration: number; requestGeneration: number; draft: PlannedMatchDraft }) =>
      api.plannedMatchPreview(activityId, draft),
    onSuccess: ({ preview }, { activityId, activityViewGeneration, requestGeneration }) => {
      if (!plannedMatchRequestIsCurrent(
        activityId,
        activityViewGeneration,
        activityIdRef.current,
        activityViewRef.current.generation,
        requestGeneration,
        plannedMatchInteractionGenerationRef.current
      )) {
        return;
      }
      const currentPreview = plannedMatchPreviewForActivity(preview, activityIdRef.current);
      if (currentPreview) {
        setMatchPreview(currentPreview);
      }
    }
  });
  const applyPlannedActivity = useMutation({
    mutationFn: ({ activityId, draft }: { activityId: string; activityViewGeneration: number; requestGeneration: number; draft: PlannedMatchDraft & { fingerprint: string } }) =>
      api.applyPlannedMatchPreview(activityId, draft),
    onSuccess: async (_result, { activityId, activityViewGeneration, requestGeneration }) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["planned-match-candidates", activityId] }),
        queryClient.invalidateQueries({ queryKey: ["planned-activities"] }),
        queryClient.invalidateQueries({ queryKey: ["activity-calendar"] }),
        queryClient.invalidateQueries({ queryKey: ["activity", activityId] }),
        queryClient.invalidateQueries({ queryKey: ["simple-activities"] })
      ]);
      if (!plannedMatchRequestIsCurrent(
        activityId,
        activityViewGeneration,
        activityIdRef.current,
        activityViewRef.current.generation,
        requestGeneration,
        plannedMatchInteractionGenerationRef.current
      )) {
        return;
      }
      setMatchPreview(undefined);
      setMatchOpen(false);
    },
    onError: (_error, { activityId, activityViewGeneration, requestGeneration }) => {
      if (plannedMatchRequestIsCurrent(
        activityId,
        activityViewGeneration,
        activityIdRef.current,
        activityViewRef.current.generation,
        requestGeneration,
        plannedMatchInteractionGenerationRef.current
      )) {
        setMatchPreview(undefined);
      }
    }
  });
  const unmatchPlannedActivity = useMutation({
    mutationFn: () => api.unmatchPlannedActivity(id!),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["planned-match-candidates", id] }),
        queryClient.invalidateQueries({ queryKey: ["planned-activities"] }),
        queryClient.invalidateQueries({ queryKey: ["activity-calendar"] }),
        queryClient.invalidateQueries({ queryKey: ["simple-activities"] })
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
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["planned-match-candidates", id] }),
        queryClient.invalidateQueries({ queryKey: ["simple-activities"] })
      ]);
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
  const updateMediaLocation = useMutation({
    mutationFn: ({ mediaId, latitude, longitude }: { mediaId: string; latitude: number; longitude: number }) =>
      api.updateActivityMediaLocation(id!, mediaId, latitude, longitude),
    onSuccess: ({ media }) => {
      queryClient.setQueryData<{ activity: Activity }>(activityQueryKey, (current) => {
        if (!current) {
          return current;
        }
        return {
          activity: {
            ...current.activity,
            media: (current.activity.media ?? []).map((item) => item.id === media.id ? media : item)
          }
        };
      });
      setPinningMediaId(undefined);
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
  const [courseSaveOpen, setCourseSaveOpen] = useState(false);
  const [mediaFileInputKey, setMediaFileInputKey] = useState(0);
  const [selectedMediaId, setSelectedMediaId] = useState<string>();
  const [pinningMediaId, setPinningMediaId] = useState<string>();
  const [analysisTab, setAnalysisTab] = useState<ActivityAnalysisTab>("stats");
  const [climbSensitivityDraft, setClimbSensitivityDraft] = useState(defaultClimbSensitivity);
  const [climbSensitivityPreview, setClimbSensitivityPreview] = useState(defaultClimbSensitivity);
  const saveAsCourse = useMutation({
    mutationFn: (input: { name: string; sportType: CourseSport; notes: string }) => api.saveActivityAsCourse(id!, input),
    onSuccess: (created) => {
      void queryClient.invalidateQueries({ queryKey: ["courses"] });
      navigate(`/courses/${encodeURIComponent(created.id)}`);
    }
  });
  const routeUsesGap = (activity.data?.activity.laps ?? []).some((lap) => lap.avgGradeAdjustedPaceSPKM !== undefined);
  const configuredClimbSensitivity = config?.climbDetection?.sensitivity ?? defaultClimbSensitivity;
  const climbSensitivity = clampClimbSensitivity(climbSensitivityDraft);
  const climbSensitivityForPreview = clampClimbSensitivity(climbSensitivityPreview);
  const isPreviewPending = climbSensitivity !== climbSensitivityForPreview;
  const climbPreview = useQuery({
    queryKey: ["activity-climb-preview", id, climbSensitivityForPreview],
    queryFn: () => api.activityClimbPreview(id!, climbSensitivityForPreview),
    placeholderData: (previousData) => previousData,
    enabled: !simple && Boolean(activity.data) && supportsClimbAnalysis(activity.data?.activity.sportType ?? "")
  });
  useEffect(() => {
    if (!routeUsesGap) {
      setRouteColorSource("pace");
    }
  }, [id, routeUsesGap]);

  useEffect(() => {
    invalidatePlannedMatchInteraction();
    invalidatePlannedMatchRetry();
    setHighlightedSample(undefined);
    setSelectedClimbIndex(undefined);
    setActionsOpen(false);
    setRenameOpen(false);
    setNotesOpen(false);
    setMatchOpen(false);
    setMatchCandidateId(undefined);
    setMatchPreview(undefined);
    setCheckInOpen(false);
    setPlannedMatchWindowDays(7);
    setRetryingPlannedMatchCandidates(false);
    setExportOpen(false);
    setCourseSaveOpen(false);
    setSelectedMediaId(undefined);
    setPinningMediaId(undefined);
    setAnalysisTab("stats");
    updateActivityNotes.reset();
    previewPlannedActivity.reset();
    applyPlannedActivity.reset();
    uploadMedia.reset();
    saveAsCourse.reset();
    updateMediaLocation.reset();
    setMediaFileInputKey((key) => key + 1);
    setClimbSensitivityDraft(configuredClimbSensitivity);
    setClimbSensitivityPreview(configuredClimbSensitivity);
  }, [id, configuredClimbSensitivity]);

  useEffect(() => {
    if ((!matchOpen && !simple) || matchCandidateId || !plannedMatchCandidates.data?.suggestedId) {
      return;
    }
    setMatchCandidateId(plannedMatchCandidates.data.suggestedId);
  }, [simple, matchOpen, matchCandidateId, plannedMatchCandidates.data?.suggestedId]);

  useEffect(() => {
    const nextValue = clampClimbSensitivity(climbSensitivity);
    const timeout = window.setTimeout(() => setClimbSensitivityPreview(nextValue), 120);
    return () => window.clearTimeout(timeout);
  }, [climbSensitivity]);

  const item = activity.data?.activity;
  const writeback = plannedMatchCandidates.data?.writeback;
  const matchedPlannedActivity = plannedMatchCandidates.data?.matched;
  const intervalAnalysisAvailable = hasIntervalAnalysis(item);
  const visibleAnalysisTab = resolveActivityAnalysisTab(analysisTab, intervalAnalysisAvailable);
  const effectiveClimbs = item ? (climbPreview.data?.climbs ?? item.climbs ?? []) : [];

  useEffect(() => {
    if (analysisTab !== visibleAnalysisTab) {
      setAnalysisTab(visibleAnalysisTab);
    }
  }, [analysisTab, visibleAnalysisTab]);

  useEffect(() => {
    if (selectedClimbIndex === undefined) {
      return;
    }
    if (!effectiveClimbs.some((climb) => climb.index === selectedClimbIndex)) {
      setSelectedClimbIndex(undefined);
    }
  }, [effectiveClimbs, selectedClimbIndex]);

  useEffect(() => {
    if (searchParams.get("section") !== "writeback") return;
    const timeout = window.setTimeout(() => document.getElementById("writeback")?.scrollIntoView({ block: "center" }));
    return () => window.clearTimeout(timeout);
  }, [searchParams, writeback]);

  useEffect(() => {
    if (!canWrite || location.hash !== "#check-in" || !item || !matchedPlannedActivity || reflectionPromptLocationRef.current === location.key) return;
    reflectionPromptLocationRef.current = location.key;
    setCheckInOpen(true);
  }, [canWrite, item, location.hash, location.key, matchedPlannedActivity]);

  if (activity.isLoading) {
    return simple ? <section className="simple-page"><LoadingRow /></section> : <Page title="Activity"><LoadingRow /></Page>;
  }
  if (!activity.data || !item) {
    return simple ? <section className="simple-page"><EmptyState title="Activity not found" /></section> : <Page title="Activity"><EmptyState title="Activity not found" /></Page>;
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
  const displayItem = { ...confirmedItem, samples: activitySeries.data?.samples ?? [] };
  const showClimbAnalysis = supportsClimbAnalysis(displayItem.sportType);
  const mediaItems = item.media ?? [];
  const locatedMedia = mediaItems.filter(hasMediaLocation);
  const pinningMedia = mediaItems.find((media) => media.id === pinningMediaId);
  const routePoints = routeForActivity(displayItem);
  const canExportGPX = (activitySeries.data?.totalSamples ?? 0) > 1;
  const paceScale = paceScaleForActivity(displayItem, "pace");
  const routePaceScale = paceScaleForActivity(displayItem, routeColorSource);
  const paceRouteSegments = paceRouteSegmentsForActivity(displayItem, routePaceScale, routeColorSource);
  const chartData: ActivityChartPoint[] = activitySeries.data?.points ?? chartDataFor(displayItem.samples ?? [], paceScale);
  const highlightedPoint = routePointForChartPoint(highlightedSample);
  const finalClimbs = showClimbAnalysis ? effectiveClimbs : [];
  const selectedClimb = selectedClimbIndex === undefined ? undefined : finalClimbs.find((climb) => climb.index === selectedClimbIndex);
  const climbMapSegments = climbMapSegmentsFor(displayItem, finalClimbs);
  const selectedClimbProfile = climbProfileFor(displayItem, selectedClimb);
  const climbPerformanceSamples = samplesForClimbPerformance(confirmedItem.samples, activitySeries.data?.samples);
  const climbPerformanceByIndex = Object.fromEntries(
    finalClimbs.map((climb) => {
      const fallback: ClimbPerformance = climb.paceSPKM === undefined || climb.gapSPKM === undefined
        ? climbPerformanceFor(climbPerformanceSamples, confirmedItem.laps ?? [], climb)
        : {};
      return [climb.index, {
        paceSPKM: climb.paceSPKM ?? fallback.paceSPKM,
        gapSPKM: climb.gapSPKM ?? fallback.gapSPKM
      }];
    })
  ) as Record<number, ClimbPerformance>;
  const selectedClimbPerformance = selectedClimb ? climbPerformanceByIndex[selectedClimb.index] : undefined;
  const isClimbSensitivitySaved = climbSensitivity === configuredClimbSensitivity;
  const activeClimbPreset = climbSensitivityPresetForValue(climbSensitivity);
  const activeClimbPresetLabel = climbSensitivityPresetLabel(climbSensitivity);
  const canSaveClimbSensitivity = !isClimbSensitivitySaved;
  const matchedTrainingSheetURL = trainingSheetSourceURL(matchedPlannedActivity?.sourceUrl);
  const feedbackAvailable = Boolean(matchedPlannedActivity?.feedbackCell?.trim());
  const loadingMorePlans = plannedMatchWindowDays === 30 && plannedMatchCandidates.isFetching;
  const loadingCandidateRetry = retryingPlannedMatchCandidates;
  const loadingCandidateRequest = loadingMorePlans || loadingCandidateRetry;
  const canLoadMorePlans = loadingCandidateRetry || (plannedMatchWindowDays === 7 && Boolean(plannedMatchCandidates.data?.hasMore)) || plannedMatchCandidates.isError;
  const loadMorePlansLabel = plannedMatchCandidates.isError || loadingCandidateRetry
    ? (loadingCandidateRetry ? "Retrying plans…" : "Retry loading plans")
    : "Load more plans";
  const candidateLoadingStatus = loadingCandidateRetry ? "Retrying planned runs…" : "Loading more plans…";
  const canRetryWriteback = Boolean(writeback && [
    writeback.summaryStatus,
    writeback.intervalsStatus,
    writeback.feedbackStatus
  ].some((status) => status === "failed" || status === "canceled" || status === "completed_with_conflicts"));

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
    invalidatePlannedMatchInteraction();
    invalidatePlannedMatchRetry();
    setRetryingPlannedMatchCandidates(false);
    const nextCandidateId = candidateId ?? plannedMatchCandidates.data?.suggestedId ?? plannedMatchCandidates.data?.candidates[0]?.id;
    previewPlannedActivity.reset();
    applyPlannedActivity.reset();
    setMatchPreview(undefined);
    setMatchCandidateId(nextCandidateId);
    setMatchOpen(true);
  };
  const handlePreviewMatch = (draft: PlannedMatchDraft) => {
    const requestGeneration = invalidatePlannedMatchInteraction();
    applyPlannedActivity.reset();
    previewPlannedActivity.mutate({
      activityId: id!,
      activityViewGeneration: activityViewRef.current.generation,
      requestGeneration,
      draft
    });
  };
  const resetMatchPreview = () => {
    invalidatePlannedMatchInteraction();
    setMatchPreview(undefined);
    previewPlannedActivity.reset();
    applyPlannedActivity.reset();
  };
  const handleApplyMatch = (draft: PlannedMatchDraft) => {
    if (!matchPreview) {
      return;
    }
    applyPlannedActivity.mutate({
      activityId: id!,
      activityViewGeneration: activityViewRef.current.generation,
      requestGeneration: plannedMatchInteractionGenerationRef.current,
      draft: { ...draft, fingerprint: matchPreview.fingerprint }
    });
  };
  const closeMatchDialog = () => {
    invalidatePlannedMatchInteraction();
    invalidatePlannedMatchRetry();
    setRetryingPlannedMatchCandidates(false);
    setMatchPreview(undefined);
    setMatchOpen(false);
  };
  const handleLoadMorePlans = () => {
    if (plannedMatchCandidates.isError) {
      const retryActivityId = id!;
      const retryActivityViewGeneration = activityViewRef.current.generation;
      const retryGeneration = invalidatePlannedMatchRetry();
      setRetryingPlannedMatchCandidates(true);
      void plannedMatchCandidates.refetch().finally(() => {
        if (plannedMatchRequestIsCurrent(
          retryActivityId,
          retryActivityViewGeneration,
          activityIdRef.current,
          activityViewRef.current.generation,
          retryGeneration,
          plannedMatchRetryGenerationRef.current
        )) {
          setRetryingPlannedMatchCandidates(false);
        }
      });
      return;
    }
    setPlannedMatchWindowDays(30);
  };

  if (simple) {
    const simpleFilter = normalizeSimpleMatchFilter(searchParams.get("matchState"));
    const simpleBackPath = simpleFilter === "all" ? "/simple" : `/simple?matchState=${simpleFilter}`;
    const matchable = confirmedItem.sportType === "Run" || confirmedItem.sportType === "Treadmill Run";
    const writeReady = simpleGoogleStatus.data?.writeReady === true;
    return (
      <section className="simple-page simple-activity-detail" aria-labelledby="simple-activity-title">
        <Link className="simple-back-link" to={simpleBackPath}><ChevronLeft size={16} />Back to runs</Link>
        <div className="simple-activity-heading">
          <div>
            <div className="eyebrow">{confirmedItem.sportType} · {formatDate(confirmedItem.startTime)}</div>
            <h1 id="simple-activity-title">{confirmedItem.name}</h1>
          </div>
          {matchedPlannedActivity && <span className="simple-match-status simple-match-status--complete">Matched</span>}
        </div>
        <section className="simple-run-facts" aria-label="Run summary">
          {supportsRouteMetrics(confirmedItem.sportType) && <div><span>Distance</span><strong>{formatDistance(confirmedItem.distanceM)}</strong></div>}
          <div><span>Duration</span><strong>{formatDuration(confirmedItem.movingTimeS || confirmedItem.elapsedTimeS)}</strong></div>
          <div><span>Structure</span><strong>{simpleIntervalSummary(confirmedItem)}</strong></div>
        </section>
        {simpleTrainingSheetConfig.data && !simpleTrainingSheetConfig.data.enabled && (
          <div className="simple-readiness-note">Automatic training-sheet sync is disabled. Existing imported plans remain available. <Link to="/settings#training-sheet">Open full settings</Link>.</div>
        )}
        {simpleGoogleStatus.isLoading && <div className="muted">Checking Google Sheets write access…</div>}
        {simpleGoogleStatus.data && !writeReady && (
          <div className="simple-readiness-note warning">Google Sheets write access is unavailable. Reconnect in <Link to="/settings#training-sheet">full settings</Link> before previewing or applying a match.</div>
        )}
        {!matchable && <EmptyState title="This activity cannot be matched" message="Training-sheet matching supports runs and treadmill runs." />}
        {matchable && matchedPlannedActivity && (
          <section className="panel simple-matched-panel">
            <div className="simple-matched-heading">
              <div>
                <div className="panel-heading">Matched planned run</div>
                <strong>{matchedPlannedActivity.name}</strong>
                <span className="muted">{formatDate(matchedPlannedActivity.plannedDate)}</span>
              </div>
              <div className="simple-matched-actions">
                {matchedTrainingSheetURL && (
                  <a className="secondary-button small-button" href={matchedTrainingSheetURL} target="_blank" rel="noreferrer">
                    <ExternalLink size={14} />
                    Training sheet
                  </a>
                )}
                <button className="danger-button small-button" type="button" disabled={!canWrite || unmatchPlannedActivity.isPending} onClick={() => {
                  if (window.confirm("Unmatch this run? Values already written to Google Sheets will not be reverted.")) {
                    unmatchPlannedActivity.mutate();
                  }
                }}>{unmatchPlannedActivity.isPending ? "Unmatching…" : "Unmatch"}</button>
              </div>
            </div>
            {writeback ? (
              <div className="activity-writeback-statuses">
                <span>Summary <strong>{trainingSheetWritebackStatusLabel(writeback.summaryStatus)}</strong></span>
                <span>Intervals <strong>{trainingSheetWritebackStatusLabel(writeback.intervalsStatus)}</strong></span>
                <span>Reflection <strong>{trainingSheetWritebackStatusLabel(writeback.feedbackStatus)}</strong></span>
              </div>
            ) : <p className="muted">Writeback has not started.</p>}
            {writeback?.summaryError && <div className="row-error">Summary: {writeback.summaryError}</div>}
            {writeback?.intervalsError && <div className="row-error">Intervals: {writeback.intervalsError}</div>}
            {writeback?.feedbackError && <div className="row-error">Reflection: {writeback.feedbackError}</div>}
            {canRetryWriteback && <button className="secondary-button small-button" type="button" disabled={!canWrite || !writeReady || retryWriteback.isPending} onClick={() => retryWriteback.mutate()}><RefreshCw size={14} />{retryWriteback.isPending ? "Retrying…" : "Retry writeback"}</button>}
          </section>
        )}
        {matchable && !matchedPlannedActivity && (
          <PlannedActivityMatchDialog
            inline
            canApply={canWrite && writeReady}
            data={plannedMatchResponseForDialog(plannedMatchCandidates.data)}
            targetDate={localDateString(new Date(confirmedItem.startTime))}
            selectedCandidateId={matchCandidateId}
            canLoadMore={canLoadMorePlans}
            loadMoreLabel={loadMorePlansLabel}
            loadingStatus={candidateLoadingStatus}
            loadingMore={loadingCandidateRequest}
            matching={previewPlannedActivity.isPending || applyPlannedActivity.isPending}
            error={plannedMatchCandidates.error ?? previewPlannedActivity.error ?? applyPlannedActivity.error}
            preview={matchPreview}
            onSelectCandidate={setMatchCandidateId}
            onPreview={handlePreviewMatch}
            onApply={handleApplyMatch}
            onPreviewReset={resetMatchPreview}
            onLoadMore={handleLoadMorePlans}
            onClose={closeMatchDialog}
          />
        )}
        {unmatchPlannedActivity.error && <div className="error">{unmatchPlannedActivity.error instanceof Error ? unmatchPlannedActivity.error.message : "Could not unmatch planned run"}</div>}
        {retryWriteback.error && <div className="error">{retryWriteback.error instanceof Error ? retryWriteback.error.message : "Could not retry sheet write-back"}</div>}
      </section>
    );
  }
  const handleMediaFilesSelected = (files: File[]) => {
    if (files.length === 0 || uploadMedia.isPending) {
      return;
    }
    uploadMedia.reset();
    uploadMedia.mutate(files);
  };
  const handleStartMediaPinning = (mediaId: string) => {
    updateMediaLocation.reset();
    setSelectedMediaId(undefined);
    setPinningMediaId(mediaId);
  };
  const handleCancelMediaPinning = () => {
    updateMediaLocation.reset();
    setPinningMediaId(undefined);
  };
  const handleMapLocationSelect = (location: RoutePoint) => {
    if (!pinningMediaId || updateMediaLocation.isPending) {
      return;
    }
    updateMediaLocation.mutate({ mediaId: pinningMediaId, latitude: location[0], longitude: location[1] });
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
      titleAccessory={<GearChipList gear={item.gear} className="activity-title-gear" />}
      eyebrow={`${confirmedItem.sportType} · ${formatDate(confirmedItem.startTime)}`}
      actions={
        <>
          <ActivityNavigation
            previousId={activityNavigation.data?.previousId}
            nextId={activityNavigation.data?.nextId}
            loading={activityNavigation.isFetching}
            onNavigate={(nextID) => navigate(activityDetailPath(nextID, activityListSearch))}
          />
          <ActivityMediaUploadAction
            inputKey={mediaFileInputKey}
            uploading={uploadMedia.isPending}
            onFilesSelected={handleMediaFilesSelected}
          />
          {isRunningSport(item.sportType) && (
            <ActivityPlannedMatchAction
              matched={Boolean(matchedPlannedActivity)}
              matchedName={matchedPlannedActivity?.name}
              matchedWorkoutId={matchedPlannedActivity?.workoutId}
              matchedTrainingSheetURL={matchedTrainingSheetURL}
              loading={plannedMatchCandidates.isLoading}
              working={previewPlannedActivity.isPending || applyPlannedActivity.isPending || unmatchPlannedActivity.isPending}
              onMatch={() => openMatchDialog()}
              onUnmatch={() => unmatchPlannedActivity.mutate()}
            />
          )}
          <ActivityDetailActions
            activity={item}
            open={actionsOpen}
            deleting={deleteActivity.isPending}
            canExportGPX={canExportGPX}
            canSaveCourse={canExportGPX && canWrite}
            canOpenCheckIn={Boolean(matchedPlannedActivity)}
            feedbackAvailable={feedbackAvailable}
            canRetryWriteback={canRetryWriteback}
            retryingWriteback={retryWriteback.isPending}
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
            onSaveCourse={() => {
              saveAsCourse.reset();
              setCourseSaveOpen(true);
              setActionsOpen(false);
            }}
            onCheckIn={() => {
              updateActivityReflection.reset();
              setCheckInOpen(true);
              setActionsOpen(false);
            }}
            onRetryWriteback={() => retryWriteback.mutate()}
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
      {matchOpen && (
        <PlannedActivityMatchDialog
          data={plannedMatchResponseForDialog(plannedMatchCandidates.data)}
          targetDate={localDateString(new Date(confirmedItem.startTime))}
          selectedCandidateId={matchCandidateId}
          canLoadMore={canLoadMorePlans}
          loadMoreLabel={loadMorePlansLabel}
          loadingStatus={candidateLoadingStatus}
          loadingMore={loadingCandidateRequest}
          matching={previewPlannedActivity.isPending || applyPlannedActivity.isPending}
          error={plannedMatchCandidates.error ?? previewPlannedActivity.error ?? applyPlannedActivity.error}
          preview={matchPreview}
          onSelectCandidate={setMatchCandidateId}
          onPreview={handlePreviewMatch}
          onApply={handleApplyMatch}
          onPreviewReset={resetMatchPreview}
          onLoadMore={handleLoadMorePlans}
          onClose={closeMatchDialog}
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
      {courseSaveOpen && (
        <ActivitySaveCourseDialog
          activity={item}
          saving={saveAsCourse.isPending}
          error={saveAsCourse.error}
          onSave={(input) => saveAsCourse.mutate(input)}
          onClose={() => setCourseSaveOpen(false)}
        />
      )}
      {plannedMatchCandidates.error && <div className="error">{plannedMatchCandidates.error instanceof Error ? plannedMatchCandidates.error.message : "Could not load planned activity matches"}</div>}
      {unmatchPlannedActivity.error && <div className="error">{unmatchPlannedActivity.error instanceof Error ? unmatchPlannedActivity.error.message : "Could not unmatch planned run"}</div>}
      {retryWriteback.error && <div className="error">{retryWriteback.error instanceof Error ? retryWriteback.error.message : "Could not retry sheet write-back"}</div>}
      {writeback && <section id="writeback" className="panel activity-writeback-panel">
        <div><div className="panel-heading">Training sheet writeback</div><p className="muted">{matchedPlannedActivity?.name || "Matched planned activity"}</p></div>
        <div className="activity-writeback-statuses">
          <span>Summary <strong>{trainingSheetWritebackStatusLabel(writeback.summaryStatus)}</strong></span>
          <span>Intervals <strong>{trainingSheetWritebackStatusLabel(writeback.intervalsStatus)}</strong></span>
          <span>Reflection <strong>{trainingSheetWritebackStatusLabel(writeback.feedbackStatus)}</strong></span>
        </div>
        {canRetryWriteback && <button className="secondary-button small-button" type="button" disabled={retryWriteback.isPending} onClick={() => retryWriteback.mutate()}><RefreshCw size={14} />{retryWriteback.isPending ? "Retrying…" : "Retry writeback"}</button>}
      </section>}
      <section className="metric-grid">
        {supportsRouteMetrics(item.sportType) && <Metric label="Distance" value={formatDistance(item.distanceM)} />}
        <Metric label="Moving Time" value={formatDuration(item.movingTimeS || item.elapsedTimeS)} />
        {supportsRouteMetrics(item.sportType) && <Metric label="Pace" value={formatPace(item.avgPaceSPKM)} />}
        {item.avgHeartRate !== undefined && <Metric label="Avg HR" value={formatBPM(item.avgHeartRate)} />}
        {item.maxHeartRate !== undefined && <Metric label="Max HR" value={formatBPM(item.maxHeartRate)} />}
        {supportsRouteMetrics(item.sportType) && <Metric label="Elevation" value={`${Math.round(item.elevationGainM).toLocaleString()} m`} />}
        {supportsRouteMetrics(item.sportType) && item.avgGradeAdjustedPaceSPKM !== undefined && <Metric label="GAP" value={formatPace(item.avgGradeAdjustedPaceSPKM)} />}
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

      {mediaItems.length > 0 ? (
        <ActivityMediaPanel
          activity={item}
          uploading={uploadMedia.isPending}
          uploadError={uploadMedia.error}
          selectedMediaId={selectedMediaId}
          onSelectMedia={setSelectedMediaId}
          onPinMedia={handleStartMediaPinning}
        />
      ) : (
        Boolean(uploadMedia.error) && <div className="error">{uploadMedia.error instanceof Error ? uploadMedia.error.message : "Upload failed"}</div>
      )}

      {(routePoints.length > 1 || locatedMedia.length > 0 || Boolean(pinningMediaId)) && (
        <section className="panel">
          <div className="route-panel-header">
            <div>
              <div className="panel-heading">{pinningMedia ? "Pin photo" : "Route"}</div>
              {pinningMedia && <p className="muted">Click the map to place this photo.</p>}
            </div>
            {pinningMedia && <button className="secondary-button small-button" type="button" onClick={handleCancelMediaPinning}>Cancel pin</button>}
          </div>
          {updateMediaLocation.error && <div className="error">{updateMediaLocation.error instanceof Error ? updateMediaLocation.error.message : "Could not pin photo"}</div>}
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
            onMapLocationSelect={pinningMediaId ? handleMapLocationSelect : undefined}
            routeColorSource={routeColorSource}
            onRouteColorSourceChange={setRouteColorSource}
            showRouteColorSelector={routeUsesGap}
          />
        </section>
      )}

      {showClimbAnalysis && (
        <ActivityClimbsPanel
          climbs={effectiveClimbs}
          selectedClimb={selectedClimb}
          performanceByIndex={climbPerformanceByIndex}
          selectedPerformance={selectedClimbPerformance}
          profileData={selectedClimbProfile}
          sensitivityControls={climbSensitivityControls}
          onSelect={handleSelectClimb}
        />
      )}

      <div className="activity-analysis-tabs" role="tablist" aria-label="Activity analysis">
        <button
          className={visibleAnalysisTab === "stats" ? "active" : ""}
          type="button"
          role="tab"
          aria-selected={visibleAnalysisTab === "stats"}
          onClick={() => setAnalysisTab("stats")}
        >
          Stats
        </button>
        {intervalAnalysisAvailable && (
          <button
            className={visibleAnalysisTab === "intervals" ? "active" : ""}
            type="button"
            role="tab"
            aria-selected={visibleAnalysisTab === "intervals"}
            onClick={() => setAnalysisTab("intervals")}
          >
            Intervals
          </button>
        )}
      </div>
      {visibleAnalysisTab === "stats" ? (
        <ActivityCombinedChart key={item.id} data={chartData} onHighlight={setHighlightedSample} />
      ) : (
        <ActivityIntervalsPanel activity={displayItem} />
      )}
    </Page>
  );
}

function ActivityIntervalsPanel({ activity }: { activity: Activity }) {
  const intervals = activity.intervals ?? [];
  const laps = activity.laps ?? [];
  const [filter, setFilter] = useState("all");
  const [expanded, setExpanded] = useState<Record<number, boolean>>({});
  const categories = Array.from(new Set(intervals.map((interval) => interval.category).filter(Boolean)));
  const hasSingleStepType = categories.length === 1;

  useEffect(() => {
    setFilter("all");
    setExpanded(hasSingleStepType ? intervals.reduce<Record<number, boolean>>((state, interval) => {
      state[interval.index] = true;
      return state;
    }, {}) : {});
  }, [activity.id, hasSingleStepType, intervals.length]);

  if (intervals.length === 0) {
    return <ActivityFlatLapTable activity={activity} />;
  }

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
        {categories.length > 1 && (
          <label className="compact-field" htmlFor="activity-interval-filter">
            <span>Step Type</span>
            <select id="activity-interval-filter" value={filter} onChange={(event) => setFilter(event.target.value)}>
              <option value="all">All</option>
              {categories.map((category) => (
                <option key={category} value={category}>{intervalCategoryLabel(category, activity.sportType)}</option>
              ))}
            </select>
          </label>
        )}
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
              const intervalLapIndexes = intervalLapIndexesForDisplay(interval, intervals, laps);
              const intervalLaps = intervalLapIndexes.map((index) => lapsByIndex.get(index)).filter((lap): lap is ActivityLap => Boolean(lap));
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
                    <td>{formatLapRange(intervalLapIndexes)}</td>
                    <td>{formatDuration(intervalDisplayTimeS(interval))}</td>
                    <td>{formatDuration(intervalCumulativeTime(interval, intervals))}</td>
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
                      <td>{formatDuration(lapDisplayTimeS(lap, laps.length > 0 ? activity.samples ?? [] : []))}</td>
                      <td>{formatDuration(lapCumulativeTime(lap, laps, activity.samples ?? []))}</td>
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
                <td>{formatDuration(lapDisplayTimeS(lap, activity.samples ?? []))}</td>
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

function supportsClimbAnalysis(sportType: string) {
  const normalized = sportType.trim().toLowerCase();
  if (/(treadmill|swim|kayak)/i.test(normalized)) {
    return false;
  }
  return /(run|walk|hike|cycl|bike|ride)/i.test(normalized);
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

function intervalLapIndexesForDisplay(interval: ActivityInterval, intervals: ActivityInterval[], laps: ActivityLap[]) {
  if (interval.lapIndexes && interval.lapIndexes.length > 0) {
    return interval.lapIndexes;
  }
  if (intervals.length === 1) {
    return laps.map((lap) => lap.index);
  }
  return [];
}

function intervalCumulativeTime(interval: ActivityInterval, intervals: ActivityInterval[]) {
  const index = intervals.findIndex((candidate) => candidate.index === interval.index);
  return Math.round(intervals.slice(0, index + 1).reduce((total, candidate) => total + intervalDisplayTimeS(candidate), 0));
}

function lapCumulativeTime(lap: ActivityLap, laps: ActivityLap[], samples: ActivitySample[]) {
  return Math.round(laps
    .filter((candidate) => candidate.index <= lap.index)
    .reduce((total, candidate) => total + lapDisplayTimeS(candidate, samples), 0));
}

function intervalDisplayTimeS(interval: ActivityInterval) {
  const duration = rawDurationS(interval.raw);
  if (duration !== undefined) {
    return duration;
  }
  return interval.movingTimeS > 0 ? interval.movingTimeS : interval.elapsedTimeS;
}

function lapDisplayTimeS(lap: ActivityLap, samples: ActivitySample[]) {
  const duration = rawDurationS(lap.raw);
  if (duration !== undefined) {
    return duration;
  }
  return lapMovingTimeS(lap, samples);
}

function rawDurationS(raw?: Record<string, unknown>) {
  const value = raw?.duration;
  if (typeof value === "number" && Number.isFinite(value) && value >= 0) {
    return value;
  }
  if (typeof value === "string") {
    const parsed = Number(value);
    if (Number.isFinite(parsed) && parsed >= 0) {
      return parsed;
    }
  }
  return undefined;
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

function PlannedActivityMatchDialog({
  inline = false,
  canApply = true,
  data,
  targetDate,
  selectedCandidateId,
  canLoadMore,
  loadMoreLabel,
  loadingStatus,
  loadingMore,
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
  inline?: boolean;
  canApply?: boolean;
  data: PlannedActivityMatchResponse;
  targetDate: string;
  selectedCandidateId?: string;
  canLoadMore: boolean;
  loadMoreLabel: string;
  loadingStatus: string;
  loadingMore: boolean;
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

  const form = (
      <form
        className={inline ? "panel simple-match-form" : "filter-dialog notes-dialog"}
        role={inline ? undefined : "dialog"}
        aria-modal={inline ? undefined : true}
        aria-labelledby="planned-match-title"
        onSubmit={(event) => {
          event.preventDefault();
          if (!canApply || !selectedCandidateId || !valid) {
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
          {!inline && <button className="icon-button" type="button" aria-label="Close planned run matching" onClick={onClose}>
            <X size={16} />
          </button>}
        </div>
        <p className="muted">Review the sheet changes before matching and writing them back.</p>
        <PlannedActivityMatchAgenda
          candidates={data.candidates ?? []}
          suggestedId={data.suggestedId}
          selectedCandidateId={selectedCandidateId}
          targetDate={targetDate}
          matching={matching || loadingMore}
          onSelectCandidate={onSelectCandidate}
        />
        {data.candidates.length === 0 && <p className="muted">No planned runs were found for this date.</p>}
        {loadingMore && <div className="muted" role="status" aria-live="polite">{loadingStatus}</div>}
        {selectedCandidate && (
          <>
            <label className="field">
              <span>RPE <strong>{rpe}/10</strong></span>
              <input
                className={`rpe-slider rpe-slider--${rpeTone(rpe)}`}
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
            <button className="secondary-button" type="button" disabled={matching || loadingMore} onClick={onLoadMore}>{loadMoreLabel}</button>
          )}
          {preview && (
            <button className="secondary-button" type="button" disabled={matching || loadingMore} onClick={resetPreview}>Edit</button>
          )}
          {!inline && <button className="secondary-button" type="button" disabled={matching} onClick={onClose}>Cancel</button>}
          <button className="primary-button" type="submit" disabled={!canApply || matching || loadingMore || !selectedCandidateId || !valid}>
            {matching ? (preview ? "Applying..." : "Building preview...") : (preview ? "Apply match & write back" : "Preview changes")}
          </button>
        </div>
      </form>
  );
  if (inline) {
    return form;
  }
  return <div className="dialog-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>{form}</div>;
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
            className={`rpe-slider rpe-slider--${rpeTone(rpe)}`}
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

function ActivityPlannedMatchAction({
  matched,
  matchedName,
  matchedWorkoutId,
  matchedTrainingSheetURL,
  loading,
  working,
  onMatch,
  onUnmatch
}: {
  matched: boolean;
  matchedName?: string;
  matchedWorkoutId?: string;
  matchedTrainingSheetURL?: string;
  loading: boolean;
  working: boolean;
  onMatch: () => void;
  onUnmatch: () => void;
}) {
  const label = working ? (matched ? "Unmatching" : "Matching") : (matched ? "Unmatch" : "Match");
  return (
    <>
      {matched && matchedWorkoutId && <Link className="secondary-button" to={`/workouts/${matchedWorkoutId}`} title={`Open workout for ${matchedName ?? "planned run"}`}><RouteIcon size={16} />Workout</Link>}
      {matched && matchedTrainingSheetURL && <a className="secondary-button" href={matchedTrainingSheetURL} target="_blank" rel="noreferrer" title={`Open training sheet for ${matchedName ?? "planned run"}`}><ExternalLink size={16} />Training sheet</a>}
      <button
        className="secondary-button"
        type="button"
        title={matched ? `Unmatch ${matchedName ?? "planned run"}` : "Match with a planned run"}
        disabled={loading || working}
        onClick={matched ? onUnmatch : onMatch}
      >
        {matched && <RotateCcw size={16} />}
        {label}
      </button>
    </>
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
  onSelectMedia,
  onPinMedia
}: {
  activity: Activity;
  uploading: boolean;
  uploadError: unknown;
  selectedMediaId?: string;
  onSelectMedia: (mediaId?: string) => void;
  onPinMedia: (mediaId: string) => void;
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
          onPinMedia={() => onPinMedia(previewMedia.id)}
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
  onNext,
  onPinMedia
}: {
  media: ActivityMedia;
  deleting: boolean;
  onClose: () => void;
  onDelete: () => void;
  onPrevious?: () => void;
  onNext?: () => void;
  onPinMedia: () => void;
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
          <button className="secondary-button" type="button" onClick={onPinMedia}>
            {hasMediaLocation(media) ? "Move pin" : "Pin to map"}
          </button>
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
  canSaveCourse,
  canOpenCheckIn,
  feedbackAvailable,
  canRetryWriteback,
  retryingWriteback,
  onToggle,
  onRename,
  onNotes,
  onExportGPX,
  onSaveCourse,
  onCheckIn,
  onRetryWriteback,
  onDelete
}: {
  activity: Activity;
  open: boolean;
  deleting: boolean;
  canExportGPX: boolean;
  canSaveCourse: boolean;
  canOpenCheckIn: boolean;
  feedbackAvailable: boolean;
  canRetryWriteback: boolean;
  retryingWriteback: boolean;
  onToggle: () => void;
  onRename: () => void;
  onNotes: () => void;
  onExportGPX: () => void;
  onSaveCourse: () => void;
  onCheckIn: () => void;
  onRetryWriteback: () => void;
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
          {canOpenCheckIn && (
            <button className="action-menu-item" type="button" role="menuitem" onClick={onCheckIn}>
              <Timer size={16} />
              {feedbackAvailable ? "RPE & feedback" : "RPE"}
            </button>
          )}
          <button className="action-menu-item" type="button" role="menuitem" disabled={!canExportGPX} onClick={onExportGPX}>
            <Download size={16} />
            Export GPX
          </button>
          <button className="action-menu-item" type="button" role="menuitem" disabled={!canSaveCourse} onClick={onSaveCourse}>
            <RouteIcon size={16} />
            Save as course
          </button>
          {activity.originalProviderUrl && (
            <a className="action-menu-item" role="menuitem" href={activity.originalProviderUrl} target="_blank" rel="noreferrer">
              <ExternalLink size={16} />
              Open original
            </a>
          )}
          {canRetryWriteback && (
            <button className="action-menu-item" type="button" role="menuitem" disabled={retryingWriteback} onClick={onRetryWriteback}>
              <RefreshCw size={16} />
              {retryingWriteback ? "Retrying..." : "Retry write-back"}
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

function ActivitySaveCourseDialog({ activity, saving, error, onSave, onClose }: { activity: Activity; saving: boolean; error: unknown; onSave: (input: { name: string; sportType: CourseSport; notes: string }) => void; onClose: () => void }) {
  const [name, setName] = useState(activity.name);
  const [sportType, setSportType] = useState<CourseSport | "">(inferCourseSport(activity.sportType));
  const [notes, setNotes] = useState(activity.notes ?? "");
  return (
    <div className="dialog-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <form className="filter-dialog" role="dialog" aria-modal="true" aria-labelledby="activity-save-course-title" onSubmit={(event) => { event.preventDefault(); if (name.trim() && sportType) onSave({ name: name.trim(), sportType, notes }); }}>
        <div className="dialog-header"><div><div className="eyebrow">GPS activity</div><h2 id="activity-save-course-title">Save as course</h2></div><button className="icon-button" type="button" aria-label="Close" onClick={onClose}><X size={16} /></button></div>
        <label className="field"><span>Name</span><input autoFocus maxLength={160} value={name} onChange={(event) => setName(event.target.value)} /></label>
        <label className="field"><span>Sport</span><select value={sportType} onChange={(event) => setSportType(event.target.value as CourseSport | "")}><option value="" disabled>Choose a sport</option>{courseSports.map((item) => <option key={item}>{item}</option>)}</select></label>
        <label className="field"><span>Notes</span><textarea maxLength={5000} rows={5} value={notes} onChange={(event) => setNotes(event.target.value)} /></label>
        <p className="muted">Runnarr copies the activity route into a private course. The activity and provider data are not changed.</p>
        {Boolean(error) && <div className="error">{courseMutationMessage(error)}</div>}
        <div className="dialog-actions"><button className="secondary-button" type="button" disabled={saving} onClick={onClose}>Cancel</button><button className="primary-button" type="submit" disabled={saving || !name.trim() || !sportType}>{saving ? "Saving…" : "Save course"}</button></div>
      </form>
    </div>
  );
}

function inferCourseSport(activitySport: string): CourseSport | "" {
  const sport = activitySport.toLowerCase();
  if (sport.includes("hike")) return "Hike";
  if (sport.includes("walk")) return "Walk";
  if (sport.includes("cycl") || sport.includes("bike") || sport.includes("biking")) return "Cycling";
  if (sport.includes("run")) return "Run";
  return "";
}

function ActivityClimbsPanel({
  climbs,
  selectedClimb,
  performanceByIndex,
  selectedPerformance,
  profileData,
  sensitivityControls,
  onSelect
}: {
  climbs: ActivityClimb[];
  selectedClimb?: ActivityClimb;
  performanceByIndex: Record<number, ClimbPerformance>;
  selectedPerformance?: ClimbPerformance;
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
              const performance = performanceByIndex[climb.index];
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
                  {(performance.paceSPKM !== undefined || performance.gapSPKM !== undefined) && (
                    <span className="climb-item-performance">
                      {performance.paceSPKM !== undefined && <span>Pace {formatPace(performance.paceSPKM)}</span>}
                      {performance.gapSPKM !== undefined && <span>GAP {formatPace(performance.gapSPKM)}</span>}
                    </span>
                  )}
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
                {selectedPerformance?.paceSPKM !== undefined && <ClimbStat label="Pace" value={formatPace(selectedPerformance.paceSPKM)} />}
                {selectedPerformance?.gapSPKM !== undefined && <ClimbStat label="GAP" value={formatPace(selectedPerformance.gapSPKM)} />}
              </div>
              <div className="climb-profile">
                <div className="climb-profile-header">
                  <span className="muted">Height above start</span>
                  <span className="climb-profile-legend" aria-label="Climb profile series">
                    <span><i className="climb-profile-legend-swatch elevation" /> Elevation</span>
                    {profileData.some((point) => point.paceSPKM !== undefined) && <span><i className="climb-profile-legend-swatch pace" /> Pace</span>}
                    {profileData.some((point) => point.gapSPKM !== undefined) && <span><i className="climb-profile-legend-swatch gap" /> GAP</span>}
                  </span>
                </div>
                <div className="climb-profile-chart">
                  <ResponsiveContainer width="100%" height="100%">
                    <AreaChart data={profileData}>
                      <CartesianGrid strokeDasharray="3 3" vertical={false} />
                      <XAxis dataKey="label" minTickGap={26} />
                      <YAxis yAxisId="elevation" width={44} domain={[0, "dataMax"]} tickFormatter={(value) => String(Math.round(Number(value)))} />
                      {(profileData.some((point) => point.paceSPKM !== undefined) || profileData.some((point) => point.gapSPKM !== undefined)) && (
                        <YAxis
                          yAxisId="performance"
                          orientation="right"
                          width={58}
                          reversed
                          domain={["auto", "auto"]}
                          tickFormatter={(value) => formatPaceMinutesSeconds(Number(value))}
                        />
                      )}
                      <Tooltip
                        contentStyle={chartTooltipContentStyle}
                        labelStyle={chartTooltipLabelStyle}
                        formatter={(value, name) => {
                          const numericValue = Number(value);
                          if (name === "Pace" || name === "GAP") {
                            return [formatPace(numericValue), String(name)];
                          }
                          return [`${Math.round(numericValue).toLocaleString()} m`, "Height above start"];
                        }}
                      />
                      <Area yAxisId="elevation" type="monotone" dataKey="elevationM" name="Elevation" stroke="#b7791f" fill="#f6c432" fillOpacity={0.5} dot={false} />
                      {profileData.some((point) => point.paceSPKM !== undefined) && (
                        <Line yAxisId="performance" type="monotone" dataKey="paceSPKM" name="Pace" stroke="#2f6df6" strokeWidth={2} dot={false} connectNulls={false} />
                      )}
                      {profileData.some((point) => point.gapSPKM !== undefined) && (
                        <Line yAxisId="performance" type="monotone" dataKey="gapSPKM" name="GAP" stroke="#c84d4d" strokeWidth={2} dot={false} connectNulls={false} />
                      )}
                    </AreaChart>
                  </ResponsiveContainer>
                </div>
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

const courseSports: CourseSport[] = ["Run", "Walk", "Hike", "Cycling"];

function CoursesPage({ canWrite }: { canWrite: boolean }) {
  const queryClient = useQueryClient();
  const [query, setQuery] = useState("");
  const [sport, setSport] = useState<CourseSport | "">("");
  const [favoritesOnly, setFavoritesOnly] = useState(false);
  const courses = useQuery({
    queryKey: ["courses", query, sport, favoritesOnly],
    queryFn: () => api.courses({ q: query, sport, favorite: favoritesOnly ? true : undefined, sort: "updated", order: "desc", limit: 100 })
  });
  const favorite = useMutation({
    mutationFn: ({ id, value }: { id: string; value: boolean }) => api.setCourseFavorite(id, value),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["courses"] })
  });
  const items = courses.data?.courses ?? [];

  return (
    <Page
      title="Courses"
      eyebrow="Reusable routes"
      actions={canWrite ? <><Link className="secondary-button" to="/courses/import"><FileUp size={16} />Upload GPX</Link><Link className="primary-button" to="/courses/new"><RouteIcon size={16} />New course</Link></> : undefined}
    >
      <section className="panel course-library-panel">
        <div className="course-library-filters">
          <label className="field course-search-field">
            <span>Search</span>
            <input type="search" value={query} placeholder="Name or notes" onChange={(event) => setQuery(event.target.value)} />
          </label>
          <label className="field">
            <span>Sport</span>
            <select value={sport} onChange={(event) => setSport(event.target.value as CourseSport | "")}>
              <option value="">All sports</option>
              {courseSports.map((item) => <option key={item}>{item}</option>)}
            </select>
          </label>
          <label className="course-favorite-filter">
            <input type="checkbox" checked={favoritesOnly} onChange={(event) => setFavoritesOnly(event.target.checked)} />
            Favorites only
          </label>
        </div>
        {courses.isLoading && <LoadingRow />}
        {courses.error && <div className="error">{courses.error instanceof Error ? courses.error.message : "Could not load courses"}</div>}
        {!courses.isLoading && !courses.error && items.length === 0 && (
          <EmptyState title="No courses in this view" message={query || sport || favoritesOnly ? "Try clearing a filter." : "Upload a GPX file or save a GPS activity as a course."} />
        )}
        {items.length > 0 && (
          <div className="table-wrap">
            <table className="data-table course-table">
              <thead><tr><th><span className="visually-hidden">Favorite</span></th><th>Name</th><th>Sport</th><th>Distance</th><th>Ascent</th><th>Updated</th></tr></thead>
              <tbody>
                {items.map((course) => (
                  <tr key={course.id}>
                    <td>
                      <button
                        className={`icon-button course-favorite-button ${course.favorite ? "active" : ""}`}
                        type="button"
                        aria-label={course.favorite ? `Remove ${course.name} from favorites` : `Add ${course.name} to favorites`}
                        aria-pressed={course.favorite}
                        disabled={!canWrite || favorite.isPending}
                        onClick={() => favorite.mutate({ id: course.id, value: !course.favorite })}
                      ><Star size={17} fill={course.favorite ? "currentColor" : "none"} /></button>
                    </td>
                    <td><Link to={`/courses/${encodeURIComponent(course.id)}`}>{course.name}</Link>{course.notes && <span className="course-row-note">{course.notes}</span>}</td>
                    <td>{course.sportType}</td>
                    <td>{formatDistance(course.distanceM)}</td>
                    <td>{formatCourseElevation(course.elevationGainM)}</td>
                    <td>{formatDate(course.updatedAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </Page>
  );
}

function CourseDetailPage({ canWrite, mapTileURL }: { canWrite: boolean; mapTileURL?: string }) {
  const { id } = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [editOpen, setEditOpen] = useState(false);
  const [duplicateOpen, setDuplicateOpen] = useState(false);
  const [highlighted, setHighlighted] = useState<CourseProfilePoint>();
  const course = useQuery({ queryKey: ["course", id], queryFn: () => api.course(id!), enabled: Boolean(id) });
  const favorite = useMutation({
    mutationFn: (value: boolean) => api.setCourseFavorite(id!, value),
    onSuccess: async () => {
      await Promise.all([queryClient.invalidateQueries({ queryKey: ["course", id] }), queryClient.invalidateQueries({ queryKey: ["courses"] })]);
    }
  });
  const update = useMutation({
    mutationFn: (input: { name: string; sportType: CourseSport; notes: string }) => api.updateCourseDetails(id!, { revision: course.data!.revision, ...input }),
    onSuccess: async () => {
      setEditOpen(false);
      await Promise.all([queryClient.invalidateQueries({ queryKey: ["course", id] }), queryClient.invalidateQueries({ queryKey: ["courses"] })]);
    }
  });
  const duplicate = useMutation({
    mutationFn: (input: { name: string; notes: string }) => api.duplicateCourse(id!, { revision: course.data!.revision, ...input }),
    onSuccess: (created) => {
      void queryClient.invalidateQueries({ queryKey: ["courses"] });
      navigate(`/courses/${encodeURIComponent(created.id)}`);
    }
  });
  const remove = useMutation({
    mutationFn: () => api.deleteCourse(id!, course.data!.revision),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["courses"] });
      navigate("/courses");
    }
  });

  if (course.isLoading) return <Page title="Course"><LoadingRow /></Page>;
  if (!course.data) return <Page title="Course"><EmptyState title="Course not found" /></Page>;
  const item = course.data;
  const diagnostics = Object.entries(item.diagnostics ?? {});

  return (
    <Page
      title={item.name}
      eyebrow={`${item.sportType} course`}
      actions={<>
        <button className={`icon-button course-favorite-button ${item.favorite ? "active" : ""}`} type="button" title={item.favorite ? "Remove from favorites" : "Add to favorites"} aria-label={item.favorite ? "Remove from favorites" : "Add to favorites"} aria-pressed={item.favorite} disabled={!canWrite || favorite.isPending} onClick={() => favorite.mutate(!item.favorite)}><Star size={18} fill={item.favorite ? "currentColor" : "none"} /></button>
        {canWrite && <button className="secondary-button small-button" type="button" onClick={() => { update.reset(); setEditOpen(true); }}><Pencil size={15} />Details</button>}
        {canWrite && <Link className="secondary-button small-button" to={`/courses/${encodeURIComponent(item.id)}/plan`}><RouteIcon size={15} />Edit route</Link>}
        {canWrite && <button className="secondary-button small-button" type="button" onClick={() => { duplicate.reset(); setDuplicateOpen(true); }}><Copy size={15} />Duplicate</button>}
        <a className="secondary-button small-button" href={courseGPXURL(item.id)}><Download size={15} />GPX</a>
        {canWrite && <button className="danger-button small-button" type="button" disabled={remove.isPending} onClick={() => { if (window.confirm(`Permanently delete “${item.name}”? This cannot be undone.`)) remove.mutate(); }}><Trash2 size={15} />Delete</button>}
      </>}
    >
      {(favorite.error || update.error || duplicate.error || remove.error) && <div className="error">{courseMutationMessage(favorite.error ?? update.error ?? duplicate.error ?? remove.error)}</div>}
      {editOpen && <CourseDetailsDialog course={item} saving={update.isPending} error={update.error} onSave={(input) => update.mutate(input)} onClose={() => setEditOpen(false)} />}
      {duplicateOpen && <CourseDuplicateDialog course={item} saving={duplicate.isPending} error={duplicate.error} onSave={(input) => duplicate.mutate(input)} onClose={() => setDuplicateOpen(false)} />}
      <section className="metric-grid course-metric-grid">
        <Metric label="Distance" value={formatDistance(item.distanceM)} icon={<RouteIcon size={18} />} />
        <Metric label="Ascent" value={formatCourseElevation(item.elevationGainM)} icon={<Mountain size={18} />} />
        <Metric label="Descent" value={formatCourseElevation(item.elevationLossM)} icon={<ArrowDown size={18} />} />
      </section>
      {item.elevationCoverage < 0.9995 && <CourseElevationCoverageNotice coverage={item.elevationCoverage} />}
      <section className="course-detail-grid">
        <section className="panel course-map-panel">
          <div className="panel-heading">Route</div>
          <CourseMap legs={item.legs} tileURL={mapTileURL} highlighted={highlighted ? [highlighted.latitude, highlighted.longitude] : undefined} allowLocation />
        </section>
        <CourseElevationProfile profile={item.profile} onHighlight={setHighlighted} />
      </section>
      {item.notes && <section className="panel"><div className="panel-heading">Notes</div><p className="course-notes">{item.notes}</p></section>}
      {(item.directLegCount > 0 || diagnostics.length > 0) && (
        <details className="panel course-diagnostics">
          <summary>Diagnostics {item.directLegCount > 0 && <span className="warning-badge">{item.directLegCount} direct {item.directLegCount === 1 ? "leg" : "legs"}</span>}</summary>
          {item.directLegCount > 0 && <p>Direct legs are shown dashed because they were not matched to a routed path.</p>}
          {diagnostics.map(([key, value]) => <div className="course-diagnostic-row" key={key}><strong>{courseDiagnosticLabel(key)}</strong><span>{formatCourseDiagnostic(value)}</span></div>)}
        </details>
      )}
    </Page>
  );
}

function CourseDetailsDialog({ course, saving, error, onSave, onClose }: { course: Course; saving: boolean; error: unknown; onSave: (input: { name: string; sportType: CourseSport; notes: string }) => void; onClose: () => void }) {
  const [name, setName] = useState(course.name);
  const [sportType, setSportType] = useState(course.sportType);
  const [notes, setNotes] = useState(course.notes ?? "");
  return (
    <div className="dialog-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <form className="filter-dialog" role="dialog" aria-modal="true" aria-labelledby="course-details-title" onSubmit={(event) => { event.preventDefault(); if (name.trim()) onSave({ name: name.trim(), sportType, notes }); }}>
        <div className="dialog-header"><div><div className="eyebrow">Course</div><h2 id="course-details-title">Edit details</h2></div><button className="icon-button" type="button" aria-label="Close" onClick={onClose}><X size={16} /></button></div>
        <CourseDetailsFields name={name} sportType={sportType} notes={notes} onName={setName} onSport={setSportType} onNotes={setNotes} />
        {Boolean(error) && <div className="error">{courseMutationMessage(error)}</div>}
        <div className="dialog-actions"><button className="secondary-button" type="button" disabled={saving} onClick={onClose}>Cancel</button><button className="primary-button" type="submit" disabled={saving || !name.trim()}>{saving ? "Saving…" : "Save"}</button></div>
      </form>
    </div>
  );
}

function CourseDuplicateDialog({ course, saving, error, onSave, onClose }: { course: Course; saving: boolean; error: unknown; onSave: (input: { name: string; notes: string }) => void; onClose: () => void }) {
  const [name, setName] = useState(`${course.name} copy`);
  const [notes, setNotes] = useState(course.notes ?? "");
  return (
    <div className="dialog-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <form className="filter-dialog" role="dialog" aria-modal="true" aria-labelledby="course-duplicate-title" onSubmit={(event) => { event.preventDefault(); if (name.trim()) onSave({ name: name.trim(), notes }); }}>
        <div className="dialog-header"><div><div className="eyebrow">Course</div><h2 id="course-duplicate-title">Duplicate</h2></div><button className="icon-button" type="button" aria-label="Close" onClick={onClose}><X size={16} /></button></div>
        <label className="field"><span>Name</span><input autoFocus maxLength={160} value={name} onChange={(event) => setName(event.target.value)} /></label>
        <label className="field"><span>Notes</span><textarea maxLength={5000} rows={5} value={notes} onChange={(event) => setNotes(event.target.value)} /></label>
        <p className="muted">The duplicate starts unfavorited and has independent details and geometry.</p>
        {Boolean(error) && <div className="error">{courseMutationMessage(error)}</div>}
        <div className="dialog-actions"><button className="secondary-button" type="button" disabled={saving} onClick={onClose}>Cancel</button><button className="primary-button" type="submit" disabled={saving || !name.trim()}>{saving ? "Duplicating…" : "Duplicate"}</button></div>
      </form>
    </div>
  );
}

function CourseDetailsFields({ name, sportType, notes, onName, onSport, onNotes }: { name: string; sportType: CourseSport; notes: string; onName: (value: string) => void; onSport: (value: CourseSport) => void; onNotes: (value: string) => void }) {
  return <>
    <label className="field"><span>Name</span><input maxLength={160} value={name} onChange={(event) => onName(event.target.value)} /></label>
    <label className="field"><span>Sport</span><select value={sportType} onChange={(event) => onSport(event.target.value as CourseSport)}>{courseSports.map((item) => <option key={item}>{item}</option>)}</select></label>
    <label className="field"><span>Notes</span><textarea maxLength={5000} rows={5} value={notes} onChange={(event) => onNotes(event.target.value)} /></label>
  </>;
}

function CoursePlannerPage({ canWrite, mapTileURL, routingEnabled }: { canWrite: boolean; mapTileURL?: string; routingEnabled: boolean }) {
  const { id } = useParams();
  const editing = Boolean(id);
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const initializedCourseRef = useRef<string>();
  const initializedCourseStartRef = useRef(false);
  const [name, setName] = useState("");
  const [sportType, setSportType] = useState<CourseSport>("Run");
  const [notes, setNotes] = useState("");
  const [waypoints, setWaypoints] = useState<CourseWaypoint[]>([]);
  const [seedLegs, setSeedLegs] = useState<CourseLeg[]>([]);
  const [geometryDirty, setGeometryDirty] = useState(!editing);
  const [directLegIndexes, setDirectLegIndexes] = useState<number[]>([]);
  const [highlighted, setHighlighted] = useState<CourseProfilePoint>();
  const [draggedWaypointIndex, setDraggedWaypointIndex] = useState<number>();
  const [waypointDropIndex, setWaypointDropIndex] = useState<number>();
  const [mapFullscreen, setMapFullscreen] = useState(false);
  const course = useQuery({ queryKey: ["course", id], queryFn: () => api.course(id!), enabled: editing });
  const previousCourse = useQuery({
    queryKey: ["courses", "planner-start"],
    queryFn: async () => {
      const page = await api.courses({ sort: "updated", order: "desc", limit: 1 });
      return page.courses[0] ? api.course(page.courses[0].id) : null;
    },
    enabled: !editing
  });

  useEffect(() => {
    if (editing || previousCourse.isLoading || initializedCourseStartRef.current) return;
    initializedCourseStartRef.current = true;
    const start = previousCourse.data?.waypoints[0];
    if (waypoints.length === 0 && start) {
      setWaypoints([{ index: 0, latitude: start.latitude, longitude: start.longitude }]);
    }
  }, [editing, previousCourse.data, previousCourse.isLoading, waypoints.length]);

  useEffect(() => {
    if (!id || !course.data || initializedCourseRef.current === id) return;
    initializedCourseRef.current = id;
    setName(course.data.name);
    setSportType(course.data.sportType);
    setNotes(course.data.notes ?? "");
    setWaypoints(course.data.waypoints.map((waypoint, index) => ({ index, name: waypoint.name, latitude: waypoint.latitude, longitude: waypoint.longitude })));
    setSeedLegs(course.data.legs);
    setDirectLegIndexes(course.data.legs.filter((leg) => leg.mode === "direct").map((leg) => leg.index));
    setGeometryDirty(false);
  }, [course.data, id]);

  useEffect(() => {
    if (!mapFullscreen) return;
    const previousOverflow = document.body.style.overflow;
    const exitOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setMapFullscreen(false);
    };
    document.body.style.overflow = "hidden";
    document.addEventListener("keydown", exitOnEscape);
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", exitOnEscape);
    };
  }, [mapFullscreen]);

  const waypointKey = waypoints.map((point) => `${point.latitude.toFixed(6)},${point.longitude.toFixed(6)}`).join(";");
  const directKey = directLegIndexes.slice().sort((a, b) => a - b).join(",");
  const routed = useQuery({
    queryKey: ["course-routing", sportType, waypointKey, directKey],
    queryFn: () => api.routeCourseLegs({ sportType, waypoints, directLegIndexes }),
    enabled: canWrite && geometryDirty && waypoints.length >= 2,
    retry: false,
    staleTime: 0
  });
  const fallbackLegs = plannerDirectLegs(waypoints, routed.error ? "Routing request failed; this leg is direct." : "");
  const plannerLegs: CourseRoutingLeg[] = !geometryDirty
    ? seedLegs.map((leg) => ({ ...leg }))
    : routed.data?.legs.length === waypoints.length - 1
      ? routed.data.legs
      : fallbackLegs;
  const directLegs = plannerLegs.filter((leg) => leg.mode === "direct");
  const routeDistanceM = !geometryDirty ? course.data?.distanceM ?? courseLegDistance(plannerLegs) : routed.data?.distanceM ?? courseLegDistance(plannerLegs);
  const elevationGainM = !geometryDirty ? course.data?.elevationGainM : routed.data?.elevationGainM;
  const elevationLossM = !geometryDirty ? course.data?.elevationLossM : routed.data?.elevationLossM;
  const elevationCoverage = !geometryDirty ? course.data?.elevationCoverage : routed.data?.elevationCoverage;
  const elevationProfile = !geometryDirty ? course.data?.profile ?? [] : routed.data?.profile ?? [];
  const returnsToStart = plannerReturnsToStart(waypoints);
  const canSave = canWrite && name.trim().length > 0 && waypoints.length >= 2 && plannerLegs.length === waypoints.length - 1 && !routed.isFetching;
  const save = useMutation({
    mutationFn: () => {
      const body = {
        name: name.trim(),
        sportType,
        notes,
        waypoints: waypoints.map((waypoint) => ({ index: waypoint.index, name: waypoint.name })),
        legs: plannerLegs.map((leg) => ({ mode: leg.mode, encodedPolyline: leg.encodedPolyline, elevationsM: leg.elevationsM }))
      };
      return editing ? api.updateCoursePlan(id!, { ...body, revision: course.data!.revision }) : api.createCourse(body);
    },
    onSuccess: async (saved) => {
      await Promise.all([queryClient.invalidateQueries({ queryKey: ["courses"] }), queryClient.invalidateQueries({ queryKey: ["course", saved.id] })]);
      navigate(`/courses/${encodeURIComponent(saved.id)}`);
    }
  });

  const markGeometryDirty = () => {
    setGeometryDirty(true);
    setHighlighted(undefined);
    save.reset();
  };
  const reindexWaypoints = (items: typeof waypoints) => items.map((point, index) => ({ ...point, index }));
  const addWaypoint = (point: RoutePoint) => {
    if (!canWrite || waypoints.length >= 100) return;
    setWaypoints((current) => reindexWaypoints([...current, { index: current.length, latitude: point[0], longitude: point[1] }]));
    markGeometryDirty();
  };
  const moveWaypoint = (index: number, point: RoutePoint) => {
    setWaypoints((current) => {
      const keepClosed = index === 0 && plannerReturnsToStart(current);
      const finishIndex = current.length - 1;
      return current.map((item) => item.index === index || (keepClosed && item.index === finishIndex) ? { ...item, latitude: point[0], longitude: point[1] } : item);
    });
    markGeometryDirty();
  };
  const addReturnToStart = () => {
    if (!canWrite || waypoints.length < 2 || waypoints.length >= 100 || returnsToStart) return;
    const start = waypoints[0];
    setWaypoints((current) => reindexWaypoints([...current, { index: current.length, latitude: start.latitude, longitude: start.longitude }]));
    markGeometryDirty();
  };
  const removeWaypoint = (index: number) => {
    setWaypoints((current) => reindexWaypoints(current.filter((item) => item.index !== index)));
    setDirectLegIndexes([]);
    markGeometryDirty();
  };
  const renameWaypoint = (index: number, value: string) => {
    setWaypoints((current) => current.map((item) => item.index === index ? { ...item, name: value } : item));
    save.reset();
  };
  const reorderWaypoint = (index: number, direction: -1 | 1) => {
    setWaypoints((current) => {
      const target = index + direction;
      if (target < 0 || target >= current.length) return current;
      const next = [...current];
      [next[index], next[target]] = [next[target], next[index]];
      return reindexWaypoints(next);
    });
    setDirectLegIndexes([]);
    markGeometryDirty();
  };
  const moveWaypointInList = (fromIndex: number, toIndex: number) => {
    if (!canWrite || fromIndex === toIndex || fromIndex < 0 || toIndex < 0 || fromIndex >= waypoints.length || toIndex >= waypoints.length) return;
    setWaypoints((current) => {
      const next = [...current];
      const [moved] = next.splice(fromIndex, 1);
      next.splice(toIndex, 0, moved);
      return reindexWaypoints(next);
    });
    setDirectLegIndexes([]);
    markGeometryDirty();
  };
  const waypointIndexAtPointer = (clientX: number, clientY: number) => {
    const row = document.elementFromPoint(clientX, clientY)?.closest<HTMLElement>("[data-waypoint-index]");
    const index = Number(row?.dataset.waypointIndex);
    return Number.isInteger(index) && index >= 0 && index < waypoints.length ? index : undefined;
  };
  const setDirect = (index: number, direct: boolean) => {
    setDirectLegIndexes((current) => direct ? Array.from(new Set([...current, index])).sort((a, b) => a - b) : current.filter((item) => item !== index));
    markGeometryDirty();
  };
  const submit = () => {
    if (!canSave) return;
    if (directLegs.length > 0 && !window.confirm(`${directLegs.length} ${directLegs.length === 1 ? "leg is" : "legs are"} direct rather than routed. Save this course anyway?`)) return;
    save.mutate();
  };

  if (editing && course.isLoading) return <Page title="Course planner"><LoadingRow /></Page>;
  if (editing && !course.data) return <Page title="Course planner"><EmptyState title="Course not found" /></Page>;

  return <Page title={editing ? "Edit course route" : "Plan a course"} eyebrow={editing ? course.data?.name : "Waypoint planner"} actions={<><Link className="secondary-button" to={editing ? `/courses/${encodeURIComponent(id!)}` : "/courses"}>Cancel</Link><button className="primary-button" type="button" disabled={!canSave || save.isPending} onClick={submit}>{save.isPending ? "Saving…" : editing ? "Save changes" : "Save course"}</button></>}>
    {!canWrite && <div className="error">The planner is disabled in read-only support mode.</div>}
    {!routingEnabled && <div className="course-routing-notice"><strong>Routing is not enabled.</strong> Waypoints are connected with direct dashed legs. Configure the optional Valhalla service to follow paths and roads.</div>}
    {save.error && <div className="error">{save.error instanceof ApiError && save.error.status === 409 ? "This course changed after you opened it. Reload the latest version before saving." : courseMutationMessage(save.error)}</div>}
    <section className="course-planner-layout">
      <aside className="panel course-planner-sidebar">
        <div className="panel-heading">Course details</div>
        <CourseDetailsFields name={name} sportType={sportType} notes={notes} onName={setName} onSport={(value) => { setSportType(value); markGeometryDirty(); }} onNotes={setNotes} />
        <div className="course-planner-summary">
          <span><strong>{waypoints.length}</strong> waypoints</span>
          <span><strong>{plannerLegs.length}</strong> legs</span>
        </div>
        <div className="course-waypoint-heading"><strong>Waypoints</strong>{waypoints.length > 0 && <span className="course-waypoint-heading-actions">{waypoints.length >= 2 && <button className="course-back-to-start-button" type="button" title={returnsToStart ? "The course already finishes at its start." : "Add a final leg back to the starting point."} disabled={!canWrite || returnsToStart || waypoints.length >= 100} onClick={addReturnToStart}><RotateCcw size={13} />Back to start</button>}<button className="danger-text-button" type="button" disabled={!canWrite} onClick={() => { setWaypoints([]); setDirectLegIndexes([]); markGeometryDirty(); }}>Clear</button></span>}</div>
        {waypoints.length === 0 && <p className="muted">Click the map to add a start and finish.</p>}
        <ol className="course-waypoint-list">
          {waypoints.map((waypoint, index) => {
            const dropDirection = waypointDropIndex === index && draggedWaypointIndex !== undefined && draggedWaypointIndex !== index
              ? index < draggedWaypointIndex ? "before" : "after"
              : undefined;
            return <li
            key={`${waypoint.index}-${waypoint.latitude}-${waypoint.longitude}`}
            className={`${draggedWaypointIndex === index ? "dragging" : ""} ${dropDirection ? `drag-over drop-${dropDirection}` : ""}`.trim()}
            data-waypoint-index={index}
            data-drop-label={dropDirection ? `Move waypoint ${draggedWaypointIndex! + 1} ${dropDirection} waypoint ${index + 1}` : undefined}
            draggable={canWrite}
            onDragStart={(event) => {
              event.dataTransfer.effectAllowed = "move";
              event.dataTransfer.setData("text/plain", String(index));
              setDraggedWaypointIndex(index);
            }}
            onDragEnter={() => setWaypointDropIndex(index)}
            onDragOver={(event) => {
              event.preventDefault();
              event.dataTransfer.dropEffect = "move";
            }}
            onDrop={(event) => {
              event.preventDefault();
              const fromIndex = draggedWaypointIndex ?? Number(event.dataTransfer.getData("text/plain"));
              moveWaypointInList(fromIndex, index);
              setDraggedWaypointIndex(undefined);
              setWaypointDropIndex(undefined);
            }}
            onDragEnd={() => {
              setDraggedWaypointIndex(undefined);
              setWaypointDropIndex(undefined);
            }}
          >
            <span
              className="course-waypoint-drag-handle"
              aria-label={`Drag waypoint ${index + 1} to reorder`}
              title={`Drag waypoint ${index + 1} to reorder`}
              onPointerDown={(event) => {
                if (!canWrite || event.pointerType === "mouse") return;
                event.preventDefault();
                try { event.currentTarget.setPointerCapture(event.pointerId); } catch { /* Synthetic pointer events do not establish capture. */ }
                setDraggedWaypointIndex(index);
                setWaypointDropIndex(index);
              }}
              onPointerMove={(event) => {
                if (!canWrite || event.pointerType === "mouse") return;
                const targetIndex = waypointIndexAtPointer(event.clientX, event.clientY);
                if (targetIndex !== undefined) setWaypointDropIndex(targetIndex);
              }}
              onPointerUp={(event) => {
                if (!canWrite || event.pointerType === "mouse") return;
                const targetIndex = waypointIndexAtPointer(event.clientX, event.clientY);
                if (targetIndex !== undefined) moveWaypointInList(index, targetIndex);
                setDraggedWaypointIndex(undefined);
                setWaypointDropIndex(undefined);
              }}
              onPointerCancel={() => {
                setDraggedWaypointIndex(undefined);
                setWaypointDropIndex(undefined);
              }}
            ><GripVertical size={16} aria-hidden="true" /></span>
            <span className="course-waypoint-number">{index + 1}</span>
            <span><input className="course-waypoint-name" aria-label={`Waypoint ${index + 1} name`} maxLength={160} placeholder={defaultCourseWaypointName(index, waypoints.length)} value={waypoint.name ?? ""} disabled={!canWrite} onChange={(event) => renameWaypoint(index, event.target.value)} /><small>{waypoint.latitude.toFixed(5)}, {waypoint.longitude.toFixed(5)}</small></span>
            <span className="course-waypoint-actions"><button className="icon-button" type="button" aria-label={`Move waypoint ${index + 1} earlier`} disabled={!canWrite || index === 0} onClick={() => reorderWaypoint(index, -1)}><ArrowUp size={14} /></button><button className="icon-button" type="button" aria-label={`Move waypoint ${index + 1} later`} disabled={!canWrite || index === waypoints.length - 1} onClick={() => reorderWaypoint(index, 1)}><ArrowDown size={14} /></button><button className="icon-button danger" type="button" aria-label={`Remove waypoint ${index + 1}`} disabled={!canWrite} onClick={() => removeWaypoint(index)}><X size={14} /></button></span>
          </li>;
          })}
        </ol>
      </aside>
      <section className={`panel course-planner-map-panel${mapFullscreen ? " course-planner-map-fullscreen" : ""}`} aria-label="Course route map">
        <div className="course-planner-map-heading"><div><div className="panel-heading">Route</div><span className="muted">Click to add; drag numbered waypoints to adjust.</span></div><div className="course-planner-map-actions">{routed.isFetching && <span className="muted">Routing…</span>}<button className="secondary-button small-button" type="button" aria-label={mapFullscreen ? "Exit fullscreen map" : "Enter fullscreen map"} aria-pressed={mapFullscreen} onClick={() => setMapFullscreen((current) => !current)}>{mapFullscreen ? <Minimize2 size={14} /> : <Maximize2 size={14} />}{mapFullscreen ? "Exit fullscreen" : "Fullscreen"}</button></div></div>
        <CoursePlannerMap legs={plannerLegs} waypoints={waypoints} tileURL={mapTileURL} canEdit={canWrite} fullscreen={mapFullscreen} highlighted={highlighted ? [highlighted.latitude, highlighted.longitude] : undefined} onAdd={addWaypoint} onMove={moveWaypoint} />
      </section>
    </section>
    {plannerLegs.length > 0 && <section className="course-planner-elevation-preview">
      <section className="metric-grid course-planner-elevation-metrics">
        <Metric label="Distance" value={formatDistance(routeDistanceM)} icon={<RouteIcon size={18} />} />
        <Metric label="Ascent" value={formatCourseElevation(elevationGainM)} icon={<Mountain size={18} />} />
        <Metric label="Descent" value={formatCourseElevation(elevationLossM)} icon={<ArrowDown size={18} />} />
      </section>
      {elevationCoverage !== undefined && elevationCoverage < 0.9995 && <CourseElevationCoverageNotice coverage={elevationCoverage} />}
      <CourseElevationProfile profile={elevationProfile} onHighlight={setHighlighted} emptyMessage={routed.isFetching ? "Elevation is being calculated with the route." : "The planned route does not contain enough usable elevation data."} />
    </section>}
    {plannerLegs.length > 0 && <section className="panel course-leg-panel"><div className="course-leg-heading"><div><div className="panel-heading">Legs</div><span className="muted">Routing failures are isolated; direct legs stay editable and visible.</span></div>{routed.error && <button className="secondary-button small-button" type="button" onClick={() => void routed.refetch()}><RefreshCw size={14} />Retry routing</button>}</div><div className="course-leg-list">{plannerLegs.map((leg, index) => {
      const manuallyDirect = directLegIndexes.includes(index);
      return <div className={`course-leg-row ${leg.mode === "direct" ? "direct" : ""}`} key={index}><span className="course-leg-index">{index + 1}</span><span><strong>{leg.mode === "direct" ? "Direct" : "Routed"}</strong><small>{formatDistance(courseLegDistance([leg]))} · {leg.pointCount.toLocaleString()} points</small>{leg.warning && <small className="warning-text">{leg.warning}</small>}</span><button className="secondary-button small-button" type="button" disabled={!canWrite || routed.isFetching} onClick={() => { if (leg.mode === "direct" && !manuallyDirect) { void routed.refetch(); } else { setDirect(index, !manuallyDirect); } }}>{leg.mode === "direct" ? manuallyDirect ? "Try routing" : "Retry routing" : "Use direct"}</button></div>;
    })}</div></section>}
  </Page>;
}

function CoursePlannerMap({ legs, waypoints, tileURL, canEdit, fullscreen, highlighted, onAdd, onMove }: { legs: CourseLeg[]; waypoints: CourseWaypoint[]; tileURL?: string; canEdit: boolean; fullscreen: boolean; highlighted?: RoutePoint; onAdd: (point: RoutePoint) => void; onMove: (index: number, point: RoutePoint) => void }) {
  const pointsByLeg = legs.map((leg) => decodeCoursePolyline(leg.encodedPolyline));
  const waypointPoints = waypoints.map((point) => [point.latitude, point.longitude] as RoutePoint);
  const allPoints = pointsByLeg.flat();
  const fitPoints = allPoints.length > 0 ? allPoints : waypointPoints;
  const center = fitPoints[0] ?? [53.3498, -6.2603] as RoutePoint;
  const [position, setPosition] = useState<{ point: RoutePoint; accuracy: number }>();
  const [locationError, setLocationError] = useState("");
  const [locating, setLocating] = useState(false);
  const locate = () => {
    setLocationError("");
    setLocating(true);
    if (!navigator.geolocation) { setLocating(false); setLocationError("Location is not available in this browser."); return; }
    navigator.geolocation.getCurrentPosition((value) => { setPosition({ point: [value.coords.latitude, value.coords.longitude], accuracy: value.coords.accuracy }); setLocating(false); }, (error) => { setLocationError(error.message || "Could not get your location."); setLocating(false); }, { enableHighAccuracy: true, maximumAge: 0, timeout: 15000 });
  };
  return <div className="map-frame course-map-frame course-planner-map">
    <MapContainer center={center} zoom={13} scrollWheelZoom className="route-map">
      <TileLayer attribution="&copy; OpenStreetMap contributors" url={tileURL || "https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"} />
      {pointsByLeg.map((points, index) => points.length > 1 && <Polyline key={index} positions={points} pathOptions={{ color: legs[index].mode === "direct" ? "#aa5b38" : "#d85c41", weight: 5, dashArray: legs[index].mode === "direct" ? "8 8" : undefined }} />)}
      {waypoints.map((waypoint, index) => {
        const customName = waypoint.name?.trim();
        const markerName = customName || defaultCourseWaypointName(index, waypoints.length);
        return <Marker key={waypoint.index} position={[waypoint.latitude, waypoint.longitude]} icon={courseWaypointIcon(index + 1)} draggable={canEdit} title={markerName} eventHandlers={canEdit ? { dragend: (event) => { const value = (event.target as { getLatLng: () => { lat: number; lng: number } }).getLatLng(); onMove(index, [value.lat, value.lng]); } } : undefined}>
          {customName && <LeafletTooltip permanent interactive direction="top" offset={[0, -12]} opacity={1} className="course-waypoint-map-tooltip"><span className="course-waypoint-map-label" tabIndex={0} title={customName} aria-label={`Waypoint ${index + 1}: ${customName}`}>{customName}</span></LeafletTooltip>}
        </Marker>;
      })}
      {canEdit && <MapLocationPicker onSelect={onAdd} />}
      {highlighted && <Marker position={highlighted} icon={routeHighlightIcon()} interactive={false} keyboard={false} zIndexOffset={1000} />}
      {position && <><CenterMapOnPoint point={position.point} /><Circle center={position.point} radius={position.accuracy} pathOptions={{ color: "#2f6df6", fillColor: "#2f6df6", fillOpacity: 0.12, weight: 1 }} /><Marker position={position.point} icon={courseLocationIcon()} title="Current location" /></>}
      <FitMapContentOnce points={fitPoints} />
      <ResizeMapOnFullscreenChange fullscreen={fullscreen} />
    </MapContainer>
    <button className="secondary-button small-button course-locate-button" type="button" disabled={locating} onClick={locate}><LocateFixed size={15} />{locating ? "Locating…" : "Current location"}</button>
    {locationError && <div className="row-error course-location-error">{locationError}</div>}
    {(!tileURL || tileURL.includes("tile.openstreetmap.org")) && <p className="muted map-privacy-warning">Map tiles are loaded from OpenStreetMap; your browser and approximate route location are visible to that provider.</p>}
  </div>;
}

function defaultCourseWaypointName(index: number, count: number) {
  if (index === 0) return "Start";
  if (index === count - 1) return "Finish";
  return `Waypoint ${index + 1}`;
}

function ResizeMapOnFullscreenChange({ fullscreen }: { fullscreen: boolean }) {
  const map = useMap();
  useEffect(() => {
    const frame = window.requestAnimationFrame(() => map.invalidateSize({ pan: false }));
    return () => window.cancelAnimationFrame(frame);
  }, [fullscreen, map]);
  return null;
}

function plannerDirectLegs(waypoints: Array<{ latitude: number; longitude: number }>, warning: string): CourseRoutingLeg[] {
  return waypoints.slice(1).map((end, index) => {
    const start = waypoints[index];
    const points: RoutePoint[] = [[start.latitude, start.longitude], [end.latitude, end.longitude]];
    return { index, mode: "direct", encodedPolyline: encodeCoursePolyline(points, 6), elevationsM: [null, null], pointCount: 2, warning };
  });
}

function plannerReturnsToStart(waypoints: Array<{ latitude: number; longitude: number }>) {
  if (waypoints.length < 2) return false;
  const start = waypoints[0];
  const finish = waypoints[waypoints.length - 1];
  return routePointDistance([start.latitude, start.longitude], [finish.latitude, finish.longitude]) <= 0.5;
}

function encodeCoursePolyline(points: RoutePoint[], precision: number) {
  const factor = 10 ** precision;
  let previousLatitude = 0;
  let previousLongitude = 0;
  let encoded = "";
  for (const [latitude, longitude] of points) {
    const nextLatitude = Math.round(latitude * factor);
    const nextLongitude = Math.round(longitude * factor);
    encoded += encodePolylineValue(nextLatitude - previousLatitude);
    encoded += encodePolylineValue(nextLongitude - previousLongitude);
    previousLatitude = nextLatitude;
    previousLongitude = nextLongitude;
  }
  return encoded;
}

function encodePolylineValue(value: number) {
  let shifted = value < 0 ? ~(value << 1) : value << 1;
  let encoded = "";
  while (shifted >= 0x20) {
    encoded += String.fromCharCode((0x20 | (shifted & 0x1f)) + 63);
    shifted >>>= 5;
  }
  return encoded + String.fromCharCode(shifted + 63);
}

function courseLegDistance(legs: Array<Pick<CourseLeg, "encodedPolyline">>) {
  return legs.reduce((total, leg) => {
    const points = decodeCoursePolyline(leg.encodedPolyline);
    for (let index = 1; index < points.length; index++) total += routePointDistance(points[index - 1], points[index]);
    return total;
  }, 0);
}

function routePointDistance(start: RoutePoint, end: RoutePoint) {
  const radians = (value: number) => value * Math.PI / 180;
  const latitude = radians(end[0] - start[0]);
  const longitude = radians(end[1] - start[1]);
  const value = Math.sin(latitude / 2) ** 2 + Math.cos(radians(start[0])) * Math.cos(radians(end[0])) * Math.sin(longitude / 2) ** 2;
  return 6371000 * 2 * Math.atan2(Math.sqrt(value), Math.sqrt(1 - value));
}

function courseWaypointIcon(number: number) {
  return divIcon({ className: "course-waypoint-marker-icon", html: `<span class="course-waypoint-marker">${number}</span>`, iconSize: [28, 28], iconAnchor: [14, 14] });
}

type CourseImportDraft = CourseImportSelection & { selected: boolean };

function CourseImportPage({ canWrite, mapTileURL }: { canWrite: boolean; mapTileURL?: string }) {
  const navigate = useNavigate();
  const [file, setFile] = useState<File>();
  const [preview, setPreview] = useState<CourseImportPreview>();
  const [drafts, setDrafts] = useState<Record<string, CourseImportDraft>>({});
  const [activeKey, setActiveKey] = useState<string>();
  const previewImport = useMutation({
    mutationFn: api.previewCourseImport,
    onSuccess: (result) => {
      const next: Record<string, CourseImportDraft> = {};
      for (const candidate of result.candidates) {
        next[candidate.key] = { key: candidate.key, name: candidate.name, sportType: candidate.sportType ?? "Run", notes: "", selected: candidate.valid && !candidate.duplicateCourse };
      }
      setPreview(result);
      setDrafts(next);
      setActiveKey(result.candidates.find((candidate) => candidate.valid)?.key ?? result.candidates[0]?.key);
    }
  });
  const commit = useMutation({
    mutationFn: () => api.commitCourseImport(file!, preview!.fileSHA256, Object.values(drafts).filter((draft) => draft.selected).map(({ selected: _selected, ...selection }) => selection)),
    onSuccess: (result) => navigate(`/courses/imports/${encodeURIComponent(result.importId)}`)
  });
  const candidates = preview?.candidates ?? [];
  const active = candidates.find((candidate) => candidate.key === activeKey);
  const activeDraft = active ? drafts[active.key] : undefined;
  const selectionCount = Object.values(drafts).filter((draft) => draft.selected).length;
  const selectFile = (next: File | undefined) => {
    setFile(next);
    setPreview(undefined);
    setDrafts({});
    setActiveKey(undefined);
    previewImport.reset();
    commit.reset();
    if (next) previewImport.mutate(next);
  };
  const updateDraft = (key: string, changes: Partial<CourseImportDraft>) => setDrafts((current) => ({ ...current, [key]: { ...current[key], ...changes } }));

  return (
    <Page title="Upload GPX" eyebrow="Course import" actions={<Link className="secondary-button" to="/courses"><ChevronLeft size={16} />Courses</Link>}>
      {!canWrite && <div className="error">Course imports are disabled in read-only support mode.</div>}
      <section className="panel course-upload-panel">
        <label className="course-file-input"><FileUp size={22} /><span><strong>Choose a GPX file</strong><small>Up to 10 MB. Tracks, track segments, and routes are reviewed before import.</small></span><input type="file" accept=".gpx,application/gpx+xml,application/xml,text/xml" disabled={!canWrite || previewImport.isPending || commit.isPending} onChange={(event) => selectFile(event.target.files?.[0])} /></label>
        {file && <span className="muted">{file.name} · {formatFileSize(file.size)}</span>}
        {previewImport.isPending && <LoadingRow />}
        {previewImport.error && <div className="error">{courseMutationMessage(previewImport.error)}</div>}
      </section>
      {preview && <>
        {(preview.diagnostics ?? []).length > 0 && <CourseImportDiagnostics diagnostics={preview.diagnostics ?? []} title="File warnings" />}
        <section className="course-import-layout">
          <section className="panel course-candidate-panel">
            <div className="panel-heading">Segments <span className="muted">{selectionCount} selected</span></div>
            <div className="course-candidate-list">
              {candidates.map((candidate) => {
                const draft = drafts[candidate.key];
                const selectable = candidate.valid && !candidate.duplicateCourse;
                return <div key={candidate.key} className={`course-candidate-row ${activeKey === candidate.key ? "active" : ""} ${!candidate.valid ? "invalid" : ""}`}>
                  <input type="checkbox" aria-label={`Import ${candidate.name}`} checked={draft?.selected ?? false} disabled={!selectable || commit.isPending} onChange={(event) => updateDraft(candidate.key, { selected: event.target.checked })} />
                  <button type="button" onClick={() => setActiveKey(candidate.key)}>
                    <strong>{candidate.name}</strong>
                    <span>{candidate.valid ? `${candidate.kind} · ${formatDistance(candidate.distanceM ?? 0)} · ${(candidate.pointCount ?? 0).toLocaleString()} source samples · ${(candidate.waypointCount ?? 0).toLocaleString()} editable points` : candidate.error}</span>
                    {candidate.duplicateCourse && <span className="warning-text">Already saved as {candidate.duplicateCourse.name}</span>}
                  </button>
                </div>;
              })}
            </div>
          </section>
          <section className="course-import-review">
            {!active && <section className="panel"><EmptyState title="Select a segment to review" /></section>}
            {active && <>
              <section className="panel">
                <div className="panel-heading">Preview</div>
                {active.encodedPolyline ? <CourseMap legs={[courseImportPreviewLeg(active)]} tileURL={mapTileURL} /> : <EmptyState title="No valid route to preview" message={active.error} />}
              </section>
              {active.profile && active.profile.length > 0 && <CourseElevationProfile profile={active.profile} />}
              {activeDraft && active.valid && <section className="panel course-import-fields"><div className="panel-heading">Imported course details</div><CourseDetailsFields name={activeDraft.name} sportType={activeDraft.sportType} notes={activeDraft.notes} onName={(value) => updateDraft(active.key, { name: value })} onSport={(value) => updateDraft(active.key, { sportType: value })} onNotes={(value) => updateDraft(active.key, { notes: value })} /></section>}
              {(active.diagnostics ?? []).length > 0 && <CourseImportDiagnostics diagnostics={active.diagnostics ?? []} title="Segment warnings" />}
            </>}
          </section>
        </section>
        {commit.error && <div className="error">{courseMutationMessage(commit.error)}</div>}
        <div className="course-import-actions"><Link className="secondary-button" to="/courses">Cancel</Link><button className="primary-button" type="button" disabled={!canWrite || commit.isPending || selectionCount === 0 || Object.values(drafts).some((draft) => draft.selected && !draft.name.trim())} onClick={() => commit.mutate()}>{commit.isPending ? "Importing…" : `Import ${selectionCount} ${selectionCount === 1 ? "course" : "courses"}`}</button></div>
      </>}
    </Page>
  );
}

function CourseImportResultPage() {
  const { id } = useParams();
  const result = useQuery({ queryKey: ["course-import", id], queryFn: () => api.courseImport(id!), enabled: Boolean(id) });
  if (result.isLoading) return <Page title="Import result"><LoadingRow /></Page>;
  if (!result.data) return <Page title="Import result"><EmptyState title="Import not found" /></Page>;
  return <Page title="Import complete" eyebrow={result.data.filename} actions={<Link className="primary-button" to="/courses">View courses</Link>}>
    <section className="panel"><div className="panel-heading">Created {result.data.created.length} {result.data.created.length === 1 ? "course" : "courses"}</div>
      {result.data.created.length === 0 ? <EmptyState title="No courses were created" /> : <div className="course-import-created-list">{result.data.created.map((course) => <Link key={course.id} to={`/courses/${encodeURIComponent(course.id)}`}><strong>{course.name}</strong><span>{course.sportType} · {formatDistance(course.distanceM)}</span><ChevronRight size={17} /></Link>)}</div>}
    </section>
    {(result.data.diagnostics ?? []).length > 0 && <CourseImportDiagnostics diagnostics={result.data.diagnostics ?? []} title="Import warnings" />}
  </Page>;
}

function CourseImportDiagnostics({ diagnostics, title }: { diagnostics: Array<{ code: string; message: string; count?: number }>; title: string }) {
  return <section className="panel course-import-diagnostics"><div className="panel-heading">{title}</div>{diagnostics.map((item, index) => <div key={`${item.code}-${index}`}><strong>{courseDiagnosticLabel(item.code)}</strong><span>{item.message}{item.count ? ` (${item.count})` : ""}</span></div>)}</section>;
}

function CourseMap({ legs, tileURL, highlighted, allowLocation = false }: { legs: CourseLeg[]; tileURL?: string; highlighted?: RoutePoint; allowLocation?: boolean }) {
  const pointsByLeg = legs.map((leg) => decodeCoursePolyline(leg.encodedPolyline));
  const allPoints = pointsByLeg.flat();
  const center = allPoints[0] ?? [53.3498, -6.2603] as RoutePoint;
  const [position, setPosition] = useState<{ point: RoutePoint; accuracy: number }>();
  const [locationError, setLocationError] = useState("");
  const [locating, setLocating] = useState(false);
  const locate = () => {
    setLocationError("");
    setLocating(true);
    if (!navigator.geolocation) {
      setLocating(false);
      setLocationError("Location is not available in this browser.");
      return;
    }
    navigator.geolocation.getCurrentPosition(
      (value) => { setPosition({ point: [value.coords.latitude, value.coords.longitude], accuracy: value.coords.accuracy }); setLocating(false); },
      (error) => { setLocationError(error.message || "Could not get your location."); setLocating(false); },
      { enableHighAccuracy: true, maximumAge: 0, timeout: 15000 }
    );
  };
  return <div className="map-frame course-map-frame">
    <MapContainer center={center} zoom={13} scrollWheelZoom className="route-map">
      <TileLayer attribution="&copy; OpenStreetMap contributors" url={tileURL || "https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"} />
      {pointsByLeg.map((points, index) => points.length > 1 && <Polyline key={legs[index].id ?? index} positions={points} pathOptions={{ color: legs[index].mode === "direct" ? "#aa5b38" : "#d85c41", weight: 5, dashArray: legs[index].mode === "direct" ? "8 8" : undefined }} />)}
      {allPoints[0] && <Marker position={allPoints[0]} icon={routeEndpointIcon("start")} interactive={false} keyboard={false} />}
      {allPoints.length > 1 && <Marker position={allPoints[allPoints.length - 1]} icon={routeEndpointIcon("end")} interactive={false} keyboard={false} />}
      {highlighted && <Marker position={highlighted} icon={routeHighlightIcon()} interactive={false} keyboard={false} zIndexOffset={1000} />}
      {position && <><CenterMapOnPoint point={position.point} /><Circle center={position.point} radius={position.accuracy} pathOptions={{ color: "#2f6df6", fillColor: "#2f6df6", fillOpacity: 0.12, weight: 1 }} /><Marker position={position.point} icon={courseLocationIcon()} title="Current location" /></>}
      <FitMapContent points={allPoints} />
    </MapContainer>
    {allowLocation && <button className="secondary-button small-button course-locate-button" type="button" disabled={locating} onClick={locate}><LocateFixed size={15} />{locating ? "Locating…" : "Current location"}</button>}
    {locationError && <div className="row-error course-location-error">{locationError}</div>}
    {(!tileURL || tileURL.includes("tile.openstreetmap.org")) && <p className="muted map-privacy-warning">Map tiles are loaded from OpenStreetMap; your browser and approximate route location are visible to that provider.</p>}
  </div>;
}

function CourseElevationProfile({ profile, onHighlight, emptyMessage = "The source did not contain enough usable elevation data." }: { profile: CourseProfilePoint[]; onHighlight?: (point?: CourseProfilePoint) => void; emptyMessage?: string }) {
  const data = profile.filter((point) => point.elevationM !== undefined).map((point) => ({ ...point, distanceKm: point.distanceM / 1000 }));
  return <section className="panel course-profile-panel"><div className="panel-heading">Elevation profile</div>
    {data.length < 2 ? <EmptyState title="No elevation profile" message={emptyMessage} /> : <div className="course-profile-chart"><ResponsiveContainer width="100%" height="100%"><AreaChart data={data} onMouseMove={(state) => onHighlight?.(courseProfilePointFromMouseState(state, data))} onMouseLeave={() => onHighlight?.(undefined)}><CartesianGrid strokeDasharray="3 3" vertical={false} /><XAxis dataKey="distanceKm" tickFormatter={(value) => `${Number(value).toFixed(1)} km`} minTickGap={24} /><YAxis width={50} tickFormatter={(value) => `${Math.round(Number(value))} m`} domain={["dataMin", "dataMax"]} /><Tooltip contentStyle={chartTooltipContentStyle} labelFormatter={(value) => `${Number(value).toFixed(2)} km`} formatter={(value) => [`${Math.round(Number(value))} m`, "Elevation"]} /><Area type="monotone" dataKey="elevationM" stroke="#b7791f" fill="#f6c432" fillOpacity={0.45} dot={false} /></AreaChart></ResponsiveContainer></div>}
  </section>;
}

function CourseElevationCoverageNotice({ coverage }: { coverage: number }) {
  return <div className="course-elevation-coverage-notice"><strong>Incomplete elevation data.</strong> Elevation covers {Math.round(coverage * 100)}% of this route, so ascent and descent may be understated.</div>;
}

function courseProfilePointFromMouseState(state: unknown, data: CourseProfilePoint[]) {
  if (!state || typeof state !== "object" || !("activeTooltipIndex" in state)) return undefined;
  const raw = (state as { activeTooltipIndex?: unknown }).activeTooltipIndex;
  const index = typeof raw === "number" ? raw : typeof raw === "string" ? Number(raw) : -1;
  return Number.isInteger(index) && index >= 0 && index < data.length ? data[index] : undefined;
}

function courseImportPreviewLeg(candidate: CourseImportCandidate): CourseLeg {
  return { index: 0, mode: candidate.kind === "route" ? "direct" : "preserved", encodedPolyline: candidate.encodedPolyline ?? "", elevationsM: [], pointCount: candidate.pointCount ?? 0 };
}

function decodeCoursePolyline(encoded: string): RoutePoint[] {
  return decodePolylineWithPrecision(encoded, 6);
}

function courseLocationIcon() {
  return divIcon({ className: "course-location-marker-icon", html: '<span class="course-location-marker"></span>', iconSize: [18, 18], iconAnchor: [9, 9] });
}

function formatCourseElevation(value?: number) {
  return value === undefined ? "" : `${Math.round(value).toLocaleString()} m`;
}

function courseDiagnosticLabel(value: string) {
  return value.replace(/([a-z])([A-Z])/g, "$1 $2").replace(/[_-]+/g, " ").replace(/^./, (letter) => letter.toUpperCase());
}

function formatCourseDiagnostic(value: unknown) {
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") return String(value);
  if (Array.isArray(value)) return value.map((item) => typeof item === "object" && item && "message" in item ? String((item as { message: unknown }).message) : String(item)).join("; ");
  return JSON.stringify(value);
}

function courseMutationMessage(error: unknown) {
  return error instanceof Error ? error.message : "The course operation failed";
}

const workoutFilterOptions = [
  { id: "upcoming", label: "Upcoming" },
  { id: "drafts", label: "Drafts" },
  { id: "attention", label: "Needs attention" },
  { id: "excluded", label: "Excluded" },
  { id: "past", label: "Past" }
] as const;

function WorkoutsPage() {
  const [filter, setFilter] = useState<(typeof workoutFilterOptions)[number]["id"]>("upcoming");
  const queryClient = useQueryClient();
  const workouts = useQuery({ queryKey: ["workouts", filter], queryFn: () => api.workouts(filter) });
  const config = useQuery({ queryKey: ["workout-config"], queryFn: api.workoutConfig });
  const jobs = useQuery({ queryKey: ["sync-jobs"], queryFn: api.syncJobs, refetchInterval: 2000 });
  const latestJob = (jobs.data?.jobs ?? []).find((job) => job.provider === "garmin" && job.kind === "workouts");
  const reconcile = useMutation({
    mutationFn: api.reconcileWorkouts,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["sync-jobs"] }),
        queryClient.invalidateQueries({ queryKey: ["workouts"] })
      ]);
    }
  });
  const items = workouts.data?.workouts ?? [];

  return (
    <Page
      title="Workouts"
      eyebrow="Structured running sessions"
      actions={<div className="workout-page-actions">
        <button className="secondary-button small-button" type="button" disabled={!config.data?.syncEnabled || reconcile.isPending || latestJob?.status === "running"} onClick={() => reconcile.mutate()}>
          <RefreshCw size={15} />
          {latestJob?.status === "running" ? "Syncing…" : "Sync Garmin"}
        </button>
        <Link className="primary-button small-button" to="/workouts/new">New workout</Link>
      </div>}
    >
      <section className="panel workout-list-panel">
        <div className="workout-filter-tabs" role="tablist" aria-label="Workout filters">
          {workoutFilterOptions.map((option) => (
            <button key={option.id} type="button" className={filter === option.id ? "active" : ""} onClick={() => setFilter(option.id)}>{option.label}</button>
          ))}
        </div>
        {!config.isLoading && !config.data?.syncEnabled && <div className="workout-notice">Garmin workout sync is off. You can build and inspect workouts without changing Garmin.</div>}
        {latestJob?.status === "failed" && <div className="error">Garmin workout sync failed: {latestJob.error || "See sync details in Settings."}</div>}
        {reconcile.error && <div className="error">{reconcile.error instanceof Error ? reconcile.error.message : "Could not start Garmin workout sync"}</div>}
        {workouts.isLoading && <LoadingRow />}
        {workouts.error && <div className="error">{workouts.error instanceof Error ? workouts.error.message : "Could not load workouts"}</div>}
        {!workouts.isLoading && !workouts.error && items.length === 0 && <div className="empty-state">No workouts in this view.</div>}
        {items.length > 0 && (
          <div className="workout-list">
            {items.map((workout) => <WorkoutListRow key={workout.id} workout={workout} />)}
          </div>
        )}
      </section>
    </Page>
  );
}

function WorkoutListRow({ workout }: { workout: Workout }) {
  const state = workout.garminExcluded
    ? { label: "Excluded", className: "muted" }
    : workout.parseStatus === "error"
      ? { label: "Parse error", className: "failed" }
      : workout.garmin.error
        ? { label: "Ownership conflict", className: "failed" }
        : workout.garmin.status === "scheduled"
          ? { label: "Scheduled", className: "completed" }
          : { label: workout.scheduledDate ? "Pending" : "Draft", className: "queued" };
  return (
    <Link className="workout-list-row" to={`/workouts/${workout.id}`}>
      <div className="workout-list-date">
        <strong>{workout.scheduledDate ? workout.scheduledDate.slice(8, 10) : "—"}</strong>
        <span>{workout.scheduledDate ? formatCalendarAgendaDate(workout.scheduledDate).replace(/^\w+,?\s*/, "") : "Unscheduled"}</span>
      </div>
      <div className="workout-list-main">
        <strong>{workout.name}</strong>
        <span>{workoutDefinitionSummaryText(workout.definition)} · {workout.source === "training_sheet" ? "Training sheet" : "Manual"}</span>
        {workout.garmin.error && <span className="workout-row-error">{workout.garmin.error}</span>}
      </div>
      <span className={`status ${state.className}`}>{state.label}</span>
    </Link>
  );
}

function WorkoutEditorPage() {
  const { id } = useParams();
  const creating = !id;
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const workout = useQuery({ queryKey: ["workout", id], queryFn: () => api.workout(id ?? ""), enabled: !creating });
  const config = useQuery({ queryKey: ["workout-config"], queryFn: api.workoutConfig });
  const [loadedID, setLoadedID] = useState("");
  const [name, setName] = useState("Running workout");
  const [sourceText, setSourceText] = useState("");
  const [scheduledDate, setScheduledDate] = useState("");
  const [useDefaultTolerance, setUseDefaultTolerance] = useState(true);
  const [paceTolerance, setPaceTolerance] = useState(0);
  const [garminExcluded, setGarminExcluded] = useState(false);

  useEffect(() => {
    const item = workout.data;
    if (!item || loadedID === item.id) {
      return;
    }
    setLoadedID(item.id);
    setName(item.name);
    setSourceText(item.sourceText ?? "");
    setScheduledDate(item.scheduledDate ?? "");
    setUseDefaultTolerance(item.paceToleranceSeconds === undefined);
    setPaceTolerance(item.paceToleranceSeconds ?? config.data?.defaultPaceToleranceSeconds ?? 0);
    setGarminExcluded(item.garminExcluded);
  }, [config.data?.defaultPaceToleranceSeconds, loadedID, workout.data]);

  useEffect(() => {
    if (creating && config.data && useDefaultTolerance) {
      setPaceTolerance(config.data.defaultPaceToleranceSeconds);
    }
  }, [config.data, creating, useDefaultTolerance]);

  useEffect(() => {
    if (searchParams.get("section") !== "garmin" || !workout.data) return;
    const timeout = window.setTimeout(() => document.getElementById("garmin")?.scrollIntoView({ behavior: "smooth", block: "center" }), 0);
    return () => window.clearTimeout(timeout);
  }, [searchParams, workout.data]);

  const parse = useMutation({ mutationFn: api.parseWorkout });
  const save = useMutation({
    mutationFn: async () => {
      const body: WorkoutMutation = {
        paceToleranceSeconds: useDefaultTolerance ? undefined : paceTolerance,
        useDefaultPaceTolerance: useDefaultTolerance,
        garminExcluded
      };
      if (creating || workout.data?.source === "manual") {
        body.name = name.trim();
        body.scheduledDate = scheduledDate;
        if (creating || sourceText !== (workout.data?.sourceText ?? "")) {
          body.sourceText = sourceText;
        }
      }
      return creating ? api.createWorkout(body) : api.updateWorkout(id ?? "", body);
    },
    onSuccess: async (saved) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["workouts"] }),
        queryClient.invalidateQueries({ queryKey: ["workout", saved.id] }),
        queryClient.invalidateQueries({ queryKey: ["activity-calendar"] }),
        queryClient.invalidateQueries({ queryKey: ["calendar-day"] })
      ]);
      navigate(`/workouts/${saved.id}`, { replace: creating });
    }
  });
  const duplicate = useMutation({
    mutationFn: () => api.duplicateWorkout(id ?? ""),
    onSuccess: async (copy) => {
      await queryClient.invalidateQueries({ queryKey: ["workouts"] });
      navigate(`/workouts/${copy.id}`);
    }
  });
  const remove = useMutation({
    mutationFn: () => api.deleteWorkout(id ?? ""),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["workouts"] });
      navigate("/workouts");
    }
  });

  if (!creating && workout.isLoading) {
    return <Page title="Workout"><LoadingRow /></Page>;
  }
  if (!creating && workout.error) {
    return <Page title="Workout"><div className="error">{workout.error instanceof Error ? workout.error.message : "Could not load workout"}</div></Page>;
  }

  const current = workout.data;
  const readOnly = current?.source === "training_sheet";
  const parsedCurrent = parse.data && parse.variables === sourceText ? parse.data : undefined;
  const definition = parsedCurrent?.definition ?? current?.definition;
  const parseStatus = parsedCurrent?.status ?? current?.parseStatus;
  const parseMessages = parsedCurrent?.messages ?? current?.parseMessages ?? [];
  const invalid = creating && sourceText.trim() === "";

  return (
    <Page
      title={creating ? "New workout" : current?.name ?? "Workout"}
      eyebrow={readOnly ? "Generated from training sheet" : creating ? "Manual workout" : "Manual workout"}
      actions={<div className="workout-page-actions">
        <Link className="secondary-button small-button" to="/workouts"><ChevronLeft size={15} />Back</Link>
        {!creating && <button className="secondary-button small-button" type="button" disabled={duplicate.isPending} onClick={() => duplicate.mutate()}>Duplicate</button>}
        {!creating && !readOnly && <button className="danger-button small-button" type="button" disabled={remove.isPending} onClick={() => {
          if (window.confirm("Delete this workout? Runnarr will safely unschedule only its own verified Garmin calendar entry.")) {
            remove.mutate();
          }
        }}>Delete</button>}
        <button className="primary-button small-button" type="button" disabled={save.isPending || invalid} onClick={() => save.mutate()}>{save.isPending ? "Saving…" : "Save"}</button>
      </div>}
    >
      {readOnly && <div className="workout-notice">Structure and date follow the training sheet. Duplicate this workout to create an editable manual copy.</div>}
      {current?.garmin.error && <div className="error"><strong>Garmin ownership conflict.</strong> {current.garmin.error} Runnarr did not modify the remote workout.</div>}
      <div className="workout-editor-layout">
        <section className="panel workout-editor-form">
          <div className="panel-heading">Workout</div>
          <label className="field"><span>Name</span><input value={name} disabled={readOnly} maxLength={160} onChange={(event) => setName(event.target.value)} /></label>
          <label className="field"><span>Date</span><input type="date" value={scheduledDate} disabled={readOnly} onChange={(event) => setScheduledDate(event.target.value)} /></label>
          <label className="field"><span>Prescription</span><textarea rows={7} value={sourceText} disabled={readOnly} placeholder="15mins warm up//5x7mins@3:35(2mins)//15mins cool down" onChange={(event) => setSourceText(event.target.value)} /></label>
          {!readOnly && <button className="secondary-button small-button workout-parse-button" type="button" disabled={!sourceText.trim() || parse.isPending} onClick={() => parse.mutate(sourceText)}>{parse.isPending ? "Parsing…" : "Preview prescription"}</button>}
          <div className="workout-operation-grid">
            <label className="checkbox-field"><input type="checkbox" checked={useDefaultTolerance} onChange={(event) => setUseDefaultTolerance(event.target.checked)} /> Use default pace tolerance</label>
            <label className="field"><span>Pace tolerance (seconds)</span><input type="number" min={0} max={60} disabled={useDefaultTolerance} value={paceTolerance} onChange={(event) => setPaceTolerance(Number(event.target.value))} /></label>
            <label className="checkbox-field"><input type="checkbox" checked={garminExcluded} onChange={(event) => setGarminExcluded(event.target.checked)} /> Do not send this workout to Garmin</label>
          </div>
          {(save.error || duplicate.error || remove.error || parse.error) && <div className="error">{mutationErrorMessage(save.error || duplicate.error || remove.error || parse.error, "Could not update workout")}</div>}
        </section>
        <section className="panel workout-preview-panel">
          <div className="filter-header"><div className="panel-heading">Steps</div>{parseStatus && <span className={`status ${parseStatus === "error" ? "failed" : parseStatus === "warning" ? "canceled" : "completed"}`}>{parseStatus}</span>}</div>
          {definition ? <WorkoutDefinitionView definition={definition} /> : <div className="muted">Enter a prescription to preview its steps.</div>}
          {parseMessages.length > 0 && <ul className="workout-parse-messages">{parseMessages.map((message, index) => <li key={`${message.message}-${index}`} className={message.level}>{message.message}</li>)}</ul>}
          <div id="garmin" className="workout-garmin-summary">
            <strong>Garmin</strong>
            <span>{garminExcluded ? "Excluded" : current?.garmin.status || (scheduledDate ? "Pending sync" : "Not scheduled")}</span>
            {current?.garmin.providerScheduleId && <span>Calendar ID {current.garmin.providerScheduleId}</span>}
          </div>
        </section>
      </div>
    </Page>
  );
}

function WorkoutDefinitionView({ definition }: { definition: WorkoutDefinition }) {
  return <div className="workout-step-list">
    {definition.steps.map((step) => <WorkoutStepView key={`${step.order}-${step.kind}`} step={step} />)}
    {definition.estimatedDurationS > 0 && <div className="workout-duration">Estimated duration: {formatDuration(definition.estimatedDurationS)}</div>}
  </div>;
}

function WorkoutStepView({ step }: { step: WorkoutStep }) {
  if (step.kind === "repeat") {
    return <div className="workout-step repeat">
      <div className="workout-step-heading"><strong>{step.repeatCount}× repeat</strong>{step.skipLastRecovery && <span>Skip final recovery</span>}</div>
      <div className="workout-step-children">{(step.children ?? []).map((child) => <WorkoutStepView key={`${child.order}-${child.kind}`} step={child} />)}</div>
    </div>;
  }
  return <div className={`workout-step ${step.kind}`}>
    <strong>{workoutStepKindLabel(step.kind)}</strong>
    <span>{workoutStepConditionLabel(step)}</span>
    {step.target.type === "pace" && <span>{workoutPaceTargetLabel(step)}</span>}
    {step.description && <small>{step.description}</small>}
  </div>;
}

function workoutDefinitionSummaryText(definition: WorkoutDefinition) {
  const repeat = definition.steps.find((step) => step.kind === "repeat");
  const duration = definition.estimatedDurationS > 0 ? formatDuration(definition.estimatedDurationS) : "Open duration";
  return repeat ? `${repeat.repeatCount}× intervals · ${duration}` : duration;
}

function workoutStepKindLabel(kind: WorkoutStep["kind"]) {
  return ({ warmup: "Warm up", work: "Work", recovery: "Recovery", cooldown: "Cool down", repeat: "Repeat" } as const)[kind];
}

function workoutStepConditionLabel(step: WorkoutStep) {
  const condition = step.endCondition;
  if (!condition || condition.type === "lap_button") return "Lap button";
  if (condition.type === "distance") return condition.value && condition.value >= 1000 ? `${(condition.value / 1000).toLocaleString()} km` : `${condition.value ?? 0} m`;
  return formatDuration(Math.round(condition.value ?? 0));
}

function workoutPaceTargetLabel(step: WorkoutStep) {
  const target = step.target;
  if (target.paceFastSecondsPerKM && target.paceSlowSecondsPerKM) {
    return `${formatPace(target.paceFastSecondsPerKM)}–${formatPace(target.paceSlowSecondsPerKM)}`;
  }
  return target.paceSecondsPerKM ? formatPace(target.paceSecondsPerKM) : "";
}

function mutationErrorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

function SettingsPage({
  canWrite,
  defaultExperience,
  onDefaultExperienceChange,
  themePreference,
  onThemePreferenceChange,
  themePreferenceError
}: {
  canWrite: boolean;
  defaultExperience: "full" | "simple";
  onDefaultExperienceChange: (value: "full" | "simple") => Promise<UserPreference>;
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
  const [healthSyncFrom, setHealthSyncFrom] = useState("");
  const [garminAllData, setGarminAllData] = useState(false);
  const garminJobs = jobs.data?.jobs ?? [];
  const latestHealthJob = garminJobs.find((job) => job.provider === "garmin" && isHealthSyncJob(job));
  const latestGearJob = garminJobs.find((job) => job.provider === "garmin" && isGearSyncJob(job));
  const latestGarminJob = garminJobs.find((job) => job.provider === "garmin" && !isGearSyncJob(job) && !isHealthSyncJob(job));
  const anyGarminSyncRunning = garminJobs.some((job) => job.provider === "garmin" && job.status === "running");
  const garminSyncRunning = latestGarminJob?.status === "running";
  const healthSyncRunning = latestHealthJob?.status === "running";
  const gearSyncRunning = latestGearJob?.status === "running";
  const visibleSyncJobs = [latestHealthJob, latestGarminJob, latestGearJob]
    .filter((job): job is SyncJob => Boolean(job))
    .filter((job, index, list) => list.findIndex((item) => item.id === job.id) === index);

  useEffect(() => {
    const section = new URLSearchParams(location.search).get("section") || (location.hash === "#import" ? "import" : "");
    if (!section) {
      return;
    }
    const timeout = window.setTimeout(() => {
      document.getElementById(section)?.scrollIntoView({ block: "start" });
    });
    return () => window.clearTimeout(timeout);
  }, [location.hash, location.search]);

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
  const garminHealthSync = useMutation({
    mutationFn: api.garminHealthSync,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["sync-jobs"] }),
        queryClient.invalidateQueries({ queryKey: ["health-daily"] })
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
          <label className="compact-field">
            <span>Health from</span>
            <input type="date" value={healthSyncFrom} max={localDateString()} onChange={(event) => setHealthSyncFrom(event.target.value)} />
          </label>
          <button className="primary-button" type="button" disabled={!garminStatus.data?.connected || garminSync.isPending || anyGarminSyncRunning} onClick={() => garminSync.mutate({ oldest: garminAllData ? undefined : garminOldest, allData: garminAllData })}>
            <RefreshCw size={16} />
            {garminSyncRunning ? "Syncing" : "Sync"}
          </button>
          <button className="secondary-button" type="button" disabled={!garminStatus.data?.connected || garminHealthSync.isPending || anyGarminSyncRunning} onClick={() => garminHealthSync.mutate({ from: healthSyncFrom || undefined, to: localDateString() })}>
            <RefreshCw size={16} />
            {healthSyncRunning ? "Syncing health" : "Sync health"}
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
      {garminHealthSync.error && <div className="error">{garminHealthSync.error instanceof Error ? garminHealthSync.error.message : "Garmin health sync failed"}</div>}
      {garminGearSync.error && <div className="error">{garminGearSync.error instanceof Error ? garminGearSync.error.message : "Garmin gear sync failed"}</div>}
      <WorkoutSettings />
      <TrainingSheetSettings />
      <NotificationSettingsSection canWrite={canWrite} />
      <ClimbDetectionSettingsSection />
      <SimpleModeSettings defaultExperience={defaultExperience} onDefaultExperienceChange={onDefaultExperienceChange} />
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

function WorkoutSettings() {
  const queryClient = useQueryClient();
  const config = useQuery({ queryKey: ["workout-config"], queryFn: api.workoutConfig });
  const [loaded, setLoaded] = useState(false);
  const [syncEnabled, setSyncEnabled] = useState(false);
  const [tolerance, setTolerance] = useState(0);
  const [timezone, setTimezone] = useState("");

  useEffect(() => {
    if (!config.data || loaded) return;
    setLoaded(true);
    setSyncEnabled(config.data.syncEnabled);
    setTolerance(config.data.defaultPaceToleranceSeconds);
    setTimezone(config.data.timezone || browserCalendarTimezone());
  }, [config.data, loaded]);

  const save = useMutation({
    mutationFn: () => api.updateWorkoutConfig({
      syncEnabled,
      defaultPaceToleranceSeconds: tolerance,
      timezone: timezone.trim()
    }),
    onSuccess: async (saved) => {
      setSyncEnabled(saved.syncEnabled);
      setTolerance(saved.defaultPaceToleranceSeconds);
      setTimezone(saved.timezone);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["workout-config"] }),
        queryClient.invalidateQueries({ queryKey: ["sync-jobs"] })
      ]);
    }
  });

  return (
    <section id="workouts" className="panel workout-settings-panel">
      <div>
        <div className="panel-heading">Garmin workouts</div>
        <p className="muted">Build workouts in Runnarr and schedule the next {config.data?.horizonDays ?? 7} days in Garmin Connect.</p>
      </div>
      <div className="workout-settings-controls">
        <label className="checkbox-field"><input type="checkbox" checked={syncEnabled} onChange={(event) => setSyncEnabled(event.target.checked)} /> Enable Garmin workout scheduling</label>
        <label className="compact-field"><span>Default pace range (± seconds)</span><input type="number" min={0} max={60} value={tolerance} onChange={(event) => setTolerance(Number(event.target.value))} /></label>
        <label className="compact-field"><span>Workout timezone</span><input value={timezone} placeholder="Europe/Dublin" onChange={(event) => setTimezone(event.target.value)} /></label>
        <button className="primary-button small-button" type="button" disabled={save.isPending || config.isLoading} onClick={() => save.mutate()}>{save.isPending ? "Saving…" : "Save workout settings"}</button>
      </div>
      <p className="muted workout-ownership-note">Safety: Runnarr schedules, unschedules, and deletes only remote templates carrying this account’s exact Runnarr ownership marker. Other Garmin workouts are never modified.</p>
      {save.error && <div className="error">{save.error instanceof Error ? save.error.message : "Could not save workout settings"}</div>}
    </section>
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
    <details id="training-sheet" className="panel settings-advanced-details">
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

function SimpleModeSettings({
  defaultExperience,
  onDefaultExperienceChange
}: {
  defaultExperience: "full" | "simple";
  onDefaultExperienceChange: (value: "full" | "simple") => Promise<UserPreference>;
}) {
  const session = useQuery({ queryKey: ["session"], queryFn: api.session });
  const [selectedExperience, setSelectedExperience] = useState(defaultExperience);
  const save = useMutation({
    mutationFn: onDefaultExperienceChange,
    onError: () => setSelectedExperience(defaultExperience)
  });
  useEffect(() => setSelectedExperience(defaultExperience), [defaultExperience]);
  const canWrite = session.data?.canWrite !== false;
  return (
    <section className="panel simple-mode-settings">
      <div>
        <div className="panel-heading">Experience</div>
        <p className="muted">Simple mode contains only completed runs and the training-sheet matching workflow.</p>
      </div>
      <div className="simple-mode-settings-actions">
        <Link className="secondary-button" to="/simple">Open simple mode</Link>
        <label className="checkbox-field">
          <input
            type="checkbox"
            checked={selectedExperience === "simple"}
            disabled={!canWrite || save.isPending}
            onChange={(event) => {
              const nextExperience = event.target.checked ? "simple" : "full";
              setSelectedExperience(nextExperience);
              save.mutate(nextExperience);
            }}
          />
          <span>Use simple mode by default</span>
        </label>
      </div>
      {save.isPending && <div className="muted">Saving experience preference…</div>}
      {save.error && <div className="error" role="alert">{save.error instanceof Error ? save.error.message : "Could not save experience preference"}</div>}
    </section>
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
        <div className="panel-heading">Appearance</div>
        <p className="muted">Choose a visual palette for this account. Your selection is saved with your account and follows you between devices.</p>
      </div>
      <ThemePreferenceControl value={value} onChange={onChange} />
      {error && <div className="error" role="alert">{error.message || "Could not save appearance preferences"}</div>}
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
  return (
    <fieldset className="theme-picker">
      <legend>Color theme</legend>
      <div className="theme-options">
        {themeOptions.map((option) => (
          <label className={`theme-option ${value === option.value ? "active" : ""}`} key={option.value}>
            <input
              type="radio"
              name="runnarr-theme"
              value={option.value}
              checked={value === option.value}
              onChange={() => onChange(option.value)}
            />
            <span className="theme-option-card">
              <span className={`theme-preview theme-preview--${option.value}`} aria-hidden="true">
                <span className="theme-preview-sidebar" />
                <span className="theme-preview-page" />
                <span className="theme-preview-accent" />
              </span>
              <span className="theme-option-copy">
                <strong>{option.label}</strong>
                <span>{option.description}</span>
              </span>
            </span>
          </label>
        ))}
      </div>
    </fieldset>
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

function formatCalendarAgendaDate(value: string) {
  const date = new Date(`${value}T12:00:00`);
  return date.toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" });
}

function formatCalendarDayLongDate(value: string) {
  const date = new Date(`${value}T12:00:00`);
  return date.toLocaleDateString(undefined, { weekday: "long", year: "numeric", month: "long", day: "numeric" });
}

function browserCalendarTimezone() {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
}

function isCalendarDate(value: string | undefined): value is string {
  if (!value || !/^\d{4}-\d{2}-\d{2}$/.test(value)) {
    return false;
  }
  const parsed = localDateFromString(value);
  return Boolean(parsed && formatCalendarDate(parsed.getFullYear(), parsed.getMonth() + 1, parsed.getDate()) === value);
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
    metric.sleepScore,
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
      sleepScore: finiteValue(metric.sleepScore),
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
    { label: "Sleep score", value: formatHealthRounded(metric.sleepScore), icon: <Moon size={18} /> },
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
    { label: "Average body battery", value: formatHealthRounded(metric.bodyBatteryAvg) },
    { label: "Minimum body battery", value: formatHealthRounded(metric.bodyBatteryMin) },
    { label: "Body battery start", value: formatHealthRounded(metric.bodyBatteryStart) },
    { label: "Body battery end", value: formatHealthRounded(metric.bodyBatteryEnd) },
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

function healthSummaryDateLabel(metric?: DailyHealthMetric) {
  return formatHealthDate(metric?.date ?? localDateString());
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

function Page({ title, titleAccessory, eyebrow, actions, children }: { title: string; titleAccessory?: ReactNode; eyebrow?: string; actions?: ReactNode; children: ReactNode }) {
  return (
    <div className="page">
      <header className="page-header">
        <div className="page-title">
          {eyebrow && <div className="eyebrow">{eyebrow}</div>}
          <div className="page-title-row">
            <h1>{title}</h1>
            {titleAccessory}
          </div>
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
  const availableKeys = availableSeries.map((series) => series.key);
  const effectiveVisibleSeries = reconcileVisibleActivitySeries(visibleSeries, availableKeys, initialVisible);
  const activeSeries = availableSeries.filter((series) => effectiveVisibleSeries.includes(series.key));
  useEffect(() => {
    setVisibleSeries((current) => {
      const next = reconcileVisibleActivitySeries(current, availableKeys, initialVisible);
      return next.length === current.length && next.every((key, index) => key === current[index]) ? current : next;
    });
  }, [availableKeys.join(","), initialVisible.join(",")]);
  const toggleSeries = (key: ActivityChartSeriesKey) => {
    setVisibleSeries((current) => {
      const selected = reconcileVisibleActivitySeries(current, availableKeys, initialVisible);
      if (selected.includes(key)) {
        return selected.length === 1 ? selected : selected.filter((item) => item !== key);
      }
      return [...selected, key];
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
                  domain={chartDisplayDomain(data.map((item) => item[series.key])) ?? ["auto", "auto"]}
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
  onMapLocationSelect,
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
  onMapLocationSelect?: (location: RoutePoint) => void;
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
        {onMapLocationSelect && <MapLocationPicker onSelect={onMapLocationSelect} />}
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

function MapLocationPicker({ onSelect }: { onSelect: (location: RoutePoint) => void }) {
  useMapEvents({
    click: (event) => onSelect([event.latlng.lat, event.latlng.lng])
  });
  return null;
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

function FitMapContentOnce({ points }: { points: RoutePoint[] }) {
  const map = useMap();
  const fitted = useRef(false);
  const pointsKey = routePointsKey(points);
  useEffect(() => {
    if (fitted.current || points.length === 0) return;
    fitted.current = true;
    if (points.length > 1) {
      map.fitBounds(points, { padding: [24, 24] });
    } else {
      map.setView(points[0], 15);
    }
  }, [map, pointsKey]);
  return null;
}

function CenterMapOnPoint({ point }: { point: RoutePoint }) {
  const map = useMap();
  const pointKey = routePointsKey([point]);
  useEffect(() => {
    map.flyTo(point, Math.max(map.getZoom(), 15), { animate: true, duration: 0.45 });
  }, [map, pointKey]);
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
  const samples = samplesForClimb(activity, climb);
  const samplesByIndex = new Map(samples.map((sample) => [sample.index, sample]));
  const points = chartDataFor(samples)
    .filter((sample) => typeof sample.distanceM === "number" && typeof sample.elevationM === "number")
    .map((sample) => {
      const distanceKm = Math.max(0, (sample.distanceM! - climb.startDistanceM) / 1000);
      const sourceSample = samplesByIndex.get(sample.index);
      return {
        label: `${distanceKm.toFixed(1)} km`,
        distanceKm,
        elevationM: sample.elevationM!,
        paceSPKM: sample.rawPaceSPKM,
        gapSPKM: sourceSample ? gapPaceForSample(activity.laps ?? [], sourceSample) : undefined
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

function activityFilterCount(filters: ActivityTypeFiltersValue) {
  return [
    filters.search?.trim(),
    filters.dateFrom || filters.dateTo,
    filters.sports.length > 0 || filters.excludeSports.length > 0
  ].filter(Boolean).length;
}

function selectedActivityTypes(filters: ActivityTypeFiltersValue, activityTypes: string[]) {
  if (filters.sports.length > 0) {
    const selected = new Set(filters.sports);
    return activityTypes.filter((sport) => selected.has(sport));
  }
  if (filters.excludeSports.length > 0) {
    const excluded = new Set(filters.excludeSports);
    return activityTypes.filter((sport) => !excluded.has(sport));
  }
  return [...activityTypes];
}

function activityTypeFiltersForSelection(
  filters: ActivityTypeFiltersValue,
  activityTypes: string[],
  selectedTypes: string[]
) {
  const selected = new Set(selectedTypes);
  const allSelected = activityTypes.every((sport) => selected.has(sport));
  return {
    ...filters,
    sports: allSelected ? [] : activityTypes.filter((sport) => selected.has(sport)),
    excludeSports: allSelected || selectedTypes.length > 0 ? [] : [...activityTypes]
  };
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
    return formatPaceMinutesSeconds(value);
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
  return decodePolylineWithPrecision(encoded, 5);
}

function decodePolylineWithPrecision(encoded: string, precision: number): RoutePoint[] {
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
    const factor = 10 ** precision;
    coordinates.push([lat / factor, lng / factor]);
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

function EmptyState({ title, message, action }: { title: string; message?: string; action?: ReactNode }) {
  return (
    <div className="empty-state">
      <div className="empty-state-copy">
        <span>{title}</span>
        {message && <span className="muted">{message}</span>}
      </div>
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

function rpeTone(value: number) {
  if (value >= 9) {
    return "max";
  }
  if (value >= 7) {
    return "hard";
  }
  if (value >= 4) {
    return "moderate";
  }
  return "easy";
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

function gearUsageTone(percent: number) {
  if (percent >= 95) {
    return "critical";
  }
  if (percent >= 70) {
    return "warning";
  }
  return "safe";
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
  const roundedSeconds = Math.round(totalSeconds);
  const hours = Math.floor(roundedSeconds / 3600);
  const minutes = Math.floor((roundedSeconds % 3600) / 60);
  const seconds = roundedSeconds % 60;
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
  return `${formatPaceMinutesSeconds(secondsPerKm)} /km`;
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
