import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { Link, NavLink, Navigate, Route, Routes, useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity as ActivityIcon, BarChart3, Cloud, Database, LogOut, Map, RefreshCw, Trash2, Upload } from "lucide-react";
import { divIcon } from "leaflet";
import { MapContainer, Marker, Polyline, TileLayer, useMap } from "react-leaflet";
import { Bar, BarChart, CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { api, ApiError, setCsrfToken } from "./api";
import type { Activity, ActivitySample, ActivityTypeFilters as ActivityTypeFiltersValue, AppConfig, SyncJob } from "./types";

type RoutePoint = [number, number];

const emptyActivityTypeFilters: ActivityTypeFiltersValue = { sports: [], excludeSports: [] };

export function App() {
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

  return <AuthenticatedApp />;
}

function AuthenticatedApp() {
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
          <NavItem to="/activities" icon={<Map size={18} />} label="Activities" />
          <NavItem to="/imports" icon={<Upload size={18} />} label="Imports" />
          <NavItem to="/settings" icon={<Cloud size={18} />} label="Providers" />
        </nav>
        <button className="nav-button" type="button" onClick={() => logout.mutate()}>
          <LogOut size={18} />
          <span>Log out</span>
        </button>
      </aside>
      <main className="main">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/activities" element={<ActivitiesPage />} />
          <Route path="/activities/:id" element={<ActivityDetailPage config={config.data} />} />
          <Route path="/imports" element={<ImportsPage />} />
          <Route path="/settings" element={<SettingsPage />} />
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
    case "intervals":
      return "Intervals.icu";
    case "strava":
      return "Strava";
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
    <Page title="Dashboard" actions={<Link className="secondary-button" to="/imports"><Upload size={16} /> Import</Link>}>
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
                <Tooltip formatter={(value) => [`${value} km`, "Distance"]} />
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
  const [filters, setFilters] = useState<ActivityTypeFiltersValue>(emptyActivityTypeFilters);
  const activityTypes = useQuery({ queryKey: ["activity-types"], queryFn: api.activityTypes });
  const activities = useQuery({ queryKey: ["activities", filters], queryFn: () => api.activities(filters) });
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
  return (
    <Page title="Activities">
      <ActivityTypeFilterPanel
        activityTypes={activityTypes.data?.activityTypes ?? []}
        filters={filters}
        onChange={setFilters}
      />
      {activities.isLoading && <LoadingRow />}
      {deleteActivity.error && <div className="error">{deleteActivity.error instanceof Error ? deleteActivity.error.message : "Delete failed"}</div>}
      {activities.data && <ActivityTable activities={activities.data.activities ?? []} onDelete={handleDelete} deletingId={deleteActivity.variables} />}
      {(activities.data?.activities ?? []).length === 0 && <EmptyState title="No activities yet" action={<Link className="secondary-button" to="/imports">Import a file</Link>} />}
    </Page>
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
      sports: nextSports,
      excludeSports: filters.excludeSports.filter((item) => item !== sport)
    });
  };
  const toggleExclude = (sport: string) => {
    const nextExcluded = excludeSet.has(sport)
      ? filters.excludeSports.filter((item) => item !== sport)
      : [...filters.excludeSports, sport];
    onChange({
      sports: filters.sports.filter((item) => item !== sport),
      excludeSports: nextExcluded
    });
  };
  const clearFilters = () => onChange(emptyActivityTypeFilters);
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
  const activity = useQuery({ queryKey: ["activity", id], queryFn: () => api.activity(id!), enabled: Boolean(id) });
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

  if (activity.isLoading) {
    return <Page title="Activity"><LoadingRow /></Page>;
  }
  if (!activity.data) {
    return <Page title="Activity"><EmptyState title="Activity not found" /></Page>;
  }

  const item = activity.data.activity;
  const routePoints = routeForActivity(item);
  const chartData = chartDataFor(item.samples ?? []);
  const handleDelete = () => {
    if (window.confirm(deleteActivityConfirmation(item))) {
      deleteActivity.mutate(item.id);
    }
  };

  return (
    <Page
      title={item.name}
      eyebrow={`${item.sportType} · ${formatDate(item.startTime)}`}
      actions={
        <button className="danger-button" type="button" disabled={deleteActivity.isPending} onClick={handleDelete}>
          <Trash2 size={16} />
          Delete
        </button>
      }
    >
      {deleteActivity.error && <div className="error">{deleteActivity.error instanceof Error ? deleteActivity.error.message : "Delete failed"}</div>}
      <section className="metric-grid">
        <Metric label="Distance" value={formatDistance(item.distanceM)} />
        <Metric label="Moving Time" value={formatDuration(item.movingTimeS || item.elapsedTimeS)} />
        <Metric label="Pace" value={formatPace(item.avgPaceSPKM)} />
        <Metric label="Elevation" value={`${Math.round(item.elevationGainM).toLocaleString()} m`} />
      </section>

      {routePoints.length > 1 && (
        <section className="panel">
          <div className="panel-heading">Route</div>
          <ActivityMap points={routePoints} tileURL={config?.mapTileURL} />
        </section>
      )}

      <section className="split-layout">
        <ChartPanel title="Elevation" data={chartData} dataKey="elevationM" unit="m" color="#4664c9" />
        <ChartPanel title="Heart rate" data={chartData} dataKey="heartRate" unit="bpm" color="#c84d4d" />
      </section>

      {item.laps && item.laps.length > 0 && (
        <section className="panel">
          <div className="panel-heading">Laps</div>
          <table className="data-table">
            <thead>
              <tr>
                <th>Lap</th>
                <th>Distance</th>
                <th>Time</th>
              </tr>
            </thead>
            <tbody>
              {item.laps.map((lap) => (
                <tr key={lap.index}>
                  <td>{lap.index + 1}</td>
                  <td>{formatDistance(lap.distanceM)}</td>
                  <td>{formatDuration(lap.elapsedTimeS)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
    </Page>
  );
}

function ImportsPage() {
  const [file, setFile] = useState<File | null>(null);
  const queryClient = useQueryClient();
  const imports = useQuery({ queryKey: ["imports"], queryFn: api.imports });
  const upload = useMutation({
    mutationFn: (selected: File) => api.upload(selected),
    onSuccess: async () => {
      setFile(null);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["imports"] }),
        queryClient.invalidateQueries({ queryKey: ["activities"] }),
        queryClient.invalidateQueries({ queryKey: ["summary"] })
      ]);
    }
  });

  return (
    <Page title="Imports">
      <section className="panel upload-panel">
        <div>
          <div className="panel-heading">Activity file</div>
          <p className="muted">GPX, TCX, and FIT are supported in v1.</p>
        </div>
        <input type="file" accept=".gpx,.tcx,.fit" onChange={(event) => setFile(event.target.files?.[0] ?? null)} />
        <button className="primary-button" type="button" disabled={!file || upload.isPending} onClick={() => file && upload.mutate(file)}>
          <Upload size={16} />
          Upload
        </button>
      </section>
      {upload.error && <div className="error">{upload.error instanceof Error ? upload.error.message : "Upload failed"}</div>}
      <section className="panel">
        <div className="panel-heading">Recent imports</div>
        {imports.isLoading && <LoadingRow />}
        {imports.data && (
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
              {(imports.data.imports ?? []).map((item) => (
                <tr key={item.id}>
                  <td>{item.filename}</td>
                  <td>{item.parser.toUpperCase()}</td>
                  <td><span className={`status ${item.status}`}>{item.status}</span>{item.error && <span className="row-error">{item.error}</span>}</td>
                  <td>{formatDate(item.createdAt)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </Page>
  );
}

function SettingsPage() {
  const queryClient = useQueryClient();
  const status = useQuery({ queryKey: ["strava-status"], queryFn: api.stravaStatus });
  const intervalsStatus = useQuery({ queryKey: ["intervals-status"], queryFn: api.intervalsStatus });
  const jobs = useQuery({ queryKey: ["sync-jobs"], queryFn: api.syncJobs, refetchInterval: 2000 });
  const [intervalsAPIKey, setIntervalsAPIKey] = useState("");
  const [intervalsOldest, setIntervalsOldest] = useState("1970-01-01");
  const latestIntervalsJob = (jobs.data?.jobs ?? []).find((job) => job.provider === "intervals");
  const intervalsSyncRunning = latestIntervalsJob?.status === "running";
  const sync = useMutation({
    mutationFn: api.stravaSync,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["strava-status"] }),
        queryClient.invalidateQueries({ queryKey: ["sync-jobs"] }),
        queryClient.invalidateQueries({ queryKey: ["activities"] }),
        queryClient.invalidateQueries({ queryKey: ["summary"] })
      ]);
    }
  });
  const intervalsConnect = useMutation({
    mutationFn: api.intervalsConnect,
    onSuccess: async () => {
      setIntervalsAPIKey("");
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["intervals-status"] }),
        queryClient.invalidateQueries({ queryKey: ["sync-jobs"] })
      ]);
    }
  });
  const intervalsSync = useMutation({
    mutationFn: api.intervalsSync,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["intervals-status"] }),
        queryClient.invalidateQueries({ queryKey: ["sync-jobs"] }),
        queryClient.invalidateQueries({ queryKey: ["activities"] }),
        queryClient.invalidateQueries({ queryKey: ["summary"] })
      ]);
    }
  });

  return (
    <Page title="Providers">
      <section className="panel provider-panel">
        <div>
          <div className="panel-heading">Strava</div>
          {status.data?.configured ? (
            <p className="muted">{status.data.connected ? `Connected as ${status.data.connection.displayName}` : "Configured but not connected."}</p>
          ) : (
            <p className="muted">Set STRAVA_CLIENT_ID and STRAVA_CLIENT_SECRET to enable OAuth.</p>
          )}
        </div>
        <div className="actions">
          <a className="secondary-button" href="/api/providers/strava/connect" aria-disabled={!status.data?.configured}>
            <Cloud size={16} />
            Connect
          </a>
          <button className="primary-button" type="button" disabled={!status.data?.connected || sync.isPending} onClick={() => sync.mutate()}>
            <RefreshCw size={16} />
            Sync
          </button>
        </div>
      </section>
      {sync.error && <div className="error">{sync.error instanceof Error ? sync.error.message : "Sync failed"}</div>}
      <section className="panel provider-panel">
        <div>
          <div className="panel-heading">Intervals.icu</div>
          <p className="muted">{intervalsStatus.data?.connected ? `Connected as ${intervalsStatus.data.connection.displayName}` : "Connect with a personal API key from Intervals.icu settings."}</p>
        </div>
        <div className="provider-controls">
          <input
            type="password"
            placeholder="API key"
            value={intervalsAPIKey}
            onChange={(event) => setIntervalsAPIKey(event.target.value)}
          />
          <button className="secondary-button" type="button" disabled={!intervalsAPIKey || intervalsConnect.isPending} onClick={() => intervalsConnect.mutate(intervalsAPIKey)}>
            <Cloud size={16} />
            Connect
          </button>
          <label className="compact-field">
            <span>Oldest</span>
            <input type="date" value={intervalsOldest} onChange={(event) => setIntervalsOldest(event.target.value)} />
          </label>
          <button className="primary-button" type="button" disabled={!intervalsStatus.data?.connected || intervalsSync.isPending || intervalsSyncRunning} onClick={() => intervalsSync.mutate(intervalsOldest)}>
            <RefreshCw size={16} />
            {intervalsSyncRunning ? "Syncing" : "Sync"}
          </button>
        </div>
      </section>
      <SyncProgressCard job={latestIntervalsJob} />
      {intervalsConnect.error && <div className="error">{intervalsConnect.error instanceof Error ? intervalsConnect.error.message : "Intervals.icu connection failed"}</div>}
      {intervalsSync.error && <div className="error">{intervalsSync.error instanceof Error ? intervalsSync.error.message : "Intervals.icu sync failed"}</div>}
      <section className="panel">
        <div className="panel-heading">Sync jobs</div>
        {jobs.isLoading && <LoadingRow />}
        {jobs.data && (
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
              {(jobs.data.jobs ?? []).map((job) => (
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
        )}
      </section>
    </Page>
  );
}

function SyncProgressCard({ job }: { job?: SyncJob }) {
  if (!job || job.provider !== "intervals") {
    return null;
  }
  const payload = job.payload ?? {};
  const imported = payloadNumber(payload, "imported");
  const processed = payloadNumber(payload, "processed");
  const activities = payloadNumber(payload, "activities");
  const failed = payloadNumber(payload, "failed");
  const skippedExcluded = payloadNumber(payload, "skippedExcluded");
  const stage = payloadText(payload, "stage") || job.status;
  const currentWindowStart = payloadText(payload, "currentWindowStart");
  const currentWindowEnd = payloadText(payload, "currentWindowEnd");
  const warnings = payloadList(payload, "warnings");
  const firstErrors = payloadList(payload, "firstErrors");
  const windowText = currentWindowStart && currentWindowEnd ? `${currentWindowStart} to ${currentWindowEnd}` : "Waiting for first window";

  return (
    <section className="panel sync-progress-panel">
      <div className="filter-header">
        <div className="panel-heading">Intervals.icu sync progress</div>
        <span className={`status ${job.status}`}>{job.status}</span>
      </div>
      <SyncProgressBar job={job} />
      <div className="sync-progress-grid">
        <SyncStat label="Imported" value={imported.toLocaleString()} />
        <SyncStat label="Processed" value={`${processed.toLocaleString()} / ${activities.toLocaleString()}`} />
        <SyncStat label="Failed" value={failed.toLocaleString()} />
        <SyncStat label="Ignored" value={skippedExcluded.toLocaleString()} />
      </div>
      <div className="sync-progress-details">
        <span>{stage}</span>
        <span>{windowText}</span>
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
  const percent = activities > 0 ? Math.min(100, Math.round((processed / activities) * 100)) : 0;
  return (
    <div className="progress-cell">
      <div className="progress-bar" aria-label={`Sync progress ${percent}%`}>
        <span style={{ width: `${percent}%` }} />
      </div>
      <span>{activities > 0 ? `${percent}%` : job.status}</span>
    </div>
  );
}

function formatSyncJobDetails(job: SyncJob) {
  const payload = job.payload ?? {};
  const imported = payloadNumber(payload, "imported");
  const failed = payloadNumber(payload, "failed");
  const skippedExcluded = payloadNumber(payload, "skippedExcluded");
  const activities = payloadNumber(payload, "activities");
  const windows = payloadNumber(payload, "fetchedWindows");
  const splitWindows = payloadNumber(payload, "splitWindows");
  const currentWindowStart = payloadText(payload, "currentWindowStart");
  const currentWindowEnd = payloadText(payload, "currentWindowEnd");
  const parts = [];
  if (activities > 0 || imported > 0 || failed > 0) {
    parts.push(`${imported}/${activities} imported`);
  }
  if (failed > 0) {
    parts.push(`${failed} failed`);
  }
  if (skippedExcluded > 0) {
    parts.push(`${skippedExcluded} ignored`);
  }
  if (windows > 0) {
    parts.push(`${windows} windows`);
  }
  if (splitWindows > 0) {
    parts.push(`${splitWindows} split`);
  }
  if (currentWindowStart && currentWindowEnd && job.status === "running") {
    parts.push(`${currentWindowStart} to ${currentWindowEnd}`);
  }
  return parts.length > 0 ? parts.join(" · ") : "-";
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

function ChartPanel({ title, data, dataKey, unit, color }: { title: string; data: Array<Record<string, number | string | undefined>>; dataKey: string; unit: string; color: string }) {
  const hasData = data.some((item) => typeof item[dataKey] === "number");
  return (
    <section className="panel">
      <div className="panel-heading">{title}</div>
      {hasData ? (
        <div className="chart-area">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={data}>
              <CartesianGrid strokeDasharray="3 3" vertical={false} />
              <XAxis dataKey="label" minTickGap={26} />
              <YAxis width={42} />
              <Tooltip formatter={(value) => [`${value} ${unit}`, title]} />
              <Line type="monotone" dataKey={dataKey} stroke={color} dot={false} strokeWidth={2} connectNulls />
            </LineChart>
          </ResponsiveContainer>
        </div>
      ) : (
        <EmptyState title="No samples for this chart" />
      )}
    </section>
  );
}

function ActivityMap({ points, tileURL }: { points: RoutePoint[]; tileURL?: string }) {
  const center = points[0] ?? [53.3498, -6.2603];
  const start = points[0];
  const end = points.length > 1 ? points[points.length - 1] : undefined;
  return (
    <div className="map-frame">
      <MapContainer center={center} zoom={13} scrollWheelZoom className="route-map">
        <TileLayer attribution="&copy; OpenStreetMap contributors" url={tileURL || "https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"} />
        <Polyline pathOptions={{ color: "#d85c41", weight: 4 }} positions={points} />
        {start && <Marker position={start} icon={routeEndpointIcon("start")} interactive={false} keyboard={false} />}
        {end && <Marker position={end} icon={routeEndpointIcon("end")} interactive={false} keyboard={false} />}
        <FitRoute points={points} />
      </MapContainer>
    </div>
  );
}

function FitRoute({ points }: { points: RoutePoint[] }) {
  const map = useMap();
  useEffect(() => {
    if (points.length > 1) {
      map.fitBounds(points, { padding: [24, 24] });
    }
  }, [map, points]);
  return null;
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

function chartDataFor(samples: ActivitySample[]) {
  return samples.map((sample, index) => ({
    label: sample.distanceM !== undefined ? `${(sample.distanceM / 1000).toFixed(1)} km` : String(index + 1),
    elevationM: sample.elevationM !== undefined ? Math.round(sample.elevationM) : undefined,
    heartRate: sample.heartRate
  }));
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

function formatDuration(totalSeconds: number) {
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (hours > 0) {
    return `${hours}:${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
  }
  return `${minutes}:${String(seconds).padStart(2, "0")}`;
}

function formatPace(secondsPerKm?: number) {
  if (!secondsPerKm || !Number.isFinite(secondsPerKm)) {
    return "-";
  }
  const minutes = Math.floor(secondsPerKm / 60);
  const seconds = Math.round(secondsPerKm % 60);
  return `${minutes}:${String(seconds).padStart(2, "0")} /km`;
}
