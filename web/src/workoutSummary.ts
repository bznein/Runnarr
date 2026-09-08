import { formatPaceMinutesSeconds, speedToPaceSPKM } from "./paceDisplay";
import type { Activity, ActivityInterval, ActivityWorkout, ActivityWorkoutStep, Workout, WorkoutStep } from "./types";

type Metric = "time" | "distance" | "pace" | "heartRate" | "power";
type Goal = { metric: Metric; low: number; high: number };
type Step = {
  key: string;
  category: string;
  label: string;
  description?: string;
  repeatPath: number[];
  occurrence: number;
  order: number;
  goals: Goal[];
  condition: string;
  target: string;
  unsupported: boolean;
};
export type WorkoutSummaryRow = {
  step?: Step;
  actual?: ActivityInterval;
  score?: number;
  status: string;
};
export type WorkoutSummary = {
  source: "matched" | "imported";
  name: string;
  prescription?: string;
  warnings: string[];
  rows: WorkoutSummaryRow[];
  score?: number;
  scoredSteps: number;
  workSteps: number;
  paceTolerance?: number;
};

const maxSteps = 500;
const maxActuals = 1000;
const positive = (value: unknown): value is number => typeof value === "number" && Number.isFinite(value) && value > 0;
const nonnegative = (value: unknown): value is number => typeof value === "number" && Number.isFinite(value) && value >= 0;

export function summaryCategory(value?: string) {
  const key = value?.toLowerCase().replace(/[ ._-]/g, "") ?? "";
  return ({ interval: "active", work: "active", run: "active", rest: "recovery", warmup: "warmup", cooldown: "cooldown" } as Record<string, string>)[key] ?? key;
}

function categoryLabel(category: string) {
  return ({ active: "Work", warmup: "Warm up", recovery: "Recovery", cooldown: "Cool down" } as Record<string, string>)[category] ?? (category || "Step");
}

export function summaryValue(metric: Metric, value?: number): string {
  if (!nonnegative(value)) return "";
  switch (metric) {
    case "time": {
      const seconds = Math.round(value);
      const minutes = Math.floor(seconds / 60);
      return minutes >= 60
        ? `${Math.floor(minutes / 60)}:${String(minutes % 60).padStart(2, "0")}:${String(seconds % 60).padStart(2, "0")}`
        : `${minutes}:${String(seconds % 60).padStart(2, "0")}`;
    }
    case "distance": return value >= 1000 ? `${(value / 1000).toFixed(2)} km` : `${Math.round(value)} m`;
    case "pace": return positive(value) ? `${formatPaceMinutesSeconds(value)} /km` : "";
    case "heartRate": return positive(value) ? `${Math.round(value)} bpm` : "";
    case "power": return `${Math.round(value)} W`;
  }
}

function goalLabel(goal: Goal) {
  return goal.low === goal.high ? summaryValue(goal.metric, goal.low) : `${summaryValue(goal.metric, goal.low)}–${summaryValue(goal.metric, goal.high)}`;
}

function range(metric: Metric, one?: number, two = one): Goal | undefined {
  if (!positive(one) || !positive(two)) return undefined;
  return { metric, low: Math.min(one, two), high: Math.max(one, two) };
}

export function summaryDuration(actual: ActivityInterval) {
  const raw = actual.raw?.duration;
  const duration = typeof raw === "string" && raw.trim() !== "" ? Number(raw) : raw;
  if (nonnegative(duration)) return duration;
  return positive(actual.movingTimeS) ? actual.movingTimeS : actual.elapsedTimeS;
}

export function summaryPace(actual: ActivityInterval) {
  if (positive(actual.avgPaceSPKM)) return actual.avgPaceSPKM;
  const duration = summaryDuration(actual);
  return positive(actual.distanceM) && positive(duration) ? duration * 1000 / actual.distanceM : undefined;
}

function actualValue(actual: ActivityInterval, metric: Metric) {
  switch (metric) {
    case "time": return summaryDuration(actual);
    case "distance": return actual.distanceM;
    case "pace": return summaryPace(actual);
    case "heartRate": return positive(actual.avgHeartRate) ? actual.avgHeartRate : undefined;
    case "power": return actual.avgPower;
  }
}

// The percentage deviation from the nearest boundary is the penalty. Ranges
// are inclusive, and extra speed receives the same penalty as going too slowly.
export function workoutGoalScore(actual: number, low: number, high = low) {
  if (!nonnegative(actual) || !positive(low) || !positive(high) || high < low) return undefined;
  const boundary = Math.max(low, Math.min(high, actual));
  return Math.max(0, 100 * (1 - Math.abs(actual - boundary) / boundary));
}

