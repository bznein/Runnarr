try {
  const preference = window.localStorage.getItem("runnarr-theme-preference");
  if (preference === "light" || preference === "dark") {
    document.documentElement.dataset.theme = preference;
  }
} catch {
  // Ignore storage errors and let CSS follow the system preference.
}
