export type ThemePreference = "system" | "runnarr" | "ocean" | "sunset" | "midnight";

export type ThemeOption = {
  value: ThemePreference;
  label: string;
  description: string;
};

export const themeOptions: ThemeOption[] = [
  {
    value: "system",
    label: "System",
    description: "Follow your device's light or dark setting."
  },
  {
    value: "runnarr",
    label: "Runnarr",
    description: "The original calm green palette."
  },
  {
    value: "ocean",
    label: "Ocean",
    description: "Cool blue surfaces with a clear cyan accent."
  },
  {
    value: "sunset",
    label: "Sunset",
    description: "Warm sand surfaces with terracotta accents."
  },
  {
    value: "midnight",
    label: "Midnight",
    description: "A dark palette for low-light use."
  }
];

const themeValues = new Set<ThemePreference>(themeOptions.map((option) => option.value));

export function parseThemePreference(value: unknown): ThemePreference {
  if (themeValues.has(value as ThemePreference)) {
    return value as ThemePreference;
  }
  if (value === "light") {
    return "runnarr";
  }
  if (value === "dark") {
    return "midnight";
  }
  return "system";
}

export function themePreferenceForAccount(
  preference: ThemePreference,
  preferenceUserID: string | undefined,
  effectiveUserID: string | undefined
): ThemePreference {
  return Boolean(effectiveUserID) && preferenceUserID === effectiveUserID ? preference : "system";
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
