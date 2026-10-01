import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { raceApi } from "./api";
import {
  RaceChecklistEditor,
  RaceError,
  RaceField,
  RaceLoading,
  useInvalidateRaces,
} from "./components";
import type { RaceResource, RaceSettings } from "./types";
import { RaceJobs } from "./RacesPage";

export function RaceSettingsPage({ canWrite }: { canWrite: boolean }) {
  const query = useQuery({
    queryKey: ["races", "settings"],
    queryFn: raceApi.settings,
  });
  return (
    <>
      <RaceError error={query.error} />
      {query.isLoading && <RaceLoading />}
      {query.data && (
        <SettingsForm
          key={query.data.revision}
          initial={query.data}
          canWrite={canWrite}
        />
      )}
      <ResourceManager kind="groups" canWrite={canWrite} />
      <ResourceManager kind="checklists" canWrite={canWrite} />
    </>
  );
}
function SettingsForm({
  initial,
  canWrite,
}: {
  initial: RaceSettings;
  canWrite: boolean;
}) {
  const [draft, setDraft] = useState(initial);
  const invalidate = useInvalidateRaces();
  const save = useMutation({
    mutationFn: () => raceApi.saveSettings(draft),
    onSuccess: invalidate,
  });
  const sync = useMutation({
    mutationFn: raceApi.syncPredictions,
    onSuccess: invalidate,
  });
  return (
    <section className="panel race-section">
      <h2>Race preferences</h2>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <fieldset disabled={!canWrite || save.isPending}>
          <div className="race-form-grid">
            <RaceField label="Birth date (optional)">
              <input
                type="date"
                max={new Date().toISOString().slice(0, 10)}
                value={draft.birthDate ?? ""}
                onChange={(e) =>
                  setDraft({ ...draft, birthDate: e.target.value || undefined })
                }
              />
            </RaceField>
            <RaceField label="Age-grade reference table">
              <select
                value={draft.gradingTable ?? ""}
                onChange={(e) =>
                  setDraft({
                    ...draft,
                    gradingTable: e.target
                      .value as RaceSettings["gradingTable"],
                  })
                }
              >
                <option value="">No preference</option>
                <option value="M">Men’s road table</option>
                <option value="F">Women’s road table</option>
              </select>
            </RaceField>
          </div>
          <p className="muted">
            USATF MLDR road 2025 tables. Only explicitly listed ages and
            distances are graded. Individual races can override these
            preferences. Birth date is excluded from report exports.
          </p>
          <label className="race-check">
            <input
              type="checkbox"
              checked={draft.predictionsEnabled}
              onChange={(e) =>
                setDraft({ ...draft, predictionsEnabled: e.target.checked })
              }
            />
            Read Garmin race predictions
          </label>
          <p className="muted">
            Requires a connected Garmin account. Imports the last 365 days
            initially, then refreshes the last 14 days daily. Reads 5K, 10K,
            half-marathon and marathon predictions.
          </p>
          <label className="race-check">
            <input
              type="checkbox"
              checked={draft.weatherEnabled}
              onChange={(e) =>
                setDraft({ ...draft, weatherEnabled: e.target.checked })
              }
            />
            Fetch race forecasts from Open-Meteo
          </label>
          <p className="muted">
            Sends rounded race coordinates for races within 16 days. Refreshes
            at most every six hours and retains the last pre-start forecast.
          </p>
          <button className="primary-button" disabled={save.isPending}>
            Save preferences
          </button>
        </fieldset>
      </form>
      <RaceError error={save.error ?? sync.error} />
      {initial.predictionError && (
        <p className="error">Prediction sync: {initial.predictionError}</p>
      )}
      {initial.predictionSyncedAt && (
        <p className="muted">
          Predictions last synced:{" "}
          {new Date(initial.predictionSyncedAt).toLocaleString()}
        </p>
      )}
      {canWrite && initial.predictionsEnabled && (
        <button
          className="secondary-button"
          disabled={sync.isPending}
          onClick={() => sync.mutate()}
        >
          Sync predictions now
        </button>
      )}
      <RaceJobs canWrite={canWrite} />
    </section>
  );
}
function ResourceManager({
  kind,
  canWrite,
}: {
  kind: "groups" | "checklists";
  canWrite: boolean;
}) {
  const invalidate = useInvalidateRaces();
  const [editing, setEditing] = useState<Partial<RaceResource> | null>(null);
  const resources = useQuery({
    queryKey: ["races", kind === "groups" ? "groups" : "templates"],
    queryFn: () => raceApi.resources(kind),
  });
  const save = useMutation({
    mutationFn: () => raceApi.saveResource(kind, editing!),
    onSuccess: async () => {
      setEditing(null);
      await invalidate();
    },
  });
  const remove = useMutation({
    mutationFn: (r: RaceResource) => raceApi.deleteResource(kind, r),
    onSuccess: invalidate,
  });
  return (
    <section className="panel race-section">
      <div className="race-heading">
        <h2>
          {kind === "groups" ? "Recurring events" : "Checklist templates"}
        </h2>
        {canWrite && (
          <button
            className="secondary-button"
            onClick={() => setEditing({ name: "", revision: 0, items: [] })}
          >
            New {kind === "groups" ? "event group" : "template"}
          </button>
        )}
      </div>
      <p className="muted">
        {kind === "groups"
          ? "Group editions of the same event. Each race keeps its own distance, course snapshot and result."
          : "Copy a template into a race, then adjust its items and due dates independently."}
      </p>
      <RaceError error={resources.error ?? save.error ?? remove.error} />
      {resources.isLoading && <RaceLoading />}
      {resources.data?.items.map((r) => (
        <div className="race-resource-row" key={r.id}>
          <strong>{r.name}</strong>
          {canWrite && (
            <div className="race-actions">
              <button
                className="secondary-button small-button"
                onClick={() => setEditing(r)}
              >
                Edit {r.name}
              </button>
              <button
                className="secondary-button small-button"
                disabled={remove.isPending}
                onClick={() => {
                  if (
                    window.confirm(
                      `Delete ${r.name}? Existing races and copied checklist items will remain.`,
                    )
                  )
                    remove.mutate(r);
                }}
              >
                Delete {r.name}
              </button>
            </div>
          )}
        </div>
      ))}
      {editing && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate();
          }}
        >
          <fieldset disabled={save.isPending}>
            <RaceField
              label={kind === "groups" ? "Event group name" : "Template name"}
            >
              <input
                required
                maxLength={160}
                value={editing.name ?? ""}
                onChange={(e) =>
                  setEditing({ ...editing, name: e.target.value })
                }
              />
            </RaceField>
            {kind === "checklists" && (
              <RaceChecklistEditor
                items={editing.items ?? []}
                onChange={(items) => setEditing({ ...editing, items })}
              />
            )}
            <div className="race-actions">
              <button className="primary-button">
                Save {kind === "groups" ? "event group" : "template"}
              </button>
              <button
                type="button"
                className="secondary-button"
                onClick={() => setEditing(null)}
              >
                Cancel
              </button>
            </div>
          </fieldset>
        </form>
      )}
    </section>
  );
}
