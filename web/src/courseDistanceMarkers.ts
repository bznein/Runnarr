export type CourseRoutePoint = [number, number];

export type CourseDistanceMarker = {
  kilometre: number;
  point: CourseRoutePoint;
};

const EARTH_RADIUS_M = 6_371_000;
const EARTH_CIRCUMFERENCE_M = 40_075_016.686;
const TILE_SIZE_PX = 256;
const MIN_MARKER_SPACING_PX = 42;

export function routePointDistanceM(start: CourseRoutePoint, end: CourseRoutePoint) {
  const latitude = radians(end[0] - start[0]);
  const longitude = radians(end[1] - start[1]);
  const value = clamp(
    Math.sin(latitude / 2) ** 2
      + Math.cos(radians(start[0])) * Math.cos(radians(end[0])) * Math.sin(longitude / 2) ** 2,
    0,
    1
  );
  return EARTH_RADIUS_M * 2 * Math.atan2(Math.sqrt(value), Math.sqrt(1 - value));
}

export function courseDistanceMarkers(polylines: CourseRoutePoint[][]): CourseDistanceMarker[] {
  const markers: CourseDistanceMarker[] = [];
  let travelledM = 0;
  let nextMarkerM = 1_000;

  for (const points of polylines) {
    for (let index = 1; index < points.length; index++) {
      const start = points[index - 1];
      const end = points[index];
      const segmentM = routePointDistanceM(start, end);
      if (!Number.isFinite(segmentM) || segmentM <= 0) continue;

      const segmentEndM = travelledM + segmentM;
      while (nextMarkerM <= segmentEndM) {
        const fraction = (nextMarkerM - travelledM) / segmentM;
        markers.push({ kilometre: nextMarkerM / 1_000, point: interpolateGreatCircle(start, end, fraction) });
        nextMarkerM += 1_000;
      }
      travelledM = segmentEndM;
    }
  }

  return markers;
}

export function kilometreMarkerStride(zoom: number, latitude: number) {
  const latitudeScale = Math.max(0.01, Math.abs(Math.cos(radians(latitude))));
  const metresPerPixel = EARTH_CIRCUMFERENCE_M * latitudeScale / (TILE_SIZE_PX * 2 ** zoom);
  return niceIntegerCeiling(metresPerPixel * MIN_MARKER_SPACING_PX / 1_000);
}

function interpolateGreatCircle(start: CourseRoutePoint, end: CourseRoutePoint, fraction: number): CourseRoutePoint {
  if (fraction <= 0) return start;
  if (fraction >= 1) return end;

  const startVector = unitVector(start);
  const endVector = unitVector(end);
  const dot = clamp(startVector[0] * endVector[0] + startVector[1] * endVector[1] + startVector[2] * endVector[2], -1, 1);
  const angle = Math.acos(dot);
  const sine = Math.sin(angle);
  if (Math.abs(sine) < 1e-12) {
    return [start[0] + (end[0] - start[0]) * fraction, start[1] + (end[1] - start[1]) * fraction];
  }

  const startWeight = Math.sin((1 - fraction) * angle) / sine;
  const endWeight = Math.sin(fraction * angle) / sine;
  const x = startVector[0] * startWeight + endVector[0] * endWeight;
  const y = startVector[1] * startWeight + endVector[1] * endWeight;
  const z = startVector[2] * startWeight + endVector[2] * endWeight;
  return [degrees(Math.atan2(z, Math.sqrt(x * x + y * y))), degrees(Math.atan2(y, x))];
}

function unitVector(point: CourseRoutePoint): [number, number, number] {
  const latitude = radians(point[0]);
  const longitude = radians(point[1]);
  const latitudeScale = Math.cos(latitude);
  return [latitudeScale * Math.cos(longitude), latitudeScale * Math.sin(longitude), Math.sin(latitude)];
}

function niceIntegerCeiling(value: number) {
  if (!Number.isFinite(value) || value <= 1) return 1;
  const magnitude = 10 ** Math.floor(Math.log10(value));
  const normalized = value / magnitude;
  const nice = normalized <= 1 ? 1 : normalized <= 2 ? 2 : normalized <= 5 ? 5 : 10;
  return Math.max(1, Math.round(nice * magnitude));
}

function radians(value: number) {
  return value * Math.PI / 180;
}

function degrees(value: number) {
  return value * 180 / Math.PI;
}

function clamp(value: number, minimum: number, maximum: number) {
  return Math.min(maximum, Math.max(minimum, value));
}
