import { useState, type ReactNode } from "react";
import {
  Link,
  NavLink,
  Route,
  Routes,
  useSearchParams,
} from "react-router-dom";
import { useMutation, useQuery } from "@tanstack/react-query";
import { api } from "../api";
import type { Course } from "../types";
import { raceApi } from "./api";
import {
  RaceError,
  RaceField,
  RaceLoading,
  useInvalidateRaces,
} from "./components";
import { RaceEditorPage } from "./RaceEditor";
import { RaceDetailPage } from "./RaceDetail";
import { RacePerformancePage } from "./RacePerformance";
import { RaceSettingsPage } from "./RaceSettings";
import { RaceReportPage } from "./RaceReport";
import {
  formatRaceTime,
  raceCountdown,
  raceDisciplines,
  raceDistance,
  raceInput,
  raceKinds,
  raceLabel,
  raceStatuses,
  resultTime,
} from "./utils";
import type { RaceCandidate } from "./types";

export function RacesPage({
  canWrite,
  renderCourse,
}: {
  canWrite: boolean;
  renderCourse: (course: Course) => ReactNode;
}) {
  return (
    <div className="race-page">
      <div className="race-heading">
        <h1>Races</h1>
        <div className="race-actions">
          <Link to="/races/settings" className="secondary-button">
            Race settings
          </Link>
          {canWrite && (
            <Link to="/races/new" className="primary-button">
              New race
            </Link>
          )}
        </div>
      </div>
      <nav className="race-tabs" aria-label="Race views">
        <NavLink to="/races" end>
          Upcoming
        </NavLink>
        <NavLink to="/races/history">History</NavLink>
        <NavLink to="/races/performance">Performance</NavLink>
        <NavLink to="/races/review">Review</NavLink>
      </nav>
      <Routes>
        <Route index element={<RaceList view="upcoming" />} />
        <Route path="history" element={<RaceList view="history" />} />
        <Route
          path="performance"
          element={<RacePerformancePage canWrite={canWrite} />}
        />
        <Route path="review" element={<RaceReview canWrite={canWrite} />} />
        <Route
          path="settings"
          element={<RaceSettingsPage canWrite={canWrite} />}
        />
        <Route path="new" element={<RaceEditorPage canWrite={canWrite} />} />
        <Route
          path=":id/edit"
          element={<RaceEditorPage canWrite={canWrite} />}
        />
        <Route
          path=":id/report"
          element={<RaceReportPage canWrite={canWrite} />}
        />
        <Route
          path=":id"
          element={
            <RaceDetailPage canWrite={canWrite} renderCourse={renderCourse} />
          }
        />
        <Route path="*" element={<p>Race page not found.</p>} />
      </Routes>
    </div>
  );
}
export function RaceFilters({ dates = true }: { dates?: boolean }) {
  const [params, setParams] = useSearchParams();
  const groups = useQuery({
    queryKey: ["races", "groups"],
    queryFn: () => raceApi.resources("groups"),
  });
  function change(key: string, value: string) {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    next.delete("offset");
    setParams(next, { replace: true });
  }
  return (
    <div className="race-filters">
      <RaceField label="Find race">
        <input
          value={params.get("q") ?? ""}
          onChange={(e) => change("q", e.target.value)}
        />
      </RaceField>
      {(
        [
          ["discipline", "Discipline", raceDisciplines],
          ["kind", "Event kind", raceKinds],
        ] as const
      ).map(([key, label, values]) => (
        <RaceField key={key} label={label}>
          <select
            value={params.get(key) ?? ""}
            onChange={(e) => change(key, e.target.value)}
          >
            <option value="">All</option>
            {values.map((v) => (
              <option key={v} value={v}>
                {raceLabel(v)}
              </option>
            ))}
          </select>
        </RaceField>
      ))}
      <RaceField label="Recurring event">
        <select
          value={params.get("groupId") ?? ""}
          onChange={(e) => change("groupId", e.target.value)}
        >
          <option value="">All events</option>
          {groups.data?.items.map((g) => (
            <option key={g.id} value={g.id}>
              {g.name}
            </option>
          ))}
        </select>
      </RaceField>
      <RaceField label="Status">
        <select
          value={params.get("status") ?? ""}
          onChange={(e) => change("status", e.target.value)}
        >
          <option value="">All statuses</option>
          {raceStatuses.map((status) => (
            <option key={status} value={status}>
              {raceLabel(status)}
            </option>
          ))}
        </select>
      </RaceField>
      <RaceField label="Sort by">
        <select
          value={params.get("sort") ?? "date"}
          onChange={(e) => change("sort", e.target.value)}
        >
          <option value="date">Date</option>
          <option value="name">Name</option>
          <option value="distance">Distance</option>
          <option value="time">Result time</option>
        </select>
      </RaceField>
      <RaceField label="Sort direction">
        <select
          value={params.get("order") ?? ""}
          onChange={(e) => change("order", e.target.value)}
        >
          <option value="">Default for view</option>
          <option value="asc">Ascending</option>
          <option value="desc">Descending</option>
        </select>
      </RaceField>
      {dates && (
        <>
          <RaceField label="From">
            <input
              type="date"
              value={params.get("from") ?? ""}
              onChange={(e) => change("from", e.target.value)}
            />
          </RaceField>
          <RaceField label="Through">
            <input
              type="date"
              value={params.get("to") ?? ""}
              onChange={(e) => change("to", e.target.value)}
            />
          </RaceField>
        </>
      )}
    </div>
  );
}
function RaceList({ view }: { view: "upcoming" | "history" }) {
  const [params, setParams] = useSearchParams();
  const query = new URLSearchParams(params);
  query.set("view", view);
  query.set("limit", "50");
  const races = useQuery({
    queryKey: ["races", "list", query.toString()],
    queryFn: () => raceApi.list(query.toString()),
  });
  const page = (offset: number) => {
    const next = new URLSearchParams(params);
    next.set("offset", String(offset));
    setParams(next);
  };
  return (
    <section className="panel race-section">
      <h2>{view === "upcoming" ? "Upcoming races" : "Race history"}</h2>
      <RaceFilters />
      <RaceError error={races.error} />
      {races.isLoading && <RaceLoading />}
      {races.data && !races.data.races.length && (
        <p className="muted">
          No races match this view. Create a race or review imported activities.
        </p>
      )}
      <div className="race-table-wrap">
        <table className="race-table">
          <thead>
            <tr>
              <th>Race</th>
              <th>Date</th>
              <th>Distance</th>
              <th>Status</th>
              <th>{view === "history" ? "Result" : "Preparation"}</th>
            </tr>
          </thead>
          <tbody>
            {races.data?.races.map((r) => (
              <tr key={r.id}>
                <td>
                  <Link to={`/races/${r.id}`}>{r.name}</Link>
                  <small>
                    {[r.priority, raceLabel(r.discipline), raceLabel(r.kind)]
                      .filter(Boolean)
                      .join(" · ")}
                  </small>
                </td>
                <td>
                  {r.date}
                  <small>{raceCountdown(r)}</small>
                </td>
                <td>{raceDistance(r.distanceM)}</td>
                <td>{raceLabel(r.status)}</td>
                <td>
                  {view === "history" ? (
                    <>
                      {formatRaceTime(resultTime(r))}
                      {resultTime(r) !== undefined && (
                        <small>
                          {r.result.confirmed ? "Confirmed" : "Unconfirmed"}
                          {r.result.excluded ? " · Excluded" : ""}
                        </small>
                      )}
                    </>
                  ) : (
                    <>
                      {r.checklist.length > 0 &&
                        `${r.checklist.filter((i) => i.done).length}/${r.checklist.length} complete`}
                      {r.registrationDeadline && (
                        <small>Register by {r.registrationDeadline}</small>
                      )}
                    </>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="race-actions">
        <button
          className="secondary-button small-button"
          disabled={!Number(params.get("offset"))}
          onClick={() => page(Math.max(0, Number(params.get("offset")) - 50))}
        >
          Previous
        </button>
        <button
          className="secondary-button small-button"
          disabled={!races.data?.hasMore}
          onClick={() => page(races.data?.nextOffset ?? 0)}
        >
          Next
        </button>
      </div>
    </section>
  );
}
export function RaceJobs({ canWrite }: { canWrite: boolean }) {
  const invalidate = useInvalidateRaces();
  const jobs = useQuery({
    queryKey: ["sync-jobs"],
    queryFn: () => api.syncJobs(),
    refetchInterval: 4000,
  });
  const cancel = useMutation({
    mutationFn: api.cancelSyncJob,
    onSuccess: invalidate,
  });
  const relevant =
    jobs.data?.jobs
      ?.filter((j) => j.provider === "races" || j.kind === "race_predictions")
      .slice(0, 3) ?? [];
  return (
    <>
      <RaceError error={jobs.error ?? cancel.error} />
      {relevant.map((j) => (
        <p className="muted" key={j.id}>
          {j.provider === "races" ? "Historical race scan" : "Prediction sync"}:{" "}
          {j.status}
          {j.error && ` · ${j.error}`}
          {typeof j.payload?.processed === "number" &&
            ` · ${j.payload.processed} activities scanned`}{" "}
          {canWrite && ["running", "queued"].includes(j.status) && (
            <button
              className="secondary-button small-button"
              disabled={cancel.isPending || !!j.cancelRequestedAt}
              onClick={() => cancel.mutate(j.id)}
            >
              {j.cancelRequestedAt ? "Cancelling…" : "Cancel job"}
            </button>
          )}
        </p>
      ))}
    </>
  );
}
function RaceReview({ canWrite }: { canWrite: boolean }) {
  const [offset, setOffset] = useState(0);
  const invalidate = useInvalidateRaces();
  const review = useQuery({
    queryKey: ["races", "review", offset],
    queryFn: () => raceApi.review(offset),
    refetchInterval: 5000,
  });
  const scan = useMutation({ mutationFn: raceApi.scan, onSuccess: invalidate });
  return (
    <section className="panel race-section">
      <div className="race-heading">
        <h2>Review race suggestions</h2>
        {canWrite && (
          <button
            className="secondary-button"
            disabled={scan.isPending}
            onClick={() => scan.mutate()}
          >
            Scan historical activities
          </button>
        )}
      </div>
      <p className="muted">
        New imports are checked automatically. Confirm each suggestion or
        dismiss it to keep it out of future scans.
      </p>
      <RaceJobs canWrite={canWrite} />
      <RaceError error={review.error ?? scan.error} />
      {review.isLoading && <RaceLoading />}
      {review.data?.activities.length === 0 && <p>No suggestions to review.</p>}
      {review.data?.activities.map((candidate) => (
        <ReviewCandidate
          key={candidate.id}
          candidate={candidate}
          canWrite={canWrite}
        />
      ))}
      <div className="race-actions">
        <button
          className="secondary-button small-button"
          disabled={!offset}
          onClick={() => setOffset(Math.max(0, offset - 50))}
        >
          Previous
        </button>
        <button
          className="secondary-button small-button"
          disabled={!review.data?.hasMore}
          onClick={() => setOffset(offset + 50)}
        >
          Next
        </button>
      </div>
    </section>
  );
}
function ReviewCandidate({
  candidate: c,
  canWrite,
}: {
  candidate: RaceCandidate;
  canWrite: boolean;
}) {
  const [existing, setExisting] = useState("");
  const invalidate = useInvalidateRaces();
  const choices = useQuery({
    queryKey: ["races", "review-link-options"],
    queryFn: () => raceApi.list("view=upcoming"),
    enabled: canWrite,
  });
  const dismiss = useMutation({
    mutationFn: () => raceApi.dismiss(c.id),
    onSuccess: invalidate,
  });
  const link = useMutation({
    mutationFn: async () => {
      const { race } = await raceApi.detail(existing);
      return raceApi.save(existing, {
        ...raceInput(race),
        activityId: c.id,
        raceOnly: false,
      });
    },
    onSuccess: invalidate,
  });
  return (
    <article className="race-candidate">
      <h3>
        <Link to={`/activities/${c.id}`}>{c.name}</Link>
      </h3>
      <p>
        {c.startTime.slice(0, 10)} · {raceDistance(c.distanceM)} ·{" "}
        {formatRaceTime(c.elapsedTimeS * 1000)} elapsed
      </p>
      <p className="muted">{c.reasons.join(" · ")}</p>
      {canWrite && (
        <div className="race-actions">
          <Link
            className="primary-button small-button"
            to={`/races/new?activityId=${c.id}`}
          >
            Create race
          </Link>
          <RaceField label="Link to existing race">
            <select
              value={existing}
              onChange={(e) => setExisting(e.target.value)}
            >
              <option value="">Choose a race</option>
              {choices.data?.races
                .filter((r) => !r.activityId)
                .map((r) => (
                  <option key={r.id} value={r.id}>
                    {r.name}
                  </option>
                ))}
            </select>
          </RaceField>
          <button
            className="secondary-button small-button"
            disabled={!existing || link.isPending}
            onClick={() => link.mutate()}
          >
            Confirm link
          </button>
          <button
            className="secondary-button small-button"
            disabled={dismiss.isPending}
            onClick={() => dismiss.mutate()}
          >
            Dismiss
          </button>
        </div>
      )}
      <RaceError error={dismiss.error ?? link.error ?? choices.error} />
    </article>
  );
}
