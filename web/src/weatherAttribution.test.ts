import { describe, expect, it } from "vitest";
import { openMeteoAISource, openMeteoUIAttribution } from "./weatherAttribution";

describe("Open-Meteo attribution", () => {
  it("explains recent multi-model midpoint selection and the chosen model", () => {
    const weather = {
      provider: "open-meteo",
      selectionMethod: "midpoint-15-minute-multi-model",
      model: "ECMWF IFS"
    };

    expect(openMeteoUIAttribution(weather)).toContain("median-temperature model among UKMO, ICON, and ECMWF");
    expect(openMeteoUIAttribution(weather)).toContain("ECMWF IFS");
    expect(openMeteoAISource(weather)).toContain("activity midpoint from 15-minute values");
  });

  it("identifies the lower-resolution archive path", () => {
    const weather = { provider: "open-meteo", selectionMethod: "midpoint-nearest-hour-archive" };

    expect(openMeteoUIAttribution(weather)).toContain("nearest-hour archive value");
    expect(openMeteoAISource(weather)).toContain("nearest-hour archive value");
  });

  it("does not mislabel legacy rows", () => {
    const weather = { provider: "open-meteo" };

    expect(openMeteoUIAttribution(weather)).toBe("Model-derived conditions. WMO code rendered as text.");
    expect(openMeteoAISource(weather)).not.toMatch(/midpoint|median|nearest-hour/);
  });
});