function scoreStep(step: Step, actual?: ActivityInterval): Pick<WorkoutSummaryRow, "score" | "status"> {
  if (!actual) return { status: "No reliable result" };
  if (step.unsupported) return { status: "Target cannot be scored" };
  if (!step.goals.length) return { status: "No numeric goal" };
  const scores = step.goals.map((goal) => {
    const value = actualValue(actual, goal.metric);
    return value === undefined ? undefined : workoutGoalScore(value, goal.low, goal.high);
  });
  if (scores.some((value) => value === undefined)) return { status: "Missing goal data" };
  return { score: (scores as number[]).reduce((sum, value) => sum + value, 0) / scores.length, status: "Scored" };
}

function executableSteps(steps: WorkoutStep[]): boolean {
  return steps.some((step) => step.kind === "repeat" ? executableSteps(step.children ?? []) : true);
}

export function buildWorkoutSummary(activity: Pick<Activity, "workout" | "intervals" | "laps">, matched?: Workout, defaultTolerance?: number): WorkoutSummary | undefined {
  const useMatched = matched && matched.parseStatus !== "error" && executableSteps(matched.definition.steps);
  if (!useMatched && !activity.workout && !matched) return undefined;
  const source = useMatched || !activity.workout ? "matched" : "imported";
  const warnings: string[] = [];
  if (matched && !useMatched) warnings.push("The matched workout has no usable parsed steps." + (activity.workout ? " Showing the imported workout." : ""));
  if (source === "matched") warnings.push(...(matched?.parseMessages ?? []).map((message) => message.message));
  const tolerance = matched?.paceToleranceSeconds ?? defaultTolerance;
  const { steps, complete } = expandSteps(source === "matched" ? matched : undefined, source === "imported" ? activity.workout : undefined, tolerance);
  if (!complete) warnings.push("The workout definition is incomplete or too large to expand safely. The overall score is unavailable.");
  const actuals = (activity.intervals?.length ? activity.intervals : recordedLaps(activity, steps)).slice(0, maxActuals);
  const tooManyActuals = (activity.intervals?.length || activity.laps?.length || 0) > maxActuals;
  if (tooManyActuals) warnings.push("There are too many recorded intervals to compare safely. The overall score is unavailable.");
  const fromLaps = !activity.intervals?.length;
  const references = source === "imported" || fromLaps ? resolveReferences(steps, actuals) : new Map<number, string>();
  const matches = uniqueOrderedMatches(steps, actuals, references, fromLaps);
  const used = new Set(matches.values());
  const rows: WorkoutSummaryRow[] = steps.map((step, i) => {
    const index = matches.get(i);
    const actual = index === undefined ? undefined : actuals[index];
    return { step, actual, ...scoreStep(step, actual) };
  });
  actuals.forEach((actual, i) => {
    if (!used.has(i)) rows.push({ actual, status: "Additional recorded interval" });
  });
  const work = rows.filter((row) => row.step?.category === "active");
  const scored = work.filter((row) => row.score !== undefined);
  return {
    source,
    name: source === "matched" ? matched?.name ?? "Matched workout" : activity.workout?.name || "Imported workout",
    prescription: source === "matched" ? matched?.sourceText : undefined,
    warnings,
    rows,
    score: complete && !tooManyActuals && scored.length ? scored.reduce((sum, row) => sum + row.score!, 0) / scored.length : undefined,
    scoredSteps: scored.length,
    workSteps: work.length,
    paceTolerance: source === "matched" && steps.some((step) => step.goals.some((goal) => goal.metric === "pace")) ? tolerance : undefined
  };
}

