import { describe, expect, it } from "vitest";
import { applyThemePreference, parseThemePreference } from "./theme";

describe("theme preferences", () => {
  it.each([
    ["system", "system"],
    ["light", "light"],
    ["dark", "dark"]
  ])("accepts %s", (value, expected) => {
    expect(parseThemePreference(value)).toBe(expected);
  });

  it.each([undefined, null, "", "solarized", 42])("falls back to system for %s", (value) => {
    expect(parseThemePreference(value)).toBe("system");
  });

  it("applies an explicit theme and clears it for system preference", () => {
    const root = { dataset: {} } as Pick<HTMLElement, "dataset">;

    applyThemePreference("dark", root);
    expect(root.dataset.theme).toBe("dark");

    applyThemePreference("light", root);
    expect(root.dataset.theme).toBe("light");

    applyThemePreference("system", root);
    expect(root.dataset.theme).toBeUndefined();
  });
});
