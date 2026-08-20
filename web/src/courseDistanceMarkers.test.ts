import { describe, expect, it } from "vitest";
import { courseDistanceMarkers, kilometreMarkerStride, routePointDistanceM } from "./courseDistanceMarkers";

describe("course distance markers", () => {
  it("places whole-kilometre markers along route geometry", () => {
    const markers = courseDistanceMarkers([[[0, 0], [0, 0.03]]]);

    expect(markers.map((marker) => marker.kilometre)).toEqual([1, 2, 3]);
    expect(markers[0].point[0]).toBeCloseTo(0, 8);
    expect(routePointDistanceM([0, 0], markers[0].point)).toBeCloseTo(1_000, 5);
    expect(routePointDistanceM([0, 0], markers[2].point)).toBeCloseTo(3_000, 5);
  });

  it("continues accumulated distance across legs without bridging gaps", () => {
    const markers = courseDistanceMarkers([
      [[0, 0], [0, 0.006]],
      [[1, 1], [1, 1.012]]
    ]);

    expect(markers.map((marker) => marker.kilometre)).toEqual([1, 2]);
    expect(markers[0].point[0]).toBeCloseTo(1, 4);
    expect(markers[1].point[0]).toBeCloseTo(1, 4);
  });

  it("ignores repeated points and handles routes crossing the date line", () => {
    const markers = courseDistanceMarkers([[[0, 179.99], [0, 179.99], [0, -179.99]]]);

    expect(markers).toHaveLength(2);
    expect(Math.abs(markers[0].point[1])).toBeGreaterThan(179.99);
  });

  it("uses clean wider strides as the map zooms out", () => {
    expect(kilometreMarkerStride(13, 53.35)).toBe(1);
    expect(kilometreMarkerStride(11, 53.35)).toBe(2);
    expect(kilometreMarkerStride(10, 53.35)).toBe(5);
    expect(kilometreMarkerStride(9, 53.35)).toBe(10);
  });
});
