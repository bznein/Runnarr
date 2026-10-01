import { useState, type ReactNode } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import type { Course } from "../types";
import { raceApi } from "./api";
import {
  RaceChecklist,
  RaceError,
  RaceField,
  RaceLoading,
  useInvalidateRaces,
} from "./components";
import {
  formatRaceTime,
  raceCountdown,
  raceDistance,
  raceInput,
  raceLabel,
} from "./utils";
import type { Race, RaceInput } from "./types";

export function RaceDetailPage({
  canWrite,
  renderCourse,
}: {
  canWrite: boolean;
  renderCourse: (course: Course) => ReactNode;
}) {
  const { id } = useParams();
  const navigate = useNavigate();
  const invalidate = useInvalidateRaces();
  const detail = useQuery({
    queryKey: ["races", "detail", id],
    queryFn: () => raceApi.detail(id!),
  });
  const save = useMutation({
    mutationFn: (input: RaceInput) => raceApi.save(id!, input),
    onSuccess: invalidate,
  });
  const remove = useMutation({
    mutationFn: () => raceApi.delete(id!, detail.data!.race.revision),
    onSuccess: async () => {
      await invalidate();
      navigate("/races");
    },
  });
  const forecast = useMutation({
    mutationFn: () => raceApi.forecast(id!),
    onSuccess: invalidate,
  });
  const r = detail.data?.race;
  const d = detail.data;
  const editions = useQuery({
    queryKey: ["races", "editions", r?.groupId],
    queryFn: () =>
      raceApi.list(`groupId=${encodeURIComponent(r!.groupId!)}&view=all`),
    enabled: !!r?.groupId,
  });
  if (detail.isLoading) return <RaceLoading />;
  if (!r || !d) return <RaceError error={detail.error} />;
  const num = (v: number | undefined, suffix: string) =>
    v === undefined ? "" : `${v.toFixed(1)}${suffix}`;
  return (
    <>
      <section className="panel race-section">
        <div className="race-heading">
          <div>
            <h2>{r.name}</h2>
            <p className="muted">
              {[
                r.date,
                r.startTime,
                r.startTime ? r.timezone : "",
                raceDistance(r.distanceM),
                raceLabel(r.discipline),
                raceLabel(r.kind),
              ]
                .filter(Boolean)
                .join(" · ")}
            </p>
          </div>
          <div className="race-actions">
            {canWrite && (
              <Link className="secondary-button" to={`/races/${r.id}/edit`}>
                Edit race
              </Link>
            )}
            <Link className="secondary-button" to={`/races/${r.id}/report`}>
              Race report
            </Link>
          </div>
        </div>
        <p>
          <span className="race-badge">{raceLabel(r.status)}</span>{" "}
          {[r.priority ? `Priority ${r.priority}` : "", raceCountdown(r)]
            .filter(Boolean)
            .join(" · ")}
        </p>
        <RaceError error={save.error ?? remove.error} />
        <div className="race-metrics">
          {(
            [
              ["Chip time", r.result.chipTimeMs],
              ["Gun time", r.result.gunTimeMs],
              ["Manual time", r.result.manualTimeMs],
            ] as const
          )
            .filter(([, v]) => v !== undefined)
            .map(([label, value]) => (
              <div key={label}>
                <span>{label}</span>
                <strong>{formatRaceTime(value)}</strong>
              </div>
            ))}
          {r.result.overallPlace !== undefined && (
            <div>
              <span>Overall place</span>
              <strong>
                {r.result.overallPlace}
                {r.result.overallTotal && ` / ${r.result.overallTotal}`}
              </strong>
            </div>
          )}
          {r.result.categoryPlace !== undefined && (
            <div>
              <span>{r.result.category || "Category"} place</span>
              <strong>
                {r.result.categoryPlace}
                {r.result.categoryTotal && ` / ${r.result.categoryTotal}`}
              </strong>
            </div>
          )}
          {d.metrics.vdot !== undefined && (
            <div>
              <span>VDOT</span>
              <strong>{d.metrics.vdot.toFixed(1)}</strong>
            </div>
          )}
          {d.metrics.ageGrade !== undefined && (
            <div>
              <span>Age grade</span>
              <strong>{d.metrics.ageGrade.toFixed(1)}%</strong>
            </div>
          )}
        </div>
        {!r.result.confirmed && (
          <p className="muted">Result is not confirmed.</p>
        )}
        {r.result.excluded && (
          <p className="muted">
            Excluded from performance comparisons
            {r.result.exclusionReason && `: ${r.result.exclusionReason}`}
          </p>
        )}
        <p className="muted">{d.metrics.vdotReason}</p>
        {d.metrics.ageGradeReason !== d.metrics.vdotReason && (
          <p className="muted">
            {d.metrics.ageGradeReason ??
              `Age-grade table: ${d.metrics.tableVersion}`}
          </p>
        )}
        {r.goals.length > 0 && (
          <>
            <h3>Goals</h3>
            <ul>
              {r.goals.map((g, i) => {
                const achieved =
                  g.timeMs !== undefined
                    ? d.metrics.timeMs !== undefined && r.status === "finished"
                      ? d.metrics.timeMs <= g.timeMs
                      : undefined
                    : g.achieved;
                return (
                  <li key={i}>
                    {g.name}
                    {g.timeMs !== undefined && ` · ${formatRaceTime(g.timeMs)}`}
                    {achieved !== undefined &&
                      ` · ${achieved ? "Achieved" : "Not achieved"}`}
                  </li>
                );
              })}
            </ul>
          </>
        )}
        {r.checklist.length > 0 && (
          <>
            <h3>Preparation</h3>
            <RaceChecklist
              items={r.checklist}
              date={r.date}
              timezone={r.timezone}
              disabled={!canWrite || save.isPending}
              onChange={async (checklist) => {
                await save.mutateAsync({ ...raceInput(r), checklist });
              }}
            />
          </>
        )}
        <dl className="race-notes">
          {(
            [
              ["location", "Location"],
              ["registrationDeadline", "Registration deadline"],
              ["registrationNotes", "Registration"],
              ["bib", "Bib number"],
              ["startDetails", "Start / bib collection"],
              ["travelNotes", "Travel"],
              ["fuelingNotes", "Fueling"],
              ["pacingNotes", "Pacing"],
              ["kitNotes", "Kit"],
              ["notes", "Notes"],
            ] as const
          )
            .filter(([key]) => r[key])
            .map(([key, label]) => (
              <div key={key}>
                <dt>{label}</dt>
                <dd>{r[key]}</dd>
              </div>
            ))}
        </dl>
        <div className="race-actions">
          {r.eventUrl && (
            <a href={r.eventUrl} target="_blank" rel="noreferrer">
              Event website
            </a>
          )}
          {r.resultsUrl && (
            <a href={r.resultsUrl} target="_blank" rel="noreferrer">
              Official results
            </a>
          )}
          {r.plannedActivityId && (
            <Link to={`/activities/${r.plannedActivityId}`}>
              Linked training plan
            </Link>
          )}
        </div>
        {d.planWarning && <p className="muted">{d.planWarning}</p>}
      </section>
      <section className="panel race-section">
        <h2>Halfway analysis</h2>
        {d.halfway.firstHalfMs !== undefined ? (
          <>
            <div className="race-metrics">
              <div>
                <span>First half</span>
                <strong>{formatRaceTime(d.halfway.firstHalfMs)}</strong>
              </div>
              {d.halfway.secondHalfMs !== undefined && (
                <>
                  <div>
                    <span>Second half</span>
                    <strong>{formatRaceTime(d.halfway.secondHalfMs)}</strong>
                  </div>
                  <div>
                    <span>Second minus first</span>
                    <strong>
                      {d.halfway.differenceMs! > 0 ? "+" : ""}
                      {formatRaceTime(d.halfway.differenceMs)}
                    </strong>
                  </div>
                </>
              )}
            </div>
            <p className="muted">
              {d.halfway.source === "official"
                ? "Official midpoint checkpoint"
                : "Estimated midpoint of the complete recorded distance"}{" "}
              · {d.halfway.basis}
              {d.halfway.scaled
                ? " · Elapsed halves scaled to the confirmed finish time"
                : ""}
            </p>
            {d.halfway.source === "estimated" && (
              <p className="muted">
                Uses full recording samples and elapsed time, including stops.
                GPS distance is scaled to race distance; this is not an official
                course timing point.
              </p>
            )}
          </>
        ) : (
          <p className="muted">{d.halfway.reason}</p>
        )}
        {r.result.checkpoints.length > 0 && (
          <>
            <h3>Official checkpoints · {r.result.splitBasis} time</h3>
            <div className="race-table-wrap">
              <table className="race-table">
                <thead>
                  <tr>
                    <th>Checkpoint</th>
                    <th>Distance</th>
                    <th>Cumulative time</th>
                  </tr>
                </thead>
                <tbody>
                  {r.result.checkpoints.map((p, i) => (
                    <tr key={i}>
                      <td>{p.name}</td>
                      <td>{raceDistance(p.distanceM)}</td>
                      <td>{formatRaceTime(p.timeMs)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </section>
      <section className="panel race-section">
        <h2>Recorded activity</h2>
        {d.activity ? (
          <>
            <p>
              <Link to={`/activities/${d.activity.id}`}>{d.activity.name}</Link>{" "}
              · {raceDistance(d.activity.distanceM)} ·{" "}
              {formatRaceTime(d.activity.elapsedTimeS * 1000)} elapsed ·{" "}
              {formatRaceTime(d.activity.movingTimeS * 1000)} moving
            </p>
            {!r.raceOnly && canWrite && (
              <button
                className="secondary-button"
                disabled={save.isPending}
                onClick={() => save.mutate({ ...raceInput(r), raceOnly: true })}
              >
                Confirm complete recording contains only the race
              </button>
            )}
            {!!d.activity.laps?.length && (
              <details>
                <summary>Watch laps</summary>
                <div className="race-table-wrap">
                  <table className="race-table">
                    <thead>
                      <tr>
                        <th>Lap</th>
                        <th>Distance</th>
                        <th>Elapsed time</th>
                      </tr>
                    </thead>
                    <tbody>
                      {d.activity.laps?.map((lap, i) => (
                        <tr key={i}>
                          <td>{i + 1}</td>
                          <td>{raceDistance(lap.distanceM)}</td>
                          <td>{formatRaceTime(lap.elapsedTimeS * 1000)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </details>
            )}
            {d.activity.weather && (
              <p className="muted">
                Observed activity weather is available on the{" "}
                <Link to={`/activities/${d.activity.id}`}>activity detail</Link>
                .
              </p>
            )}
          </>
        ) : canWrite ? (
          <ActivityPicker race={r} />
        ) : (
          <p className="muted">No linked activity.</p>
        )}
      </section>
      {r.courseSnapshot && (
        <section className="panel race-section">
          <h2>Saved course · {r.courseSnapshot.name}</h2>
          <p className="muted">
            {raceDistance(r.courseSnapshot.distanceM)} · Race-specific geometry
            snapshot
            {r.courseId && (
              <>
                {" "}
                · <Link to={`/courses/${r.courseId}`}>Course library</Link>
              </>
            )}
          </p>
          {renderCourse(r.courseSnapshot)}
        </section>
      )}
      {d.buildUp.length > 0 && (
        <section className="panel race-section">
          <h2>12-week training build-up</h2>
          <p className="muted">
            Recorded running before race day, grouped in the event timezone.
            Race day is excluded.
          </p>
          <div className="race-chart">
            <ResponsiveContainer width="100%" height={220}>
              <BarChart
                data={d.buildUp.map((w) => ({
                  ...w,
                  km: Number((w.distanceM / 1000).toFixed(1)),
                }))}
              >
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="from" minTickGap={40} />
                <YAxis />
                <Tooltip />
                <Bar
                  dataKey="km"
                  name="Running (km)"
                  fill="var(--color-primary, #397c9c)"
                  isAnimationActive={false}
                />
              </BarChart>
            </ResponsiveContainer>
          </div>
          <div className="race-table-wrap">
            <table className="race-table">
              <thead>
                <tr>
                  <th>Week</th>
                  <th>Running</th>
                  <th>Moving time</th>
                  <th>Elevation</th>
                  <th>Runs</th>
                  <th>Longest</th>
                </tr>
              </thead>
              <tbody>
                {d.buildUp.map((w) => (
                  <tr key={w.from}>
                    <td>
                      <Link
                        to={`/activities?dateFrom=${w.from}&dateTo=${w.to}&sport=Run&sport=Treadmill+Run`}
                      >
                        {w.from} – {w.to}
                      </Link>
                    </td>
                    <td>{raceDistance(w.distanceM)}</td>
                    <td>{formatRaceTime(w.durationS * 1000)}</td>
                    <td>{Math.round(w.elevationM)} m</td>
                    <td>{w.count}</td>
                    <td>{raceDistance(w.longestM)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
      <section className="panel race-section">
        <div className="race-heading">
          <h2>Race-day forecast</h2>
          {canWrite && (
            <button
              className="secondary-button small-button"
              disabled={forecast.isPending}
              onClick={() => forecast.mutate()}
            >
              Refresh forecast
            </button>
          )}
        </div>
        <RaceError error={forecast.error} />
        {forecast.data?.reason && (
          <p role="status" className="muted">
            {forecast.data.reason}
          </p>
        )}
        {d.forecast.reason && <p className="muted">{d.forecast.reason}</p>}
        {d.forecast.error && <p className="error">{d.forecast.error}</p>}
        {d.forecast.fetchedAt && (
          <p className="muted">
            Fetched {new Date(d.forecast.fetchedAt).toLocaleString()} ·{" "}
            {d.forecast.saved
              ? "Saved pre-race forecast"
              : d.forecast.stale
                ? "Stale forecast"
                : "Current forecast"}
          </p>
        )}
        {d.forecast.hours.length > 0 && (
          <div className="race-table-wrap">
            <table className="race-table">
              <thead>
                <tr>
                  <th>Time · {r.timezone}</th>
                  <th>Temperature</th>
                  <th>Feels like</th>
                  <th>Humidity</th>
                  <th>Rain chance</th>
                  <th>Wind</th>
                  <th>Direction</th>
                </tr>
              </thead>
              <tbody>
                {d.forecast.hours.map((h) => (
                  <tr key={h.time}>
                    <td>
                      {new Date(h.time).toLocaleTimeString([], {
                        timeZone: r.timezone,
                        hour: "2-digit",
                        minute: "2-digit",
                      })}
                    </td>
                    <td>{num(h.temperatureC, " °C")}</td>
                    <td>{num(h.apparentTemperatureC, " °C")}</td>
                    <td>{num(h.humidity, "%")}</td>
                    <td>{num(h.rainProbability, "%")}</td>
                    <td>{num(h.windKph, " km/h")}</td>
                    <td>{num(h.windDirection, "°")}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <p className="muted">
          Weather data:{" "}
          <a href="https://open-meteo.com/" target="_blank" rel="noreferrer">
            Open-Meteo
          </a>
          , CC BY 4.0. Forecasts are separate from observed activity weather.
        </p>
      </section>
      <section className="panel race-section">
        <h2>Predictions and outcome</h2>
        {d.comparisons.length === 0 && (
          <p className="muted">
            No eligible pre-race prediction. Local VDOT comparisons must be
            captured before race day.
          </p>
        )}
        {d.comparisons.map((p) => (
          <div className="race-candidate" key={p.source}>
            <strong>
              {p.source} · {formatRaceTime(p.timeMs)}
            </strong>
            <p className="muted">
              Prediction date: {p.date}
              {p.backfilled &&
                " · Historical prediction retrieved after race start"}
              {p.capturedAt &&
                ` · Captured ${new Date(p.capturedAt).toLocaleString()}`}
              {p.fetchedAt &&
                ` · Retrieved ${new Date(p.fetchedAt).toLocaleString()}`}
              {p.version && ` · ${p.version}`}
              {p.referenceId && (
                <>
                  {" "}
                  · <Link to={`/races/${p.referenceId}`}>Reference race</Link>
                </>
              )}
            </p>
            {p.differenceMs !== undefined && (
              <p>
                Result minus prediction: {p.differenceMs > 0 ? "+" : ""}
                {formatRaceTime(p.differenceMs)}
              </p>
            )}
          </div>
        ))}
      </section>
      {r.groupId && (
        <section className="panel race-section">
          <h2>Same-event history</h2>
          <RaceError error={editions.error} />
          <p className="muted">
            Course and distance can change between editions; compare the saved
            course and race distance.
          </p>
          <ul>
            {editions.data?.races
              .filter((e) => e.id !== r.id)
              .map((e) => (
                <li key={e.id}>
                  <Link to={`/races/${e.id}`}>
                    {e.date} · {e.name}
                  </Link>{" "}
                  · {raceDistance(e.distanceM)}
                </li>
              ))}
          </ul>
          <Link to={`/races/history?groupId=${r.groupId}`}>
            All event results
          </Link>
        </section>
      )}
      {canWrite && (
        <button
          className="secondary-button race-delete"
          disabled={remove.isPending}
          onClick={() => {
            if (
              window.confirm(
                `Delete ${r.name} and its report? Linked activities, training plans and saved courses will remain.`,
              )
            )
              remove.mutate();
          }}
        >
          Delete race
        </button>
      )}
    </>
  );
}
function ActivityPicker({ race }: { race: Race }) {
  const [search, setSearch] = useState("");
  const invalidate = useInvalidateRaces();
  const candidates = useQuery({
    queryKey: ["races", "candidates", race.id, search],
    queryFn: () => raceApi.candidates(race.id, search),
  });
  const link = useMutation({
    mutationFn: (activityId: string) =>
      raceApi.save(race.id, {
        ...raceInput(race),
        activityId,
        raceOnly: false,
      }),
    onSuccess: invalidate,
  });
  return (
    <>
      <RaceField label="Find recorded activity">
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search all runs by name"
        />
      </RaceField>
      <p className="muted">
        Suggestions use date and distance. Confirm a whole recording to link it.
      </p>
      <RaceError error={candidates.error ?? link.error} />
      {candidates.isLoading && <RaceLoading />}
      {candidates.data?.activities.length === 0 && (
        <p className="muted">No matching activities. Try searching by name.</p>
      )}
      {candidates.data?.activities.map((a) => (
        <div className="race-candidate" key={a.id}>
          <Link to={`/activities/${a.id}`}>{a.name}</Link>
          <span>
            {a.startTime.slice(0, 10)} · {raceDistance(a.distanceM)} ·{" "}
            {a.reasons.join(" · ")}
          </span>
          <button
            className="secondary-button small-button"
            disabled={link.isPending}
            onClick={() => link.mutate(a.id)}
          >
            Confirm activity link
          </button>
        </div>
      ))}
    </>
  );
}
