import { defineConfig, devices, type Project } from "@playwright/test";
import path from "node:path";

type VisualProfile = {
  label: string;
  id: string;
  tag: string;
  viewport: "desktop" | "mobile";
  project: "chromium" | "mobile-chromium";
};

function escapeRegex(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function selectedProfiles(): VisualProfile[] {
  const raw = process.env.RUNNARR_VISUAL_PROFILES_JSON;
  if (!raw) throw new Error("RUNNARR_VISUAL_PROFILES_JSON must contain one or two resolved profiles.");
  const profiles = JSON.parse(raw) as VisualProfile[];
  if (!Array.isArray(profiles) || profiles.length < 1 || profiles.length > 2) {
    throw new Error("Visual review requires one or two resolved profiles.");
  }
  return profiles;
}

function projectsFor(profiles: VisualProfile[]): Project[] {
  return (["desktop", "mobile"] as const).flatMap((viewport) => {
    const tags = profiles.filter((profile) => profile.viewport === viewport).map((profile) => profile.tag);
    if (tags.length === 0) return [];
    return [{
      name: viewport === "mobile" ? "mobile-chromium" : "chromium",
      grep: new RegExp(tags.map(escapeRegex).join("|")),
      use: viewport === "mobile" ? { ...devices["Pixel 8 Pro"] } : { ...devices["Desktop Chrome"] }
    }];
  });
}

const profiles = selectedProfiles();
const artifactRoot = path.resolve(process.env.RUNNARR_E2E_ARTIFACT_DIR ?? "test-results/visual-review");

export default defineConfig({
  testDir: "./e2e",
  outputDir: path.join(artifactRoot, "test-results"),
  timeout: 45_000,
  expect: {
    timeout: 8_000
  },
  fullyParallel: false,
  forbidOnly: true,
  retries: 0,
  workers: 1,
  reporter: [
    ["list"],
    ["html", { outputFolder: path.join(artifactRoot, "playwright-report"), open: "never" }]
  ],
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? "http://127.0.0.1:37617",
    locale: "en-IE",
    timezoneId: "Europe/Dublin",
    reducedMotion: "reduce",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: {
      mode: "on",
      show: {
        actions: { duration: 1_000, position: "bottom", fontSize: 16 },
        test: { level: "title", position: "top", fontSize: 16 }
      }
    },
    launchOptions: {
      slowMo: Number(process.env.PLAYWRIGHT_SLOW_MO ?? "200")
    }
  },
  projects: projectsFor(profiles)
});
