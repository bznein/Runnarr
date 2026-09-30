import { describe, expect, it } from "vitest";
import { heatmapAppearance, heatmapDatePreset, heatmapExportSize, heatmapQuery, heatmapViewport } from "./heatmap";

describe("heatmap selection", () => {
  it("keeps account requests independent of viewport and appearance", () => {
    const params = new URLSearchParams("lat=53&lng=-6&zoom=12&appearance=dark&sport=Run&sport=Cycling&sport=Run&from=2026-03-29");
    expect(heatmapQuery(params, "Europe/Dublin")).toBe("timezone=Europe%2FDublin&from=2026-03-29&sport=Cycling&sport=Run");
    expect(heatmapQuery(new URLSearchParams(), "UTC")).toBe("timezone=UTC&sport=running");
  });
  it("follows the effective app theme unless explicitly overridden", () => {
    expect(heatmapAppearance("app", "midnight", false)).toBe("dark");
    expect(heatmapAppearance("app", "system", true)).toBe("dark");
    expect(heatmapAppearance("app", "sunset", true)).toBe("light");
    expect(heatmapAppearance("light", "midnight", true)).toBe("light");
  });
  it("rejects incomplete and out-of-world bookmarked viewports", () => {
    expect(heatmapViewport(new URLSearchParams())).toBeUndefined();
    expect(heatmapViewport(new URLSearchParams("lat=91&lng=0&zoom=10"))).toBeUndefined();
    expect(heatmapViewport(new URLSearchParams("lat=53&lng=-6&zoom=12"))).toEqual({ center: [53, -6], zoom: 12 });
  });
  it("uses calendar dates for preset boundaries", () => {
    expect(heatmapDatePreset("year", new Date(2026, 8, 30))).toEqual({ from: "2026-01-01", to: "2026-09-30" });
    expect(heatmapDatePreset("last12", new Date(2026, 8, 30))).toEqual({ from: "2025-09-30", to: "2026-09-30" });
  });
  it("caps export dimensions while preserving the crop", () => {
    expect(heatmapExportSize(1200, 600)).toEqual({ width: 2400, height: 1200, scale: 2 });
    expect(heatmapExportSize(3000, 1500)).toEqual({ width: 4096, height: 2048, scale: 4096 / 3000 });
  });
});
