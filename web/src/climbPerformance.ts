import type { ActivityClimb, ActivityLap, ActivitySample } from "./types";

export type ClimbPerformance = {
  paceSPKM?: number;
  gapSPKM?: number;
};

export function samplesForClimbPerformance(fullSamples?: ActivitySample[], seriesSamples?: ActivitySample[]): ActivitySample[] {
  if (fullSamples && fullSamples.length > 0) {
    return fullSamples;
  }
  return seriesSamples ?? [];
}

export function climbPerformanceFor(samples: ActivitySample[], laps: ActivityLap[], climb: ActivityClimb): ClimbPerformance {
  return {
    paceSPKM: paceForClimbSamples(samples, climb),
    gapSPKM: gapForClimbLaps(laps, climb)
  };
}

export function paceForClimbSamples(samples: ActivitySample[], climb: ActivityClimb): number | undefined {
  const climbSamples = samplesForClimb(samples, climb);
  if (climbSamples.length < 2) {
    return undefined;
  }

  let distanceM = 0;
  let movingTimeS = 0;
  for (let index = 1; index < climbSamples.length; index += 1) {
    const previous = climbSamples[index - 1];
    const current = climbSamples[index];
    const previousDistance = finiteDistance(previous.distanceM);
    const currentDistance = finiteDistance(current.distanceM);
    if (previousDistance === undefined || currentDistance === undefined) {
      continue;
    }
    const distanceDeltaM = currentDistance - previousDistance;
    if (distanceDeltaM <= 0) {
      continue;
    }

    const durationS = sampleDurationSeconds(previous, current, distanceDeltaM);
    if (durationS === undefined || durationS <= 0) {
      continue;
    }
    distanceM += distanceDeltaM;
    movingTimeS += durationS;
  }

  if (distanceM <= 0 || movingTimeS <= 0) {
    return undefined;
  }
  return movingTimeS / distanceM * 1000;
}

export function gapForClimbLaps(laps: ActivityLap[], climb: ActivityClimb): number | undefined {
  const startDistanceM = Math.min(climb.startDistanceM, climb.endDistanceM);
  const endDistanceM = Math.max(climb.startDistanceM, climb.endDistanceM);
  let lapStartDistanceM = 0;
  let weightedGap = 0;
  let weightedDistance = 0;

  for (const lap of laps.slice().sort((left, right) => left.index - right.index)) {
    const lapDistanceM = finitePositiveOrZero(lap.distanceM);
    const lapEndDistanceM = lapStartDistanceM + lapDistanceM;
    const overlapDistanceM = Math.max(0, Math.min(endDistanceM, lapEndDistanceM) - Math.max(startDistanceM, lapStartDistanceM));
    if (overlapDistanceM > 0 && validPace(lap.avgGradeAdjustedPaceSPKM)) {
      weightedGap += lap.avgGradeAdjustedPaceSPKM! * overlapDistanceM;
      weightedDistance += overlapDistanceM;
    }
    lapStartDistanceM = lapEndDistanceM;
  }

  return weightedDistance > 0 ? weightedGap / weightedDistance : undefined;
}

export function gapPaceForSample(laps: ActivityLap[], sample: ActivitySample): number | undefined {
  if (typeof sample.distanceM !== "number" || !Number.isFinite(sample.distanceM)) {
    return undefined;
  }
  let lapStartDistanceM = 0;
  for (const lap of laps.slice().sort((left, right) => left.index - right.index)) {
    const lapDistanceM = finitePositiveOrZero(lap.distanceM);
    const lapEndDistanceM = lapStartDistanceM + lapDistanceM;
    if (sample.distanceM >= lapStartDistanceM && sample.distanceM <= lapEndDistanceM && validPace(lap.avgGradeAdjustedPaceSPKM)) {
      return lap.avgGradeAdjustedPaceSPKM;
    }
    lapStartDistanceM = lapEndDistanceM;
  }
  return undefined;
}

function samplesForClimb(samples: ActivitySample[], climb: ActivityClimb) {
  return samples
    .filter((sample) => sample.index >= climb.startSampleIndex && sample.index <= climb.endSampleIndex)
    .slice()
    .sort((left, right) => left.index - right.index);
}

function sampleDurationSeconds(previous: ActivitySample, current: ActivitySample, distanceDeltaM: number) {
  const elapsedDeltaS = difference(previous.elapsedS, current.elapsedS);
  if (elapsedDeltaS !== undefined) {
    return elapsedDeltaS;
  }

  if (previous.timestamp && current.timestamp) {
    const timestampDeltaS = (Date.parse(current.timestamp) - Date.parse(previous.timestamp)) / 1000;
    if (Number.isFinite(timestampDeltaS) && timestampDeltaS > 0) {
      return timestampDeltaS;
    }
  }

  const speedMPS = [current.speedMPS, previous.speedMPS].find((value) => validSpeed(value));
  return speedMPS === undefined ? undefined : distanceDeltaM / speedMPS;
}

function difference(previous?: number, current?: number) {
  if (typeof previous !== "number" || !Number.isFinite(previous) || typeof current !== "number" || !Number.isFinite(current)) {
    return undefined;
  }
  const result = current - previous;
  return result > 0 ? result : undefined;
}

function finitePositiveOrZero(value?: number) {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : 0;
}

function finiteDistance(value?: number) {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : undefined;
}

function validSpeed(value?: number): value is number {
  return typeof value === "number" && Number.isFinite(value) && value > 0;
}

function validPace(value?: number): value is number {
  return typeof value === "number" && Number.isFinite(value) && value > 0;
}