function expandSteps(matched?: Workout, imported?: ActivityWorkout, tolerance?: number) {
  const steps: Step[] = [];
  let complete = true;
  let visited = 0;
  const occurrences = new Map<string, number>();
  const visit = (items: Array<WorkoutStep | ActivityWorkoutStep>, path: string, repeatPath: number[], depth: number, skipRecovery = false) => {
    if (depth > 10) { complete = false; return; }
    for (let i = 0; i < items.length; i++) {
      if (steps.length >= maxSteps || visited++ >= 5000) { complete = false; return; }
      const item = items[i];
      const local = "kind" in item;
      const kind = local ? item.kind : item.type;
      if (skipRecovery && i === items.length - 1 && summaryCategory(kind) === "recovery") continue;
      const key = `${path}.${i}`;
      if (kind === "repeat") {
        if (!Number.isInteger(item.repeatCount) || !positive(item.repeatCount) || !(item.children?.length)) { complete = false; continue; }
        for (let repeat = 1; repeat <= Math.min(item.repeatCount, maxSteps); repeat++) {
          visit(item.children, key, [...repeatPath, repeat], depth + 1, Boolean(item.skipLastRecovery && repeat === item.repeatCount));
          if (steps.length >= maxSteps || visited >= 5000) {
            if (repeat < item.repeatCount) complete = false;
            break;
          }
        }
        if (item.repeatCount > maxSteps) complete = false;
        continue;
      }
      const category = summaryCategory(kind);
      const goals: Goal[] = [];
      let unsupported = false;
      const end = local ? item.endCondition?.type : item.endCondition;
      const value = local ? item.endCondition?.value : item.endConditionValue;
      let condition = "Open duration";
      if (end === "time" || end === "distance") {
        const goal = range(end, value);
        condition = goal ? goalLabel(goal) : (end === "time" ? "Time unavailable" : "Distance unavailable");
        if (goal) goals.push(goal); else unsupported = true;
      } else if (end === "lap_button" || end === "lap.button") {
        condition = "Lap button";
      } else if (end) {
        condition = `${end}${nonnegative(value) ? `: ${value}` : ""}`;
        unsupported = true;
      }
      let intensity: Goal | undefined;
      let target = "";
      if (local) {
        if (item.target.type === "pace") {
          if (positive(item.target.paceFastSecondsPerKM) && positive(item.target.paceSlowSecondsPerKM)) {
            intensity = range("pace", item.target.paceFastSecondsPerKM, item.target.paceSlowSecondsPerKM);
          } else if (positive(item.target.paceSecondsPerKM) && nonnegative(tolerance)) {
            intensity = range("pace", Math.max(1, item.target.paceSecondsPerKM - tolerance), item.target.paceSecondsPerKM + tolerance);
          }
          if (!intensity) { unsupported = true; target = positive(item.target.paceSecondsPerKM) ? `${summaryValue("pace", item.target.paceSecondsPerKM)} (tolerance unavailable)` : "Pace target unavailable"; }
        }
      } else {
        const type = item.targetType?.toLowerCase();
        const unit = item.targetValueUnit?.toLowerCase() ?? "";
        const hasZone = positive(item.zoneNumber);
        if (type === "pace.zone" && !hasZone && ["", "m/s", "mps", "meter_per_second"].includes(unit)) {
          intensity = range("pace", speedToPaceSPKM(item.targetValueOne), speedToPaceSPKM(item.targetValueTwo ?? item.targetValueOne));
        } else if (type === "heart.rate.zone" && !hasZone && ["bpm", "beats_per_minute"].includes(unit)) {
          intensity = range("heartRate", item.targetValueOne, item.targetValueTwo ?? item.targetValueOne);
        } else if (type === "power.zone" && !hasZone && ["watt", "watts", "w"].includes(unit)) {
          intensity = range("power", item.targetValueOne, item.targetValueTwo ?? item.targetValueOne);
        }
        if (type && type !== "no.target" && type !== "none" && !intensity) {
          unsupported = true;
          const values = [item.targetValueOne, item.targetValueTwo].filter(nonnegative).join("–");
          target = `${type.replace(/\./g, " ")}${hasZone ? ` ${item.zoneNumber}` : values ? `: ${values}${unit ? ` ${unit}` : ""}` : ""}`;
        }
      }
      if (intensity) { goals.push(intensity); target = goalLabel(intensity); }
      const occurrence = (occurrences.get(key) ?? 0) + 1;
      occurrences.set(key, occurrence);
      steps.push({ key, category, label: `${repeatPath.length ? `Set ${repeatPath.join(".")} · ` : ""}${categoryLabel(category)}`, description: item.description, repeatPath, occurrence, order: item.order, goals, condition, target, unsupported });
    }
  };
  if (matched?.parseStatus !== "error") visit(matched?.definition.steps ?? imported?.steps ?? [], "step", [], 0);
  return { steps, complete };
}

// Normalized index is synthetic and is never a provider reference. Providers
// may use stepOrder or zero-based order. Use a convention only if all references
// resolves uniquely and agrees with recorded categories. Conflicting valid
// conventions deliberately leave those references unresolved.
function resolveReferences(steps: Step[], actuals: ActivityInterval[]) {
  const schemes = [(step: Step) => step.order, (step: Step) => step.order - 1];
  const withReferences = actuals.filter((actual) => actual.workoutStepIndex !== undefined);
  const valid = schemes.map((scheme) => {
    const resolved = new Map<number, string>();
    for (const actual of withReferences) {
      const candidates = steps.filter((step) => scheme(step) === actual.workoutStepIndex && (!actual.category || summaryCategory(actual.category) === step.category));
      const keys = new Set(candidates.map((step) => step.key));
      if (keys.size !== 1) return undefined;
      resolved.set(actual.workoutStepIndex!, candidates[0].key);
    }
    return resolved;
  }).filter((value): value is Map<number, string> => value !== undefined);
  const result = new Map<number, string>();
  for (const actual of withReferences) {
    const keys = new Set(valid.map((scheme) => scheme.get(actual.workoutStepIndex!)));
    if (keys.size === 1) result.set(actual.workoutStepIndex!, [...keys][0]!);
  }
  return result;
}

