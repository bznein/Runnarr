import { describe, expect, it } from "vitest";
import { applyThemePreference, parseThemePreference, themeOptions, themePreferenceForAccount } from "./theme";

describe("theme preferences", () => {
  it("offers multiple named palettes alongside system behavior", () => {
    expect(themeOptions.map((option) => option.value)).toEqual([
      "system",
      "runnarr",
      "ocean",
      "sunset",
      "midnight"
    ]);
  });

  it.each([
    ["system", "system"],
    ["runnarr", "runnarr"],
    ["ocean", "ocean"],
    ["sunset", "sunset"],
    ["midnight", "midnight"],
    ["light", "runnarr"],
    ["dark", "midnight"]
  ])("normalizes %s to %s", (value, expected) => {
    expect(parseThemePreference(value)).toBe(expected);
  });

  it.each([undefined, null, "", "solarized", 42])("falls back to system for %s", (value) => {
    expect(parseThemePreference(value)).toBe("system");
  });

  it("does not apply a previous account's palette while another account loads", () => {
    expect(themePreferenceForAccount("ocean", "user-1", "user-1")).toBe("ocean");
    expect(themePreferenceForAccount("ocean", "user-1", "user-2")).toBe("system");
    expect(themePreferenceForAccount("ocean", "user-1", undefined)).toBe("system");
  });

  it("applies named palettes without using browser-local storage", () => {
    const root = { dataset: {} } as Pick<HTMLElement, "dataset">;

    applyThemePreference("ocean", root);
    expect(root.dataset.theme).toBe("ocean");

    applyThemePreference("sunset", root);
    expect(root.dataset.theme).toBe("sunset");

    applyThemePreference("system", root);
    expect(root.dataset.theme).toBeUndefined();
  });
});
