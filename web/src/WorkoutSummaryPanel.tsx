import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "./api";
import type { Activity } from "./types";
import { buildWorkoutSummary, summaryDuration, summaryPace, summaryValue } from "./workoutSummary";

export function WorkoutSummaryPanel({ activity, matchedWorkoutId, matchLoading, matchError, retryMatch }: {
  activity: Activity;
  matchedWorkoutId?: string;
  matchLoading: boolean;
  matchError: boolean;
  retryMatch: () => void;
}) {
  const workout = useQuery({ queryKey: ["workout", matchedWorkoutId], queryFn: () => api.workout(matchedWorkoutId!), enabled: Boolean(matchedWorkoutId) });
  const config = useQuery({ queryKey: ["workout-config"], queryFn: api.workoutConfig, enabled: Boolean(matchedWorkoutId) });
  const summary = useMemo(() => buildWorkoutSummary(activity, workout.data, config.data?.defaultPaceToleranceSeconds), [activity, workout.data, config.data]);
  const loading = matchLoading || Boolean(matchedWorkoutId && workout.isPending);
  const hasHeartRate = summary?.rows.some((row) => row.actual?.avgHeartRate !== undefined);
  const hasPower = summary?.rows.some((row) => row.actual?.avgPower !== undefined);

  return <section className="panel activity-workout-summary" aria-label="Workout summary">
    <div className="intervals-header">
      <div><div className="panel-heading">Workout summary</div>{!loading && summary && <><strong>{summary.name}</strong><p className="muted">{summary.source === "matched" ? "Goals from matched workout" : "Goals from imported workout"}</p></>}</div>
      {matchedWorkoutId && <Link className="secondary-button small-button" to={`/workouts/${matchedWorkoutId}`}>View workout</Link>}
    </div>
    {loading ? <p role="status">Loading workout goals…</p> : <>
      {(matchError || workout.isError) && <div className="error" role="alert">
        Could not load the matched workout.{summary?.source === "imported" ? " Showing the imported workout." : workout.data ? " Showing the last loaded prescription." : " Retry to load its goals."}
        <button className="secondary-button small-button" type="button" onClick={() => { if (matchError) retryMatch(); else void workout.refetch(); }}>Retry workout</button>
      </div>}
      {summary?.source === "matched" && config.isError && workout.data?.paceToleranceSeconds === undefined && <div className="error" role="alert">Could not load the default pace tolerance. Exact pace targets remain unscored. <button className="secondary-button small-button" type="button" onClick={() => void config.refetch()}>Retry tolerance</button></div>}
      {summary ? <>
        {summary.prescription && <p className="workout-summary-prescription">{summary.prescription}</p>}
        {summary.warnings.map((warning, i) => <p className="muted" key={i}>{warning}</p>)}
        <div className="workout-summary-score" aria-label="Execution score">
          <span>Execution score <strong>{summary.score === undefined ? "Unavailable" : `${Math.round(summary.score)}/100`}</strong></span>
          <span>{summary.scoredSteps} of {summary.workSteps} work steps scored{summary.scoredSteps < summary.workSteps ? " · Partial coverage" : ""}</span>
        </div>
        <details className="workout-score-explanation">
          <summary>How the score is calculated</summary>
          <p>Runnarr averages the scores of comparable work steps equally. Warm-up, recovery, cooldown, and additional recorded intervals do not affect the overall score.</p>
          <p>Each numeric goal scores 100 within its target range. Outside the range, the percentage deviation from the nearest boundary is subtracted, down to zero. For example, 4:30 against a 5:00 duration goal scores 90. A step’s score is the average of its goal scores.</p>
          <p>Every prescribed goal must have a supported numeric target and reliable recorded data for the step to be scored. Missing or ambiguous results are excluded; coverage shows how much of the workout was scored. This compares interval averages, not time spent within target.</p>
          {summary.source === "matched" && <p>Goals use the current matched prescription.{summary.paceTolerance !== undefined ? ` Exact pace targets use a tolerance of ±${summary.paceTolerance} seconds/km.` : ""} Explicit pace ranges are used as written.</p>}
        </details>
        {summary.rows.some((row) => row.step) ? null : <p className="muted">No usable workout steps are available for comparison.</p>}
        {!summary.rows.some((row) => row.actual) && <p className="muted">No recorded intervals or workout laps are available.</p>}
        {summary.rows.length > 0 && <div className="table-wrap" tabIndex={0} role="region" aria-label="Workout step comparisons">
          <table className="data-table workout-summary-table">
            <thead><tr><th scope="col">Step</th><th scope="col">Duration / distance goal</th><th scope="col">Intensity goal</th><th scope="col">Actual time</th><th scope="col">Actual distance</th><th scope="col">Avg pace</th>{hasHeartRate && <th scope="col">Avg HR</th>}{hasPower && <th scope="col">Avg power</th>}<th scope="col">Adherence</th></tr></thead>
            <tbody>{summary.rows.map((row, i) => <tr key={i}>
              <th scope="row"><span>{row.step?.label ?? `Additional interval ${row.actual!.index + 1}`}</span>{row.step?.description && <small>{row.step.description}</small>}</th>
              <td>{row.step?.condition}</td><td>{row.step?.target}</td>
              <td>{row.actual && summaryValue("time", summaryDuration(row.actual))}</td>
              <td>{row.actual && summaryValue("distance", row.actual.distanceM)}</td>
              <td>{row.actual && summaryValue("pace", summaryPace(row.actual))}</td>
              {hasHeartRate && <td>{row.actual && summaryValue("heartRate", row.actual.avgHeartRate)}</td>}
              {hasPower && <td>{row.actual && summaryValue("power", row.actual.avgPower)}</td>}
              <td>{row.score === undefined ? <span className="muted">{row.status}</span> : `${Math.round(row.score)}/100`}</td>
            </tr>)}</tbody>
          </table>
        </div>}
      </> : !workout.isError && !matchError && <p className="muted">No workout goals are available.</p>}
    </>}
  </section>;
}
