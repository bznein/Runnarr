import { useState } from "react";
import {
  Link,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router-dom";
import { useMutation, useQuery } from "@tanstack/react-query";
import { api } from "../api";
import type { Activity } from "../types";
import { raceApi } from "./api";
import type { Race, RaceInput } from "./types";
import {
  RaceChecklistEditor,
  RaceError,
  RaceField,
  RaceLoading,
  RaceTimeInput,
  useInvalidateRaces,
} from "./components";
import {
  distancePresets,
  formatRaceTime,
  newRace,
  raceDisciplines,
  raceDistance,
  raceInput,
  raceKinds,
  raceLabel,
  raceStatuses,
} from "./utils";

export function RaceEditorPage({ canWrite }: { canWrite: boolean }) {
  const { id } = useParams();
  const [search] = useSearchParams();
  const activityId = search.get("activityId") ?? "";
  const detail = useQuery({
    queryKey: ["races", "detail", id],
    queryFn: () => raceApi.detail(id!),
    enabled: !!id,
  });
  const activity = useQuery({
    queryKey: ["activity", activityId],
    queryFn: () => api.activity(activityId),
    enabled: !!activityId && !id,
  });
  const error = detail.error ?? activity.error;
  if (!canWrite)
    return (
      <p className="muted">
        Race editing is unavailable in read-only support view.
      </p>
    );
  if ((id && detail.isLoading) || (activityId && activity.isLoading))
    return <RaceLoading />;
  if (error) return <RaceError error={error} />;
  return (
    <RaceEditor
      key={id ?? activityId ?? "new"}
      current={detail.data?.race}
      activity={detail.data?.activity ?? activity.data?.activity}
      initial={
        detail.data
          ? raceInput(detail.data.race)
          : newRace(activity.data?.activity)
      }
    />
  );
}
function RaceEditor({
  initial,
  current,
  activity,
}: {
  initial: RaceInput;
  current?: Race;
  activity?: Activity;
}) {
  const [draft, setDraft] = useState(initial);
  const [unit, setUnit] = useState("km");
  const [distanceText, setDistanceText] = useState(
    initial.distanceM === undefined ? "" : String(initial.distanceM / 1000),
  );
  const navigate = useNavigate();
  const invalidate = useInvalidateRaces();
  const groups = useQuery({
    queryKey: ["races", "groups"],
    queryFn: () => raceApi.resources("groups"),
  });
  const templates = useQuery({
    queryKey: ["races", "templates"],
    queryFn: () => raceApi.resources("checklists"),
  });
  const [courseSearch, setCourseSearch] = useState("");
  const courses = useQuery({
    queryKey: ["races", "course-picker", courseSearch],
    queryFn: () => api.courses({ sport: "Run", q: courseSearch }),
  });
  const course = useQuery({
    queryKey: ["races", "course", draft.courseId],
    queryFn: () => api.course(draft.courseId!),
    enabled: !!draft.courseId,
  });
  const courseStart = course.data?.waypoints[0] ?? course.data?.profile[0];
  const planFrom =
    draft.date && Number.isFinite(Date.parse(`${draft.date}T12:00:00Z`))
      ? new Date(Date.parse(`${draft.date}T12:00:00Z`) - 30 * 86400000)
          .toISOString()
          .slice(0, 10)
      : undefined;
  const planTo =
    draft.date && Number.isFinite(Date.parse(`${draft.date}T12:00:00Z`))
      ? new Date(Date.parse(`${draft.date}T12:00:00Z`) + 30 * 86400000)
          .toISOString()
          .slice(0, 10)
      : undefined;
  const plans = useQuery({
    queryKey: ["races", "plan-picker", planFrom, planTo],
    queryFn: () => api.plannedActivities(planFrom, planTo),
    enabled: !!draft.date,
  });
  const save = useMutation({
    mutationFn: () => raceApi.save(current?.id, draft),
    onSuccess: async (race) => {
      await invalidate();
      navigate(`/races/${race.id}`);
    },
  });
  const set = <K extends keyof RaceInput>(key: K, value: RaceInput[K]) =>
    setDraft((old) => ({ ...old, [key]: value }));
  const result = (patch: Partial<RaceInput["result"]>) =>
    setDraft((old) => ({ ...old, result: { ...old.result, ...patch } }));
  const numeric = (value: string) => (value === "" ? undefined : Number(value));
  const optionalText = (key: keyof RaceInput, label: string, type = "text") => (
    <RaceField label={label}>
      <input
        type={type}
        value={String(draft[key] ?? "")}
        maxLength={type === "url" ? 2048 : 5000}
        onChange={(e) => set(key, e.target.value)}
      />
    </RaceField>
  );
  return (
    <form
      className="race-editor"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <div className="race-heading">
        <h1>{current ? "Edit race" : "New race"}</h1>
        <Link
          className="secondary-button"
          to={current ? `/races/${current.id}` : "/races"}
        >
          Cancel
        </Link>
      </div>
      <RaceError error={save.error} />
      <fieldset className="panel race-form-section" disabled={save.isPending}>
        <legend>Race</legend>
        <div className="race-form-grid">
          <RaceField label="Race name">
            <input
              autoFocus
              required
              maxLength={160}
              value={draft.name}
              onChange={(e) => set("name", e.target.value)}
            />
          </RaceField>
          <RaceField label="Status">
            <select
              value={draft.status}
              onChange={(e) =>
                set("status", e.target.value as RaceInput["status"])
              }
            >
              {raceStatuses.map((v) => (
                <option key={v} value={v}>
                  {raceLabel(v)}
                </option>
              ))}
            </select>
          </RaceField>
          <RaceField label="Race date">
            <input
              type="date"
              min="1900-01-01"
              max="2200-12-31"
              value={draft.date ?? ""}
              onChange={(e) =>
                setDraft((old) => ({
                  ...old,
                  date: e.target.value || undefined,
                  startTime: e.target.value ? old.startTime : undefined,
                }))
              }
            />
          </RaceField>
          {optionalText("startTime", "Start time", "time")}
          <RaceField label="Event timezone">
            <input
              required
              value={draft.timezone}
              onChange={(e) => set("timezone", e.target.value)}
              placeholder="Europe/Dublin"
            />
          </RaceField>
          <RaceField label="Discipline">
            <select
              value={draft.discipline}
              onChange={(e) =>
                set("discipline", e.target.value as RaceInput["discipline"])
              }
            >
              {raceDisciplines.map((v) => (
                <option key={v} value={v}>
                  {raceLabel(v)}
                </option>
              ))}
            </select>
          </RaceField>
          <RaceField label="Event kind">
            <select
              value={draft.kind}
              onChange={(e) => set("kind", e.target.value as RaceInput["kind"])}
            >
              {raceKinds.map((v) => (
                <option key={v} value={v}>
                  {raceLabel(v)}
                </option>
              ))}
            </select>
          </RaceField>
          <RaceField label="Race importance">
            <select
              value={draft.priority ?? ""}
              onChange={(e) =>
                set("priority", e.target.value as RaceInput["priority"])
              }
            >
              <option value="">Unassigned</option>
              <option value="A">A · Main goal</option>
              <option value="B">B · Important</option>
              <option value="C">C · Supporting race</option>
            </select>
          </RaceField>
          <RaceField label="Distance preset">
            <select
              value={
                distancePresets.find((p) => p.distanceM === draft.distanceM)
                  ?.distanceM ?? ""
              }
              onChange={(e) => {
                const distance = numeric(e.target.value);
                set("distanceM", distance);
                setDistanceText(
                  distance === undefined
                    ? ""
                    : String(distance / (unit === "km" ? 1000 : 1609.344)),
                );
              }}
            >
              <option value="">Custom / undecided</option>
              {distancePresets.map((p) => (
                <option key={p.label} value={p.distanceM}>
                  {p.label}
                </option>
              ))}
            </select>
          </RaceField>
          <RaceField label="Race distance">
            <input
              type="number"
              step="any"
              min="0.000001"
              value={distanceText}
              onChange={(e) => {
                setDistanceText(e.target.value);
                set(
                  "distanceM",
                  e.target.value === ""
                    ? undefined
                    : Number(
                        (
                          Number(e.target.value) *
                          (unit === "km" ? 1000 : 1609.344)
                        ).toFixed(6),
                      ),
                );
              }}
            />
          </RaceField>
          <RaceField label="Distance unit">
            <select
              value={unit}
              onChange={(e) => {
                setUnit(e.target.value);
                setDistanceText(
                  draft.distanceM === undefined
                    ? ""
                    : String(
                        Number(
                          (
                            draft.distanceM /
                            (e.target.value === "km" ? 1000 : 1609.344)
                          ).toFixed(8),
                        ),
                      ),
                );
              }}
            >
              <option value="km">Kilometres</option>
              <option value="mi">Miles</option>
            </select>
          </RaceField>
          <RaceField label="Recurring event">
            <select
              value={draft.groupId ?? ""}
              onChange={(e) => set("groupId", e.target.value || undefined)}
            >
              <option value="">No event group</option>
              {groups.data?.items.map((g) => (
                <option key={g.id} value={g.id}>
                  {g.name}
                </option>
              ))}
            </select>
          </RaceField>
        </div>
        <p className="muted">
          Leave unknown dates and distances blank. Manage recurring event groups
          and checklist templates in{" "}
          <Link to="/races/settings">race settings</Link>.
        </p>
        <RaceError error={groups.error} />
      </fieldset>
      <fieldset className="panel race-form-section" disabled={save.isPending}>
        <legend>Results and official checkpoints</legend>
        {activity && (
          <p className="muted">
            Recorded activity: {raceDistance(activity.distanceM)} · elapsed{" "}
            {formatRaceTime(activity.elapsedTimeS * 1000)} · moving{" "}
            {formatRaceTime(activity.movingTimeS * 1000)}. Enter race results
            independently.
          </p>
        )}
        <div className="race-form-grid">
          <RaceTimeInput
            label="Chip time"
            value={draft.result.chipTimeMs}
            onChange={(chipTimeMs) => result({ chipTimeMs })}
          />
          <RaceTimeInput
            label="Gun time"
            value={draft.result.gunTimeMs}
            onChange={(gunTimeMs) => result({ gunTimeMs })}
          />
          <RaceTimeInput
            label="Manual finish time"
            value={draft.result.manualTimeMs}
            onChange={(manualTimeMs) => result({ manualTimeMs })}
          />
          {(
            [
              "overallPlace",
              "overallTotal",
              "categoryPlace",
              "categoryTotal",
            ] as const
          ).map((key) => (
            <RaceField
              label={
                {
                  overallPlace: "Overall place",
                  overallTotal: "Overall field size",
                  categoryPlace: "Category place",
                  categoryTotal: "Category field size",
                }[key]
              }
              key={key}
            >
              <input
                type="number"
                min="1"
                value={draft.result[key] ?? ""}
                onChange={(e) => result({ [key]: numeric(e.target.value) })}
              />
            </RaceField>
          ))}
          <RaceField label="Result category">
            <input
              maxLength={160}
              value={draft.result.category ?? ""}
              onChange={(e) => result({ category: e.target.value })}
            />
          </RaceField>
        </div>
        <label className="race-check">
          <input
            type="checkbox"
            checked={draft.result.confirmed}
            onChange={(e) => result({ confirmed: e.target.checked })}
          />
          I confirm the race distance and finish time
        </label>
        <label className="race-check">
          <input
            type="checkbox"
            checked={draft.result.excluded}
            onChange={(e) => result({ excluded: e.target.checked })}
          />
          Exclude this result from PBs and performance comparisons
        </label>
        {draft.result.excluded && (
          <RaceField label="Exclusion reason">
            <input
              maxLength={5000}
              value={draft.result.exclusionReason ?? ""}
              onChange={(e) => result({ exclusionReason: e.target.value })}
            />
          </RaceField>
        )}
        <RaceField label="Official checkpoint timing basis">
          <select
            value={draft.result.splitBasis ?? ""}
            onChange={(e) =>
              result({
                splitBasis: e.target.value as RaceInput["result"]["splitBasis"],
              })
            }
          >
            <option value="">Select when entering checkpoints</option>
            <option value="chip">Chip time</option>
            <option value="gun">Gun time</option>
            <option value="manual">Manual time</option>
          </select>
        </RaceField>
        {draft.result.checkpoints.map((point, i) => {
          const change = (patch: Partial<typeof point>) =>
            result({
              checkpoints: draft.result.checkpoints.map((p, j) =>
                j === i ? { ...p, ...patch } : p,
              ),
            });
          return (
            <div className="race-checkpoint-edit-row" key={i}>
              <RaceField label={`Checkpoint ${i + 1} name`}>
                <input
                  required
                  value={point.name}
                  maxLength={160}
                  onChange={(e) => change({ name: e.target.value })}
                />
              </RaceField>
              <RaceField label={`Checkpoint ${i + 1} distance (km)`}>
                <input
                  required
                  type="number"
                  step="any"
                  min="0.000001"
                  value={point.distanceM / 1000 || ""}
                  onChange={(e) =>
                    change({ distanceM: Number(e.target.value) * 1000 })
                  }
                />
              </RaceField>
              <RaceTimeInput
                label={`Checkpoint ${i + 1} cumulative time`}
                value={point.timeMs || undefined}
                onChange={(timeMs) => change({ timeMs: timeMs ?? 0 })}
              />
              <button
                type="button"
                className="secondary-button small-button"
                onClick={() =>
                  result({
                    checkpoints: draft.result.checkpoints.filter(
                      (_, j) => i !== j,
                    ),
                  })
                }
              >
                Remove checkpoint
              </button>
            </div>
          );
        })}
        <div className="race-actions">
          <button
            type="button"
            className="secondary-button small-button"
            onClick={() =>
              result({
                checkpoints: [
                  ...draft.result.checkpoints,
                  { name: "", distanceM: 0, timeMs: 0 },
                ],
              })
            }
          >
            Add checkpoint
          </button>
          <button
            type="button"
            className="secondary-button small-button"
            disabled={
              !draft.distanceM ||
              draft.result.checkpoints.some(
                (p) => p.distanceM === draft.distanceM! / 2,
              )
            }
            onClick={() =>
              result({
                checkpoints: [
                  ...draft.result.checkpoints,
                  {
                    name: "Halfway",
                    distanceM: draft.distanceM! / 2,
                    timeMs: 0,
                  },
                ].sort((a, b) => a.distanceM - b.distanceM),
              })
            }
          >
            Add halfway checkpoint
          </button>
        </div>
        <p className="muted">
          Enter cumulative times in distance order. Watch laps stay separate.
          GPS halfway estimates need a recording containing only the race.
        </p>
        {draft.activityId && (
          <label className="race-check">
            <input
              type="checkbox"
              checked={draft.raceOnly}
              onChange={(e) => set("raceOnly", e.target.checked)}
            />
            The complete linked recording contains only this race
          </label>
        )}
      </fieldset>
      <fieldset className="panel race-form-section" disabled={save.isPending}>
        <legend>Goals</legend>
        {draft.goals.map((goal, i) => (
          <div className="race-goal-edit-row" key={i}>
            <RaceField label={`Goal ${i + 1}`}>
              <input
                required
                maxLength={500}
                value={goal.name}
                onChange={(e) =>
                  set(
                    "goals",
                    draft.goals.map((g, j) =>
                      j === i ? { ...g, name: e.target.value } : g,
                    ),
                  )
                }
              />
            </RaceField>
            <RaceTimeInput
              label={`Goal ${i + 1} time (optional)`}
              value={goal.timeMs}
              onChange={(timeMs) =>
                set(
                  "goals",
                  draft.goals.map((g, j) => (j === i ? { ...g, timeMs } : g)),
                )
              }
            />
            {goal.timeMs === undefined && (
              <RaceField label="Goal outcome">
                <select
                  value={
                    goal.achieved === undefined ? "" : String(goal.achieved)
                  }
                  onChange={(e) =>
                    set(
                      "goals",
                      draft.goals.map((g, j) =>
                        j === i
                          ? {
                              ...g,
                              achieved:
                                e.target.value === ""
                                  ? undefined
                                  : e.target.value === "true",
                            }
                          : g,
                      ),
                    )
                  }
                >
                  <option value="">Unrecorded</option>
                  <option value="true">Achieved</option>
                  <option value="false">Not achieved</option>
                </select>
              </RaceField>
            )}
            <button
              type="button"
              className="secondary-button small-button"
              onClick={() =>
                set(
                  "goals",
                  draft.goals.filter((_, j) => j !== i),
                )
              }
            >
              Remove goal
            </button>
          </div>
        ))}
        <button
          type="button"
          className="secondary-button small-button"
          disabled={draft.goals.length >= 30}
          onClick={() =>
            set("goals", [
              ...draft.goals,
              {
                name:
                  ["Stretch", "Target", "Fallback"][draft.goals.length] ?? "",
              },
            ])
          }
        >
          Add goal
        </button>
      </fieldset>
      <fieldset className="panel race-form-section" disabled={save.isPending}>
        <legend>Preparation checklist</legend>
        <RaceField label="Copy checklist template">
          <select
            value=""
            onChange={(e) => {
              const template = templates.data?.items.find(
                (t) => t.id === e.target.value,
              );
              if (template)
                set("checklist", [
                  ...draft.checklist,
                  ...(template.items ?? []).map((item) => ({
                    ...item,
                    done: false,
                  })),
                ]);
            }}
          >
            <option value="">Select a template to append</option>
            {templates.data?.items.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name}
              </option>
            ))}
          </select>
        </RaceField>
        <RaceError error={templates.error} />
        <RaceChecklistEditor
          items={draft.checklist}
          onChange={(items) => set("checklist", items)}
        />
      </fieldset>
      <fieldset className="panel race-form-section" disabled={save.isPending}>
        <legend>Event details and logistics</legend>
        <div className="race-form-grid">
          {optionalText("location", "Location")}
          {optionalText("eventUrl", "Event website", "url")}
          {optionalText("resultsUrl", "Official results website", "url")}
          {optionalText(
            "registrationDeadline",
            "Registration deadline",
            "date",
          )}
          {optionalText("bib", "Bib number")}
          {optionalText("startDetails", "Start / bib collection details")}
        </div>
        <div className="race-form-grid">
          {(
            [
              ["registrationNotes", "Registration notes"],
              ["travelNotes", "Travel notes"],
              ["fuelingNotes", "Fueling notes"],
              ["pacingNotes", "Pacing notes"],
              ["kitNotes", "Kit notes"],
              ["notes", "Race notes"],
            ] as const
          ).map(([key, label]) => (
            <RaceField label={label} key={key}>
              <textarea
                maxLength={5000}
                rows={3}
                value={draft[key] ?? ""}
                onChange={(e) => set(key, e.target.value)}
              />
            </RaceField>
          ))}
        </div>
      </fieldset>
      <fieldset className="panel race-form-section" disabled={save.isPending}>
        <legend>Course, weather location, and training link</legend>
        <div className="race-form-grid">
          <RaceField label="Find a saved course">
            <input
              value={courseSearch}
              onChange={(e) => setCourseSearch(e.target.value)}
            />
          </RaceField>
          <RaceField label="Saved course">
            <select
              value={draft.courseId ?? ""}
              onChange={(e) =>
                setDraft((old) => ({
                  ...old,
                  courseId: e.target.value || undefined,
                  removeCourse: !e.target.value,
                  refreshCourse: true,
                }))
              }
            >
              <option value="">No course</option>
              {draft.courseId &&
                !courses.data?.courses.some((c) => c.id === draft.courseId) && (
                  <option value={draft.courseId}>
                    {current?.courseSnapshot?.name ?? "Selected course"}
                  </option>
                )}
              {courses.data?.courses.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name} · {raceDistance(c.distanceM)}
                </option>
              ))}
            </select>
          </RaceField>
          <RaceField label="Forecast latitude">
            <input
              type="number"
              step="any"
              min={-90}
              max={90}
              value={draft.latitude ?? ""}
              onChange={(e) => set("latitude", numeric(e.target.value))}
            />
          </RaceField>
          <RaceField label="Forecast longitude">
            <input
              type="number"
              step="any"
              min={-180}
              max={180}
              value={draft.longitude ?? ""}
              onChange={(e) => set("longitude", numeric(e.target.value))}
            />
          </RaceField>
          <RaceField label="Linked training-plan row">
            <select
              value={draft.plannedActivityId ?? ""}
              onChange={(e) =>
                set("plannedActivityId", e.target.value || undefined)
              }
            >
              <option value="">No training-plan link</option>
              {draft.plannedActivityId &&
                !plans.data?.planned?.some(
                  (p) => p.id === draft.plannedActivityId,
                ) && (
                  <option value={draft.plannedActivityId}>
                    Existing linked row
                  </option>
                )}
              {plans.data?.planned?.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.plannedDate.slice(0, 10)} · {p.name}
                </option>
              ))}
            </select>
          </RaceField>
        </div>
        <RaceError error={courses.error ?? course.error ?? plans.error} />
        <div className="race-actions">
          {courseStart && (
            <button
              type="button"
              className="secondary-button small-button"
              onClick={() =>
                setDraft((old) => ({
                  ...old,
                  latitude: courseStart.latitude,
                  longitude: courseStart.longitude,
                }))
              }
            >
              Use course start for weather
            </button>
          )}
          {current?.courseSnapshot && draft.courseId && (
            <button
              type="button"
              className="secondary-button small-button"
              onClick={() => set("refreshCourse", true)}
            >
              {draft.refreshCourse
                ? "Latest course will replace snapshot on save"
                : "Replace snapshot with latest course"}
            </button>
          )}
          {current?.courseSnapshot && !draft.removeCourse && (
            <button
              type="button"
              className="secondary-button small-button"
              onClick={() =>
                setDraft((old) => ({
                  ...old,
                  courseId: undefined,
                  removeCourse: true,
                  refreshCourse: false,
                }))
              }
            >
              Remove saved course snapshot
            </button>
          )}
        </div>
        <p className="muted">
          Course geometry is saved with the race. Forecast requests send rounded
          coordinates only when race weather is enabled. A training link does
          not match activities or write to the sheet.
        </p>
        {draft.activityId && (
          <p>
            Linked activity:{" "}
            <Link to={`/activities/${draft.activityId}`}>
              {activity?.name ?? "Open activity"}
            </Link>{" "}
            <button
              className="secondary-button small-button"
              type="button"
              onClick={() =>
                setDraft((old) => ({
                  ...old,
                  activityId: undefined,
                  raceOnly: false,
                }))
              }
            >
              Unlink activity
            </button>
          </p>
        )}
      </fieldset>
      <details className="panel race-form-section">
        <summary>Age-grading overrides</summary>
        <div className="race-form-grid">
          <RaceField label="Age on race day override">
            <input
              type="number"
              min={0}
              max={130}
              value={draft.ageOverride ?? ""}
              onChange={(e) => set("ageOverride", numeric(e.target.value))}
            />
          </RaceField>
          <RaceField label="Reference table override">
            <select
              value={draft.tableOverride ?? ""}
              onChange={(e) =>
                set(
                  "tableOverride",
                  e.target.value as RaceInput["tableOverride"],
                )
              }
            >
              <option value="">Use profile preference</option>
              <option value="M">Men’s road table</option>
              <option value="F">Women’s road table</option>
            </select>
          </RaceField>
        </div>
      </details>
      <RaceError error={save.error} />
      <div className="race-save-bar">
        <button
          className="primary-button"
          disabled={save.isPending}
          type="submit"
        >
          {save.isPending ? "Saving…" : "Save race"}
        </button>
        <Link
          className="secondary-button"
          to={current ? `/races/${current.id}` : "/races"}
        >
          Cancel
        </Link>
      </div>
    </form>
  );
}
