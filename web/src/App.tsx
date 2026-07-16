import { useEffect, useRef, useState } from "react";
import type { CSSProperties, ReactNode } from "react";
import { Link, NavLink, Navigate, Route, Routes, useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity as ActivityIcon, ArrowDown, ArrowUp, ArrowUpDown, BarChart3, ChevronDown, ChevronLeft, ChevronRight, Cloud, Database, ExternalLink, Filter, LogOut, Map as MapIcon, Monitor, Moon, MoreVertical, Pencil, RefreshCw, RotateCcw, Settings as SettingsIcon, Sun, Trash2, Upload, X } from "lucide-react";
import { divIcon } from "leaflet";
import { MapContainer, Marker, Polyline, TileLayer, useMap } from "react-leaflet";
import { Area, AreaChart, Bar, BarChart, CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { api, ApiError, setCsrfToken } from "./api";
import { PACE_ROUTE_COLORS, clampPaceToScale, paceColorForPace, paceScaleFromSpeeds, speedToPaceSPKM } from "./paceDisplay";
import type { PaceDisplayScale } from "./paceDisplay";
import type { Activity, ActivityClimb, ActivityMedia, ActivitySample, ActivitySortBy, ActivityTypeFilters as ActivityTypeFiltersValue, AppConfig, ImportFile, SyncJob } from "./types";

type RoutePoint = [number, number];
type ActivityDateRange = Pick<ActivityTypeFiltersValue, "dateFrom" | "dateTo">;
type ActivitySort = Required<Pick<ActivityTypeFiltersValue, "sortBy" | "sortOrder">>;
type ThemePreference = "system" | "light" | "dark";
type ActivityChartSeriesKey = "elevationM" | "heartRate" | "paceSPKM" | "power" | "cadence";
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

const defaultActivitySort: ActivitySort = { sortBy: "date", sortOrder: "desc" };
const emptyActivityTypeFilters: ActivityTypeFiltersValue = { sports: [], excludeSports: [], search: "", dateFrom: "", dateTo: "", ...defaultActivitySort };
const ACTIVITY_LIST_PAGE_SIZE = 100;
const themePreferenceStorageKey = "runnarr-theme-preference";
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
const activityChartSeries: ActivityChartSeries[] = [
  { key: "elevationM", label: "Elevation", color: "#4664c9", defaultVisible: true, format: (value) => `${Math.round(value).toLocaleString()} m` },
  { key: "heartRate", label: "Heart rate", color: "#c84d4d", defaultVisible: true, format: (value) => `${Math.round(value)} bpm` },
  { key: "paceSPKM", label: "Pace", color: "#2f8f83", defaultVisible: true, format: (value) => formatPace(value) },
  { key: "power", label: "Power", color: "#b7791f", defaultVisible: false, format: (value) => `${Math.round(value)} W` },
  { key: "cadence", label: "Cadence", color: "#7a4eb2", defaultVisible: false, format: (value) => `${Math.round(value)} spm` }
];

function useThemePreference(): [ThemePreference, (preference: ThemePreference) => void] {
  const [themePreference, setThemePreferenceState] = useState<ThemePreference>(readStoredThemePreference);

  useEffect(() => {
    applyThemePreference(themePreference);
  }, [themePreference]);

  const setThemePreference = (preference: ThemePreference) => {
    setThemePreferenceState(preference);
    try {
      window.localStorage.setItem(themePreferenceStorageKey, preference);
    } catch {
      // Local storage can be unavailable in private or restricted browser contexts.
    }
  };

  return [themePreference, setThemePreference];
}

function readStoredThemePreference(): ThemePreference {
  try {
    return parseThemePreference(window.localStorage.getItem(themePreferenceStorageKey));
  } catch {
    return "system";
  }
}

function parseThemePreference(value: string | null): ThemePreference {
  return value === "light" || value === "dark" || value === "system" ? value : "system";
}

function applyThemePreference(preference: ThemePreference) {
  const root = document.documentElement;
  if (preference === "system") {
    delete root.dataset.theme;
    return;
  }
  root.dataset.theme = preference;
}

export function App() {
  const [themePreference, setThemePreference] = useThemePreference();
  const session = useQuery({ queryKey: ["session"], queryFn: api.session });

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

  return <AuthenticatedApp themePreference={themePreference} onThemePreferenceChange={setThemePreference} />;
}

function AuthenticatedApp({
  themePreference,
  onThemePreferenceChange
}: {
  themePreference: ThemePreference;
  onThemePreferenceChange: (preference: ThemePreference) => void;
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
        </nav>
        <div className="sidebar-bottom">
          <NavItem to="/settings" icon={<SettingsIcon size={18} />} label="Settings" />
          <button className="nav-button" type="button" onClick={() => logout.mutate()}>
            <LogOut size={18} />
            <span>Log out</span>
          </button>
        </div>
      </aside>
      <main className="main">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/activities" element={<ActivitiesPage />} />
          <Route path="/activities/:id" element={<ActivityDetailPage config={config.data} />} />
          <Route path="/imports" element={<Navigate to="/settings#import" replace />} />
          <Route path="/settings" element={<SettingsPage themePreference={themePreference} onThemePreferenceChange={onThemePreferenceChange} />} />
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

function LoginPage() {
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const login = useMutation({
    mutationFn: api.login,
    onSuccess: async (session) => {
      setCsrfToken(session.csrfToken);
      await queryClient.invalidateQueries({ queryKey: ["session"] });
      navigate("/");
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : "Login failed")
  });

  return (
    <div className="login-page">
      <form
        className="login-panel"
        onSubmit={(event) => {
          event.preventDefault();
          login.mutate(password);
        }}
      >
        <div className="brand login-brand">
          <ActivityIcon size={26} />
          <span>Runnarr</span>
        </div>
        <label className="field">
          <span>Password</span>
          <input autoFocus type="password" value={password} onChange={(event) => setPassword(event.target.value)} />
        </label>
        {error && <div className="error">{error}</div>}
        <button className="primary-button" type="submit" disabled={login.isPending || password.length === 0}>
          Log in
        </button>
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
        <Metric label="Activities" value={summary.data.activityCount.toLocaleString()} />
        <Metric label="Distance" value={formatDistance(summary.data.distanceM)} />
        <Metric label="Moving Time" value={formatDuration(summary.data.movingTimeS)} />
        <Metric label="Elevation" value={`${Math.round(summary.data.elevationGainM).toLocaleString()} m`} />
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

function ActivitiesPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const filters = activityFiltersFromSearchParams(searchParams);
  const setFilters = (nextFilters: ActivityTypeFiltersValue) => {
    setSearchParams(activityFiltersToSearchParams(nextFilters), { replace: true });
  };
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [sortOpen, setSortOpen] = useState(false);
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
          <ActivityTable activities={activityList} onDelete={handleDelete} deletingId={deleteActivity.variables} />
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

function ActivityTable({
  activities,
  compact = false,
  onDelete,
  deletingId
}: {
  activities: Activity[];
  compact?: boolean;
  onDelete?: (activity: Activity) => void;
  deletingId?: string;
}) {
  if (activities.length === 0) {
    return <EmptyState title="No activities found" />;
  }
  return (
    <div className="table-wrap">
      <table className="data-table">
        <thead>
          <tr>
            <th>Date</th>
            <th>Name</th>
            {!compact && <th>Type</th>}
            <th>Distance</th>
            <th>Time</th>
            {!compact && <th>Calories</th>}
            {!compact && <th>Source</th>}
            {onDelete && <th aria-label="Actions" />}
          </tr>
        </thead>
        <tbody>
          {activities.map((activity) => (
            <tr key={activity.id}>
              <td>{formatDate(activity.startTime)}</td>
              <td><Link to={`/activities/${activity.id}`}>{activity.name}</Link></td>
              {!compact && <td>{activity.sportType}</td>}
              <td>{formatDistance(activity.distanceM)}</td>
              <td>{formatDuration(activity.movingTimeS || activity.elapsedTimeS)}</td>
              {!compact && <td>{formatCalories(activity.caloriesKcal)}</td>}
              {!compact && <td><span className="source-pill">{activity.source}</span></td>}
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
  const activity = useQuery({ queryKey: activityQueryKey, queryFn: () => api.activity(id!), enabled: Boolean(id) });
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
  const [actionsOpen, setActionsOpen] = useState(false);
  const [renameOpen, setRenameOpen] = useState(false);
  const [mediaFileInputKey, setMediaFileInputKey] = useState(0);
  const [selectedMediaId, setSelectedMediaId] = useState<string>();

  useEffect(() => {
    setHighlightedSample(undefined);
    setSelectedClimbIndex(undefined);
    setActionsOpen(false);
    setRenameOpen(false);
    setSelectedMediaId(undefined);
    uploadMedia.reset();
    setMediaFileInputKey((key) => key + 1);
  }, [id]);

  if (activity.isLoading) {
    return <Page title="Activity"><LoadingRow /></Page>;
  }
  if (!activity.data) {
    return <Page title="Activity"><EmptyState title="Activity not found" /></Page>;
  }

  const item = activity.data.activity;
  const mediaItems = item.media ?? [];
  const locatedMedia = mediaItems.filter(hasMediaLocation);
  const routePoints = routeForActivity(item);
  const paceScale = paceScaleForActivity(item);
  const paceRouteSegments = paceRouteSegmentsForActivity(item, paceScale);
  const chartData = chartDataFor(item.samples ?? [], paceScale);
  const highlightedPoint = routePointForChartPoint(highlightedSample);
  const climbs = item.climbs ?? [];
  const selectedClimb = selectedClimbIndex === undefined ? undefined : climbs.find((climb) => climb.index === selectedClimbIndex);
  const climbMapSegments = climbMapSegmentsFor(item, climbs);
  const selectedClimbProfile = climbProfileFor(item, selectedClimb);
  const showLapElevation = (item.laps ?? []).some((lap) => lap.elevationGainM !== undefined || lap.elevationLossM !== undefined);
  const showLapGap = (item.laps ?? []).some((lap) => lap.avgGradeAdjustedPaceSPKM !== undefined);
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
  const handleMediaFilesSelected = (files: File[]) => {
    if (files.length === 0 || uploadMedia.isPending) {
      return;
    }
    uploadMedia.reset();
    uploadMedia.mutate(files);
  };

  return (
    <Page
      title={item.name}
      eyebrow={`${item.sportType} · ${formatDate(item.startTime)}`}
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
            onToggle={() => setActionsOpen((current) => !current)}
            onRename={() => {
              renameActivity.reset();
              setRenameOpen(true);
              setActionsOpen(false);
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
      <section className="metric-grid">
        <Metric label="Distance" value={formatDistance(item.distanceM)} />
        <Metric label="Moving Time" value={formatDuration(item.movingTimeS || item.elapsedTimeS)} />
        <Metric label="Pace" value={formatPace(item.avgPaceSPKM)} />
        <Metric label="Elevation" value={`${Math.round(item.elevationGainM).toLocaleString()} m`} />
        {item.avgGradeAdjustedPaceSPKM !== undefined && <Metric label="GAP" value={formatPace(item.avgGradeAdjustedPaceSPKM)} />}
        {item.caloriesKcal !== undefined && <Metric label="Calories" value={formatCalories(item.caloriesKcal)} />}
      </section>

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
          <div className="panel-heading">Route</div>
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
          />
        </section>
      )}

      {climbs.length > 0 && (
        <ActivityClimbsPanel
          climbs={climbs}
          selectedClimb={selectedClimb}
          profileData={selectedClimbProfile}
          onSelect={handleSelectClimb}
        />
      )}

      <ActivityCombinedChart key={item.id} data={chartData} onHighlight={setHighlightedSample} />

      {item.laps && item.laps.length > 0 && (
        <section className="panel">
          <div className="panel-heading">Laps</div>
          <table className="data-table">
            <thead>
              <tr>
                <th>Lap</th>
                <th>Distance</th>
                <th>Time</th>
                <th>Pace</th>
                {showLapGap && <th>GAP</th>}
                {showLapElevation && <th>Gain</th>}
                {showLapElevation && <th>Loss</th>}
              </tr>
            </thead>
            <tbody>
              {item.laps.map((lap) => (
                <tr key={lap.index}>
                  <td>{lap.index + 1}</td>
                  <td>{formatDistance(lap.distanceM)}</td>
                  <td>{formatDuration(lap.elapsedTimeS)}</td>
                  <td>{formatPace(lapPaceSPKM(lap))}</td>
                  {showLapGap && <td>{lap.avgGradeAdjustedPaceSPKM !== undefined ? formatPace(lap.avgGradeAdjustedPaceSPKM) : ""}</td>}
                  {showLapElevation && <td>{lap.elevationGainM !== undefined ? `${Math.round(lap.elevationGainM).toLocaleString()} m` : "-"}</td>}
                  {showLapElevation && <td>{lap.elevationLossM !== undefined ? `${Math.round(lap.elevationLossM).toLocaleString()} m` : "-"}</td>}
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
    </Page>
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
  onToggle,
  onRename,
  onDelete
}: {
  activity: Activity;
  open: boolean;
  deleting: boolean;
  onToggle: () => void;
  onRename: () => void;
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
          {activity.originalProviderUrl && (
            <a className="action-menu-item" role="menuitem" href={activity.originalProviderUrl} target="_blank" rel="noreferrer">
              <ExternalLink size={16} />
              Open original
            </a>
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
  onSelect
}: {
  climbs: ActivityClimb[];
  selectedClimb?: ActivityClimb;
  profileData: ClimbProfilePoint[];
  onSelect: (climb: ActivityClimb) => void;
}) {
  return (
    <section className="panel climbs-panel">
      <div className="chart-header">
        <div className="panel-heading">Climbs</div>
        <span className="muted">{climbs.length.toLocaleString()} detected</span>
      </div>
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
  onThemePreferenceChange
}: {
  themePreference: ThemePreference;
  onThemePreferenceChange: (preference: ThemePreference) => void;
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
  const [garminOldest, setGarminOldest] = useState("1970-01-01");
  const latestGarminJob = (jobs.data?.jobs ?? []).find((job) => job.provider === "garmin");
  const garminSyncRunning = latestGarminJob?.status === "running";

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
        queryClient.invalidateQueries({ queryKey: ["activities"] }),
        queryClient.invalidateQueries({ queryKey: ["summary"] })
      ]);
    }
  });
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
          <label className="compact-field">
            <span>Oldest</span>
            <input type="date" value={garminOldest} onChange={(event) => setGarminOldest(event.target.value)} />
          </label>
          <button className="primary-button" type="button" disabled={!garminStatus.data?.connected || garminSync.isPending || garminSyncRunning} onClick={() => garminSync.mutate(garminOldest)}>
            <RefreshCw size={16} />
            {garminSyncRunning ? "Syncing" : "Sync"}
          </button>
        </div>
      </section>
      <SyncProgressCard job={latestGarminJob} />
      {garminConnect.error && <div className="error">{garminConnect.error instanceof Error ? garminConnect.error.message : "Garmin connection failed"}</div>}
      {garminSync.error && <div className="error">{garminSync.error instanceof Error ? garminSync.error.message : "Garmin sync failed"}</div>}
      <DisplaySettingsSection value={themePreference} onChange={onThemePreferenceChange} />
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

function DisplaySettingsSection({
  value,
  onChange
}: {
  value: ThemePreference;
  onChange: (preference: ThemePreference) => void;
}) {
  return (
    <section className="panel display-panel">
      <div>
        <div className="panel-heading">Display</div>
        <p className="muted">Choose how Runnarr follows your browser color settings.</p>
      </div>
      <ThemePreferenceControl value={value} onChange={onChange} />
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
            </tr>
          ))}
        </tbody>
      </table>
    </div>
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
  const payload = job.payload ?? {};
  const imported = payloadNumber(payload, "imported");
  const processed = payloadNumber(payload, "processed");
  const activities = payloadNumber(payload, "activities");
  const failed = payloadNumber(payload, "failed");
  const skippedExcluded = payloadNumber(payload, "skippedExcluded");
  const stage = payloadText(payload, "stage") || job.status;
  const listing = isSyncListingStage(stage);
  const fetchedPages = payloadNumber(payload, "fetchedPages");
  const oldest = payloadText(payload, "oldest");
  const currentActivityName = payloadText(payload, "currentActivityName");
  const warnings = payloadList(payload, "warnings");
  const firstErrors = payloadList(payload, "firstErrors");
  const foundLabel = activities === 1 ? "activity" : "activities";
  const detailText = syncProgressDetailText(job, stage, currentActivityName, oldest, activities);

  return (
    <section className="panel sync-progress-panel">
      <div className="filter-header">
        <div className="panel-heading">Garmin sync progress</div>
        <span className={`status ${job.status}`}>{job.status}</span>
      </div>
      <SyncProgressBar job={job} />
      <div className="sync-progress-grid">
        {listing ? (
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
  const activities = payloadNumber(payload, "activities");
  const stage = payloadText(payload, "stage");
  const listing = job.status === "running" && isSyncListingStage(stage);
  const hasKnownTotal = activities > 0 && !listing;
  const percent = hasKnownTotal ? Math.min(100, Math.round((processed / activities) * 100)) : 0;
  return (
    <div className="progress-cell">
      <div className={`progress-bar${listing ? " indeterminate" : ""}`} aria-label={listing ? "Listing Garmin activities" : `Sync progress ${percent}%`}>
        <span style={listing ? undefined : { width: `${percent}%` }} />
      </div>
      <span>{listing ? "Listing" : hasKnownTotal ? `${percent}%` : job.status}</span>
    </div>
  );
}

function formatSyncJobDetails(job: SyncJob) {
  const payload = job.payload ?? {};
  const imported = payloadNumber(payload, "imported");
  const processed = payloadNumber(payload, "processed");
  const failed = payloadNumber(payload, "failed");
  const skippedExcluded = payloadNumber(payload, "skippedExcluded");
  const activities = payloadNumber(payload, "activities");
  const fetchedPages = payloadNumber(payload, "fetchedPages");
  const stage = payloadText(payload, "stage");
  const parts = [];
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

function syncProgressDetailText(job: SyncJob, stage: string, currentActivityName: string, oldest: string, activities: number) {
  if (isSyncListingStage(stage)) {
    return oldest ? `Searching from ${oldest}` : "Searching Garmin Connect";
  }
  if (currentActivityName) {
    return currentActivityName;
  }
  if (job.status === "completed") {
    return activities > 0 ? "Sync finished" : "No activities found";
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

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="metric">
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
                  connectNulls
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
  onSelectMedia
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
      {paceSegments.length > 0 && <ActivityPaceRouteLegend />}
    </div>
  );
}

function ActivityPaceRouteLegend() {
  return (
    <div className="pace-route-legend" aria-label="Route pace color legend">
      <span>slowest</span>
      <span className="pace-route-legend-gradient" style={{ background: `linear-gradient(to right, ${PACE_ROUTE_COLORS.join(", ")})` }} />
      <span>fastest</span>
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

function paceScaleForActivity(activity: Activity) {
  return paceScaleFromSpeeds((activity.samples ?? []).map((sample) => sample.speedMPS));
}

function paceRouteSegmentsForActivity(activity: Activity, paceScale?: PaceDisplayScale): PaceRouteSegment[] {
  const samples = (activity.samples ?? [])
    .filter((sample) => typeof sample.latitude === "number" && typeof sample.longitude === "number")
    .map((sample) => ({
      point: [sample.latitude!, sample.longitude!] as RoutePoint,
      speedMPS: typeof sample.speedMPS === "number" && Number.isFinite(sample.speedMPS) && sample.speedMPS > 0 ? sample.speedMPS : undefined
    }));
  if (samples.length < 2) {
    return [];
  }

  const segments: Array<{ start: RoutePoint; end: RoutePoint; paceSPKM: number }> = [];
  for (let index = 1; index < samples.length; index += 1) {
    const paceSPKM = paceForRouteSegment(samples[index - 1].speedMPS, samples[index].speedMPS);
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

function paceForRouteSegment(previousSpeedMPS?: number, currentSpeedMPS?: number) {
  const speeds = [previousSpeedMPS, currentSpeedMPS].filter((speed): speed is number => typeof speed === "number" && speed > 0);
  if (speeds.length === 0) {
    return undefined;
  }
  const avgSpeedMPS = speeds.reduce((total, speed) => total + speed, 0) / speeds.length;
  return speedToPaceSPKM(avgSpeedMPS);
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

function lapPaceSPKM(lap: NonNullable<Activity["laps"]>[number]) {
  if (lap.distanceM <= 0 || lap.elapsedTimeS <= 0) {
    return undefined;
  }
  return lap.elapsedTimeS / (lap.distanceM / 1000);
}

function formatPace(secondsPerKm?: number) {
  if (!secondsPerKm || !Number.isFinite(secondsPerKm)) {
    return "-";
  }
  const minutes = Math.floor(secondsPerKm / 60);
  const seconds = Math.round(secondsPerKm % 60);
  return `${minutes}:${String(seconds).padStart(2, "0")} /km`;
}

function formatCalories(value?: number) {
  if (value === undefined || !Number.isFinite(value)) {
    return "-";
  }
  return `${Math.round(value).toLocaleString()} kcal`;
}
