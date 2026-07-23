export type ThemePreference = "system" | "light" | "dark";

export function parseThemePreference(value: unknown): ThemePreference {
  return value === "light" || value === "dark" || value === "system" ? value : "system";
}

export function applyThemePreference(
  preference: ThemePreference,
  root: Pick<HTMLElement, "dataset"> = document.documentElement
) {
  if (preference === "system") {
    delete root.dataset.theme;
    return;
  }
  root.dataset.theme = preference;
}
