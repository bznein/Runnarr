import {
  cloneElement,
  isValidElement,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import type { RaceChecklistItem, RaceSummary } from "./types";
import {
  checklistDueDate,
  eventToday,
  formatRaceTime,
  parseRaceTime,
  raceCountdown,
  raceDistance,
  raceLabel,
} from "./utils";

export function RaceField({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  const id = useId();
  return (
    <div className="race-field">
      <label htmlFor={id}>{label}</label>
      {isValidElement<{ id?: string }>(children)
        ? cloneElement(children, { id })
        : children}
    </div>
  );
}
export function RaceError({ error }: { error: unknown }) {
  return error ? (
    <p role="alert" className="error">
      {error instanceof Error ? error.message : String(error)}
    </p>
  ) : null;
}
export function RaceLoading() {
  return (
    <p role="status" className="muted">
      Loading races…
    </p>
  );
}
export function RaceTimeInput({
  label,
  value,
  onChange,
}: {
  label: string;
  value?: number;
  onChange: (ms?: number) => void;
}) {
  const [text, setText] = useState(formatRaceTime(value));
  const focused = useRef(false);
  useEffect(() => {
    if (!focused.current) setText(formatRaceTime(value));
  }, [value]);
  return (
    <RaceField label={label}>
      <input
        value={text}
        placeholder="h:mm:ss"
        inputMode="decimal"
        onFocus={() => {
          focused.current = true;
        }}
        onChange={(e) => {
          setText(e.target.value);
          try {
            const ms = parseRaceTime(e.target.value);
            e.target.setCustomValidity("");
            onChange(ms);
          } catch (error) {
            e.target.setCustomValidity((error as Error).message);
          }
        }}
        onBlur={(e) => {
          focused.current = false;
          try {
            const ms = parseRaceTime(text);
            e.target.setCustomValidity("");
            setText(formatRaceTime(ms));
            onChange(ms);
          } catch (error) {
            e.target.setCustomValidity((error as Error).message);
            e.target.reportValidity();
          }
        }}
      />
    </RaceField>
  );
}
export function useInvalidateRaces() {
  const client = useQueryClient();
  return async () => {
    await Promise.all(
      [
        "races",
        "activities",
        "activity",
        "activity-calendar",
        "calendar-day",
        "sync-jobs",
      ].map((key) => client.invalidateQueries({ queryKey: [key] })),
    );
  };
}
export function RaceCalendarEntries({ races = [] }: { races?: RaceSummary[] }) {
  if (!races.length) return null;
  return (
    <ul className="race-calendar-entries">
      {races.map((r) => (
        <li key={r.id}>
          <Link to={`/races/${r.id}`}>
            <span className="race-badge">
              Race{r.priority ? ` ${r.priority}` : ""}
            </span>{" "}
            {r.name}
          </Link>
          <span className="muted">
            {[
              r.startTime,
              r.startTime ? r.timezone : "",
              raceDistance(r.distanceM),
              raceLabel(r.status),
              raceCountdown(r),
            ]
              .filter(Boolean)
              .join(" · ")}
          </span>
        </li>
      ))}
    </ul>
  );
}
export function RaceChecklistEditor({
  items,
  onChange,
}: {
  items: RaceChecklistItem[];
  onChange: (items: RaceChecklistItem[]) => void;
}) {
  function change(index: number, patch: Partial<RaceChecklistItem>) {
    onChange(
      items.map((item, i) => (i === index ? { ...item, ...patch } : item)),
    );
  }
  return (
    <div className="race-checklist-editor">
      {items.map((item, i) => (
        <div className="race-checklist-edit-row" key={i}>
          <RaceField label={`Checklist item ${i + 1}`}>
            <input
              required
              maxLength={500}
              value={item.label}
              onChange={(e) => change(i, { label: e.target.value })}
            />
          </RaceField>
          <RaceField label="Due date">
            <input
              type="date"
              value={item.dueDate ?? ""}
              onChange={(e) =>
                change(i, {
                  dueDate: e.target.value || undefined,
                  daysBefore: undefined,
                })
              }
            />
          </RaceField>
          <RaceField label="Days before race">
            <input
              type="number"
              min={-365}
              max={365}
              value={item.daysBefore ?? ""}
              onChange={(e) =>
                change(i, {
                  daysBefore:
                    e.target.value === "" ? undefined : Number(e.target.value),
                  dueDate: undefined,
                })
              }
            />
          </RaceField>
          <button
            type="button"
            className="secondary-button small-button"
            aria-label={`Remove checklist item ${i + 1}`}
            onClick={() => onChange(items.filter((_, index) => index !== i))}
          >
            Remove
          </button>
        </div>
      ))}
      <button
        type="button"
        className="secondary-button small-button"
        onClick={() => onChange([...items, { label: "", done: false }])}
        disabled={items.length >= 200}
      >
        Add checklist item
      </button>
    </div>
  );
}
export function RaceChecklist({
  items,
  date,
  timezone,
  disabled,
  onChange,
}: {
  items: RaceChecklistItem[];
  date?: string;
  timezone: string;
  disabled: boolean;
  onChange: (items: RaceChecklistItem[]) => Promise<void>;
}) {
  const [displayItems, setDisplayItems] = useState(items);
  useEffect(() => setDisplayItems(items), [items]);
  async function toggle(index: number, done: boolean) {
    const next = items.map((item, i) =>
      i === index ? { ...item, done } : item,
    );
    setDisplayItems(next);
    try {
      await onChange(next);
    } catch {
      setDisplayItems(items);
    }
  }
  const today = eventToday(timezone);
  return (
    <ul className="race-checklist">
      {displayItems.map((item, i) => {
        const due = checklistDueDate(item, date);
        const overdue = !item.done && due && due < today;
        return (
          <li key={i}>
            <label>
              <input
                type="checkbox"
                checked={item.done}
                disabled={disabled}
                onChange={(e) => void toggle(i, e.target.checked)}
              />
              <span>{item.label}</span>
            </label>
            {due && (
              <span className={overdue ? "race-overdue" : "muted"}>
                {overdue ? "Overdue · " : "Due "}
                {due}
              </span>
            )}
          </li>
        );
      })}
    </ul>
  );
}
