import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { raceApi } from "./api";
import {
  RaceError,
  RaceField,
  RaceLoading,
  useInvalidateRaces,
} from "./components";
import { RaceFilters } from "./RacesPage";
import {
  distancePresets,
  formatRaceTime,
  raceDistance,
  raceLabel,
} from "./utils";

export function RacePerformancePage({ canWrite }: { canWrite: boolean }) {
  const [params] = useSearchParams();
  const [series, setSeries] = useState("");
  const [predictionDistance, setPredictionDistance] = useState(5000);
  const data = useQuery({
    queryKey: ["races", "performance", params.toString()],
    queryFn: () => raceApi.performance(params.toString()),
  });
  const settings = useQuery({
    queryKey: ["races", "settings"],
    queryFn: raceApi.settings,
  });
  const invalidate = useInvalidateRaces();
  const reference = useMutation({
    mutationFn: (id: string) =>
      raceApi.saveSettings({
        ...settings.data!,
        vdotReferenceId: id || undefined,
      }),
    onSuccess: invalidate,
  });
  const eligible =
    data.data?.results.filter(
      (r) =>
        r.metrics.eligible &&
        r.date &&
        r.date <= new Date().toISOString().slice(0, 10),
    ) ?? [];
  const options = [
    ...new Map(
      eligible.map((r) => [
        `${r.discipline}:${r.distanceM}`,
        `${raceLabel(r.discipline)} · ${raceDistance(r.distanceM)}`,
      ]),
    ).entries(),
  ];
  const selectedSeries = options.some(([key]) => key === series)
    ? series
    : options[0]?.[0];
  const points = eligible
    .filter((r) => `${r.discipline}:${r.distanceM}` === selectedSeries)
    .sort((a, b) => (a.date ?? "").localeCompare(b.date ?? ""))
    .map((r) => ({
      ...r,
      seconds: r.metrics.timeMs! / 1000,
      vdot: r.metrics.vdot,
      ageGrade: r.metrics.ageGrade,
    }));
  const current = data.data?.reference;
  return (
    <>
      <section className="panel race-section">
        <h2>Current equivalents</h2>
        <RaceError error={data.error ?? settings.error ?? reference.error} />
        {data.isLoading && <RaceLoading />}
        {current ? (
          <>
            <p>
              <Link to={`/races/${current.id}`}>{current.name}</Link> ·{" "}
              {current.date} · VDOT {current.metrics.vdot?.toFixed(1)}
            </p>
            <div className="race-metrics">
              {current.metrics.equivalents?.map((e) => (
                <div key={e.race}>
                  <span>{e.race}</span>
                  <strong>{e.timeLabel}</strong>
                </div>
              ))}
            </div>
          </>
        ) : (
          <p className="muted">{data.data?.referenceReason}</p>
        )}
        <p className="muted">
          Automatic reference: best eligible road/track VDOT from the last 90
          days. These are performance equivalents, not course-specific
          forecasts.
        </p>
        {canWrite && (
          <RaceField label="VDOT reference">
            <select
              disabled={!settings.data || reference.isPending}
              value={settings.data?.vdotReferenceId ?? ""}
              onChange={(e) => reference.mutate(e.target.value)}
            >
              <option value="">Automatic · best in last 90 days</option>
              {settings.data?.vdotReferenceId &&
                !eligible.some(
                  (r) => r.id === settings.data?.vdotReferenceId,
                ) && (
                  <option value={settings.data.vdotReferenceId}>
                    {current?.name ??
                      "Selected reference (outside filters or ineligible)"}
                  </option>
                )}
              {eligible
                .filter((r) => r.metrics.vdot !== undefined)
                .map((r) => (
                  <option key={r.id} value={r.id}>
                    {r.date} · {r.name} · {r.metrics.vdot?.toFixed(1)}
                  </option>
                ))}
            </select>
          </RaceField>
        )}
      </section>
      <section className="panel race-section">
        <h2>Results and bests</h2>
        <RaceFilters />
        <RaceField label="Result series">
          <select
            value={selectedSeries ?? ""}
            onChange={(e) => setSeries(e.target.value)}
          >
            {options.map(([key, label]) => (
              <option key={key} value={key}>
                {label}
              </option>
            ))}
          </select>
        </RaceField>
        {!eligible.length && !data.isLoading && (
          <p className="muted">
            Confirm a finished race result to include it here. Excluded results
            do not affect comparisons.
          </p>
        )}
        {points.length > 0 && (
          <>
            <RaceTrend data={points} field="seconds" label="Finish time" time />
            <div className="race-chart-pair">
              <RaceTrend data={points} field="vdot" label="VDOT" />
              <RaceTrend data={points} field="ageGrade" label="Age grade (%)" />
            </div>
          </>
        )}
        <div className="race-table-wrap">
          <table className="race-table">
            <thead>
              <tr>
                <th>Race</th>
                <th>Date</th>
                <th>Distance / discipline</th>
                <th>Result</th>
                <th>Bests</th>
                <th>VDOT</th>
                <th>Age grade</th>
              </tr>
            </thead>
            <tbody>
              {data.data?.results.map((r) => (
                <tr key={r.id}>
                  <td>
                    <Link to={`/races/${r.id}`}>{r.name}</Link>
                  </td>
                  <td>{r.date}</td>
                  <td>
                    {raceDistance(r.distanceM)} · {raceLabel(r.discipline)}
                  </td>
                  <td>
                    {formatRaceTime(r.metrics.timeMs)}
                    <small>{r.metrics.basis}</small>
                  </td>
                  <td>
                    {[
                      r.personalBest ? "PB" : "",
                      r.seasonBest ? `${r.date?.slice(0, 4)} best` : "",
                    ]
                      .filter(Boolean)
                      .join(" · ")}
                  </td>
                  <td title={r.metrics.vdotReason}>
                    {r.metrics.vdot?.toFixed(1)}
                  </td>
                  <td title={r.metrics.ageGradeReason}>
                    {r.metrics.ageGrade !== undefined &&
                      `${r.metrics.ageGrade.toFixed(1)}%`}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="muted">
          PBs use exact distance and discipline within these filters. Season
          bests use calendar years. Age grading: {data.data?.tableVersion}.
        </p>
      </section>
      <section className="panel race-section">
        <h2>Garmin prediction history</h2>
        <RaceField label="Prediction distance">
          <select
            value={predictionDistance}
            onChange={(e) => setPredictionDistance(Number(e.target.value))}
          >
            {distancePresets
              .filter((d) =>
                [5000, 10000, 21097.5, 42195].includes(d.distanceM),
              )
              .map((d) => (
                <option key={d.distanceM} value={d.distanceM}>
                  {d.label}
                </option>
              ))}
          </select>
        </RaceField>
        <RaceTrend
          data={(data.data?.predictions ?? [])
            .filter((p) => p.distanceM === predictionDistance)
            .map((p) => ({
              date: p.date,
              seconds: p.timeMs === undefined ? undefined : p.timeMs / 1000,
            }))}
          field="seconds"
          label="Predicted time"
          time
        />
        <p className="muted">
          Missing predictions stay blank. Enable prediction sync in{" "}
          <Link to="/races/settings">race settings</Link>. Race details show
          retrieval dates and whether a prediction was imported after race day.
        </p>
      </section>
    </>
  );
}
function RaceTrend({
  data,
  field,
  label,
  time = false,
}: {
  data: Array<Record<string, unknown>>;
  field: string;
  label: string;
  time?: boolean;
}) {
  const display = (value: number) =>
    time ? formatRaceTime(value * 1000) : value.toFixed(1);
  if (!data.some((d) => d[field] !== undefined && d[field] !== null))
    return <p className="muted">No {label.toLowerCase()} data available.</p>;
  return (
    <figure className="race-chart">
      <figcaption>{label}</figcaption>
      <ResponsiveContainer width="100%" height={230}>
        <LineChart data={data}>
          <CartesianGrid strokeDasharray="3 3" />
          <XAxis dataKey="date" minTickGap={40} />
          <YAxis domain={["auto", "auto"]} width={76} tickFormatter={display} />
          <Tooltip
            formatter={(v) => (v === undefined ? "" : display(Number(v)))}
          />
          <Line
            dataKey={field}
            name={label}
            type="linear"
            stroke="var(--color-primary, #397c9c)"
            connectNulls={false}
            isAnimationActive={false}
          />
        </LineChart>
      </ResponsiveContainer>
    </figure>
  );
}