function recordedLaps(activity: Pick<Activity, "laps">, steps: Step[]): ActivityInterval[] {
  const laps = (activity.laps ?? []).slice(0, maxActuals).map((lap): ActivityInterval => ({ ...lap, category: summaryCategory(lap.intensityType) }));
  const references = resolveReferences(steps, laps);
  const groups: ActivityInterval[][] = [];
  for (const lap of laps) {
    const key = references.get(lap.workoutStepIndex!);
    const step = steps.find((candidate) => candidate.key === key);
    const previous = groups[groups.length - 1];
    const canGroup = key !== undefined && (lap.workoutRepeatIndex !== undefined || steps.filter((candidate) => candidate.key === key).length === 1);
    if (canGroup && previous && previous[0].workoutStepIndex === lap.workoutStepIndex && previous[0].workoutRepeatIndex === lap.workoutRepeatIndex) previous.push(lap);
    else groups.push([{ ...lap, category: step?.category ?? lap.category }]);
  }
  return groups.map((group) => {
    if (group.length === 1) return group[0];
    const duration = group.reduce((sum, lap) => sum + summaryDuration(lap), 0);
    const weighted = (metric: "avgHeartRate" | "avgPower") => group.every((lap) => nonnegative(lap[metric])) && positive(duration)
      ? group.reduce((sum, lap) => sum + lap[metric]! * summaryDuration(lap), 0) / duration : undefined;
    const paces = group.map(summaryPace);
    return {
      ...group[0],
      elapsedTimeS: group.reduce((sum, lap) => sum + lap.elapsedTimeS, 0),
      movingTimeS: group.reduce((sum, lap) => sum + lap.movingTimeS, 0),
      distanceM: group.reduce((sum, lap) => sum + lap.distanceM, 0),
      avgPaceSPKM: paces.every(positive) && positive(duration) ? duration / group.reduce((sum, lap, i) => sum + summaryDuration(lap) / paces[i]!, 0) : undefined,
      avgHeartRate: weighted("avgHeartRate"),
      avgPower: weighted("avgPower"),
      raw: { duration }
    };
  });
}

// Accept only pairs present in every optimal ordered alignment. A missing rep
// amongst otherwise indistinguishable efforts must not shift later targets.
function uniqueOrderedMatches(steps: Step[], actuals: ActivityInterval[], references: Map<number, string>, requireReferences: boolean) {
  const n = steps.length, m = actuals.length;
  const compatible = (i: number, j: number) => {
    const step = steps[i], actual = actuals[j];
    if (summaryCategory(actual.category) !== step.category) return false;
    const reference = references.get(actual.workoutStepIndex!);
    if (requireReferences && reference === undefined) return false;
    if (reference !== undefined && reference !== step.key) return false;
    if (step.repeatPath.length && positive(actual.workoutRepeatIndex) && actual.workoutRepeatIndex !== step.occurrence) return false;
    return true;
  };
  const prefix = Array.from({ length: n + 1 }, () => new Uint16Array(m + 1));
  const suffix = Array.from({ length: n + 1 }, () => new Uint16Array(m + 1));
  for (let i = 0; i < n; i++) for (let j = 0; j < m; j++) {
    prefix[i + 1][j + 1] = Math.max(prefix[i][j + 1], prefix[i + 1][j], compatible(i, j) ? prefix[i][j] + 1 : 0);
  }
  for (let i = n - 1; i >= 0; i--) for (let j = m - 1; j >= 0; j--) {
    suffix[i][j] = Math.max(suffix[i + 1][j], suffix[i][j + 1], compatible(i, j) ? suffix[i + 1][j + 1] + 1 : 0);
  }
  const optimum = suffix[0][0];
  const matches = new Map<number, number>();
  for (let i = 0; i < n; i++) {
    let canSkip = false;
    const candidates: number[] = [];
    for (let j = 0; j <= m; j++) {
      if (prefix[i][j] + suffix[i + 1][j] === optimum) canSkip = true;
      if (j < m && compatible(i, j) && prefix[i][j] + 1 + suffix[i + 1][j + 1] === optimum) candidates.push(j);
    }
    if (!canSkip && candidates.length === 1) matches.set(i, candidates[0]);
  }
  return matches;
}
