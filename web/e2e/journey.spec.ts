import { expect, test, type Page } from "@playwright/test";
import { readFile } from "node:fs/promises";
import path from "node:path";

const username = process.env.RUNNARR_E2E_USERNAME ?? "e2e-admin";
const password = process.env.RUNNARR_E2E_PASSWORD ?? "e2e-password-123";
const gpxPath = path.resolve(process.cwd(), "../examples/morning-run.gpx");

function activityName(projectName: string) {
  return `E2E ${projectName} Morning Run`;
}

function projectSlug(projectName: string) {
  return projectName.replace(/[^a-z0-9]+/gi, "-").toLowerCase();
}

async function login(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Username").fill(username);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Log in" }).click();
  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByRole("heading", { name: "Dashboard" })).toBeVisible();
}

async function ensureActivityImported(page: Page, projectName: string) {
  const name = activityName(projectName);
  await page.goto("/activities");
  await expect(page.getByRole("heading", { name: "Activities" })).toBeVisible();
  await expect(page.locator("table.activity-table, .empty-state").first()).toBeVisible();

  if (await page.getByRole("link", { name, exact: true }).count() === 0) {
    const original = await readFile(gpxPath, "utf8");
    const fixture = original
      .replace("<name>Example Morning Run</name>", `<name>${name}</name>`)
      .replace("</gpx>", `<!-- ${projectSlug(projectName)} -->\n</gpx>`);

    await page.goto("/settings#import");
    await page.locator('input[type="file"][accept=".gpx,.tcx,.fit"]').setInputFiles({
      name: `${projectSlug(projectName)}-morning-run.gpx`,
      mimeType: "application/gpx+xml",
      buffer: Buffer.from(fixture)
    });
    await Promise.all([
      page.waitForResponse((response) => response.url().includes("/api/imports") && response.status() === 201),
      page.getByRole("button", { name: "Upload", exact: true }).click()
    ]);

    await page.goto("/activities");
    await expect(page.getByRole("link", { name, exact: true })).toBeVisible();
  }
}

test.describe("local product journey", () => {
  test("redirects unauthenticated users, logs in, and logs out", async ({ page }) => {
    await page.goto("/");
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.getByText("Runnarr", { exact: true })).toBeVisible();
    await login(page);

    await page.getByRole("button", { name: "Log out" }).click();
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.getByRole("button", { name: "Log in" })).toBeVisible();
  });

  test("imports and inspects an activity, media, and export", async ({ page }, testInfo) => {
    await login(page);
    const projectName = testInfo.project.name;
    const name = activityName(projectName);
    await ensureActivityImported(page, projectName);

    await page.getByRole("link", { name, exact: true }).click();
    await expect(page.getByRole("heading", { name })).toBeVisible();
    await expect(page.getByText("Route", { exact: true })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Stats" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Intervals" })).toBeVisible();

    const png = Buffer.from(
      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
      "base64"
    );
    await page.locator('input[type="file"][accept="image/jpeg,image/png"]').setInputFiles({
      name: "e2e-photo.png",
      mimeType: "image/png",
      buffer: png
    });
    await expect(page.getByText("1 photo", { exact: true })).toBeVisible();

    await page.getByRole("button", { name: "Activity actions" }).click();
    await page.getByRole("menuitem", { name: "Export GPX" }).click();
    await expect(page.getByRole("dialog", { name: "Export GPX" })).toBeVisible();
    const downloadPromise = page.waitForEvent("download");
    await page.getByRole("dialog", { name: "Export GPX" }).getByRole("button", { name: "Download" }).click();
    const download = await downloadPromise;
    expect(download.suggestedFilename()).toMatch(/\.gpx$/);
  });

  test("covers calendar, health, gear, tools, and settings", async ({ page }) => {
    await login(page);

    await page.getByRole("link", { name: "Calendar" }).click();
    await expect(page.getByRole("heading", { name: "Calendar" })).toBeVisible();
    await expect(page.getByText("Monthly activity calendar", { exact: true })).toBeVisible();

    await page.getByRole("link", { name: "Health" }).click();
    await expect(page.getByRole("heading", { name: "Health" })).toBeVisible();
    await expect(page.getByText("Daily metrics", { exact: true })).toBeVisible();
    await expect(page.locator("strong").filter({ hasText: "12,450" })).toBeVisible();

    await page.getByRole("link", { name: "Gear" }).click();
    await expect(page.getByRole("heading", { name: "Gear" })).toBeVisible();
    await expect(page.getByText("E2E Daily Trainers", { exact: true })).toBeVisible();
    await page.locator("a.gear-card").filter({ hasText: "E2E Daily Trainers" }).click();
    await expect(page.getByRole("heading", { name: "E2E Daily Trainers" })).toBeVisible();

    await page.getByRole("link", { name: "Tools" }).click();
    await expect(page.getByRole("heading", { name: "Tools" })).toBeVisible();
    await page.getByLabel("Distance").first().fill("10");
    await page.getByLabel("Time").first().fill("45:00");
    await page.getByRole("button", { name: "Calculate", exact: true }).first().click();
    await expect(page.getByText("4:30 /km", { exact: true })).toBeVisible();

    await page.getByRole("link", { name: "Settings" }).click();
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
    await Promise.all([
      page.waitForResponse((response) => response.url().includes("/api/preferences") && response.request().method() === "PATCH" && response.ok()),
      page.getByRole("group", { name: "Theme preference" }).getByRole("button", { name: "Dark" }).click()
    ]);
    await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
    await page.reload();
    await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  });

  test("keeps navigation and key controls usable on mobile", async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== "mobile-chromium", "This assertion targets the mobile project.");
    await login(page);
    const widths = await page.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth
    }));
    expect(widths.scrollWidth).toBeLessThanOrEqual(widths.clientWidth);

    await page.getByRole("link", { name: "Activities" }).click();
    await expect(page.getByRole("heading", { name: "Activities" })).toBeVisible();
    await page.getByRole("link", { name: "Settings" }).click();
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Upload", exact: true })).toBeVisible();
  });
});
