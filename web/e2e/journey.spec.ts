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

function isMobileProject(projectName: string) {
  return projectName === "mobile-chromium";
}

function visibleActivityContainer(page: Page, mobile: boolean) {
  const selector = mobile
    ? ".activity-card-list:visible, .empty-state:visible"
    : ".activity-table-desktop:visible, .empty-state:visible";
  return page.locator(selector).first();
}

function visibleActivityLink(page: Page, name: string, mobile: boolean) {
  const container = mobile ? ".activity-card-list:visible" : ".activity-table-desktop:visible";
  return page.locator(container).getByRole("link", { name, exact: true });
}

async function expectNoHorizontalOverflow(page: Page) {
  const widths = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth
  }));
  expect(widths.scrollWidth).toBeLessThanOrEqual(widths.clientWidth);
}

async function openMobileMenu(page: Page) {
  const menu = page.getByRole("dialog", { name: "More navigation" });
  if (await menu.count() === 0) {
    await page.getByRole("button", { name: "More", exact: true }).click();
  }
  await expect(menu).toBeVisible();
  return menu;
}

async function navigateMobileMenu(page: Page, label: string) {
  const menu = await openMobileMenu(page);
  await menu.getByRole("link", { name: label, exact: true }).click();
}

async function navigateTo(page: Page, label: string, mobile: boolean) {
  if (mobile) {
    if (["Activities", "Calendar", "Health"].includes(label)) {
      await page.getByRole("navigation", { name: "Primary navigation" }).getByRole("link", { name: label, exact: true }).click();
    } else {
      await navigateMobileMenu(page, label);
    }
    return;
  }
  await page.locator(".sidebar").getByRole("link", { name: label, exact: true }).click();
}

async function login(page: Page, mobile: boolean) {
  await page.goto("/login");
  await page.getByLabel("Username").fill(username);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Log in" }).click();
  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByRole("heading", { name: "Dashboard" })).toBeVisible();
  if (mobile) {
    await expect(page.locator(".mobile-header")).toBeVisible();
    await expect(page.locator(".mobile-bottom-nav")).toBeVisible();
    await expect(page.locator(".sidebar")).toBeHidden();
    await expect(page.locator(".mobile-header-title")).toHaveText("Dashboard");
  }
}

async function logout(page: Page, mobile: boolean) {
  if (mobile) {
    const menu = await openMobileMenu(page);
    await menu.getByRole("button", { name: "Log out", exact: true }).click();
    return;
  }
  await page.locator(".sidebar").getByRole("button", { name: "Log out", exact: true }).click();
}

async function ensureActivityImported(page: Page, projectName: string, mobile: boolean) {
  const name = activityName(projectName);
  await page.goto("/activities");
  await expect(page.getByRole("heading", { name: "Activities" })).toBeVisible();
  await expect(visibleActivityContainer(page, mobile)).toBeVisible();

  if (await visibleActivityLink(page, name, mobile).count() === 0) {
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
    await expect(visibleActivityLink(page, name, mobile)).toBeVisible();
  }
}

test.describe("local product journey", () => {
  test("redirects unauthenticated users, logs in, and logs out", async ({ page }, testInfo) => {
    const mobile = isMobileProject(testInfo.project.name);
    await page.goto("/");
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.getByText("Runnarr", { exact: true })).toBeVisible();
    await login(page, mobile);

    await logout(page, mobile);
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.getByRole("button", { name: "Log in" })).toBeVisible();
  });

  test("imports and inspects an activity, media, and export", async ({ page }, testInfo) => {
    const mobile = isMobileProject(testInfo.project.name);
    await login(page, mobile);
    const projectName = testInfo.project.name;
    const name = activityName(projectName);
    await ensureActivityImported(page, projectName, mobile);

    await visibleActivityLink(page, name, mobile).click();
    await expect(page.getByRole("heading", { name })).toBeVisible();
    await expect(page.getByText("Route", { exact: true })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Stats" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Intervals" })).toBeVisible();
    if (mobile) {
      await expect(page.locator(".mobile-header-title")).toHaveText("Activity");
      await expectNoHorizontalOverflow(page);
    }

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

  test("pins an activity photo to a map location", async ({ page }, testInfo) => {
    const mobile = isMobileProject(testInfo.project.name);
    const projectName = testInfo.project.name;
    const name = activityName(projectName);
    await login(page, mobile);
    await ensureActivityImported(page, projectName, mobile);

    await visibleActivityLink(page, name, mobile).click();
    await expect(page.getByRole("heading", { name })).toBeVisible();

    const png = Buffer.from(
      "iVBORw0KGgoAAAANSUhEUgAAAAIAAAABCAYAAAD0In+KAAAADklEQVR4nGP4z8AAQv8BD/kD/YURmXYAAAAASUVORK5CYII=",
      "base64"
    );
    const filename = `e2e-pinned-photo-${projectSlug(projectName)}.png`;
    await page.locator('input[type="file"][accept="image/jpeg,image/png"]').setInputFiles({
      name: filename,
      mimeType: "image/png",
      buffer: png
    });

    const mediaPreview = page.locator(".media-preview-dialog");
    await page.getByRole("button", { name: `Open ${filename}` }).click();
    await expect(mediaPreview).toBeVisible();
    await mediaPreview.getByRole("button", { name: /Pin to map|Move pin/ }).click();
    await expect(page.getByText("Click the map to place this photo.", { exact: true })).toBeVisible();

    const map = page.locator(".route-map:visible");
    await expect(map).toBeVisible();
    await map.scrollIntoViewIfNeeded();
    const patchPromise = page.waitForResponse((response) =>
      response.url().includes("/media/") && response.request().method() === "PATCH" && response.ok()
    );
    await map.click({ position: { x: 80, y: 80 } });
    await patchPromise;
    await expect(mediaPreview).toBeHidden();

    await page.reload();
    await expect(page.getByRole("heading", { name })).toBeVisible();
    await page.getByRole("button", { name: `Open ${filename}` }).click();
    await expect(page.locator(".media-preview-dialog")).toContainText("GPS");
  });

  test("covers calendar, health, gear, tools, and settings", async ({ page }, testInfo) => {
    const mobile = isMobileProject(testInfo.project.name);
    await login(page, mobile);

    await navigateTo(page, "Calendar", mobile);
    await expect(page.getByRole("heading", { name: "Calendar" })).toBeVisible();
    await expect(page.getByText("Monthly activity calendar", { exact: true })).toBeVisible();
    if (mobile) {
      await expect(page.locator(".mobile-calendar-agenda:visible")).toBeVisible();
      await expect(page.locator(".calendar-grid")).toBeHidden();
      await expect(page.locator(".mobile-header-title")).toHaveText("Calendar");
    }

    await navigateTo(page, "Health", mobile);
    await expect(page.getByRole("heading", { name: "Health" })).toBeVisible();
    await expect(page.getByText("Daily metrics", { exact: true })).toBeVisible();
    await expect(page.locator(".metric-grid strong").filter({ hasText: "12,450" })).toBeVisible();
    if (mobile) {
      const healthCard = page.locator(".health-card-list:visible .health-card").filter({ hasText: "12,450" }).first();
      await expect(healthCard).toBeVisible();
      await healthCard.click();
      await expect(healthCard).toContainText("Selected");
      await expect(page.locator(".mobile-header-title")).toHaveText("Health");
    }

    await navigateTo(page, "Gear", mobile);
    await expect(page.getByRole("heading", { name: "Gear" })).toBeVisible();
    await expect(page.getByText("E2E Daily Trainers", { exact: true })).toBeVisible();
    await page.locator("a.gear-card").filter({ hasText: "E2E Daily Trainers" }).click();
    await expect(page.getByRole("heading", { name: "E2E Daily Trainers" })).toBeVisible();

    await navigateTo(page, "Tools", mobile);
    await expect(page.getByRole("heading", { name: "Tools" })).toBeVisible();
    await page.getByLabel("Distance").first().fill("10");
    await page.getByLabel("Time").first().fill("45:00");
    await page.getByRole("button", { name: "Calculate", exact: true }).first().click();
    await expect(page.getByText("4:30 /km", { exact: true })).toBeVisible();

    await navigateTo(page, "Settings", mobile);
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
    if (mobile) {
      await expect(page.locator(".mobile-header-title")).toHaveText("Settings");
      await expect(page.locator('input[type="file"][accept=".gpx,.tcx,.fit"]')).toBeVisible();
    }
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
    await login(page, true);
    await expectNoHorizontalOverflow(page);

    const primaryNavigation = page.getByRole("navigation", { name: "Primary navigation" });
    await expect(primaryNavigation.getByRole("link")).toHaveCount(4);
    await expect(primaryNavigation.getByRole("button", { name: "More", exact: true })).toBeVisible();

    const menu = await openMobileMenu(page);
    await expect(menu.getByRole("link", { name: "Tools", exact: true })).toBeVisible();
    await expect(menu.getByRole("link", { name: "Gear", exact: true })).toBeVisible();
    await expect(menu.getByRole("link", { name: "Settings", exact: true })).toBeVisible();
    await menu.getByRole("button", { name: "Close navigation", exact: true }).click();
    await expect(menu).toBeHidden();

    await navigateTo(page, "Activities", true);
    await expect(page.getByRole("heading", { name: "Activities" })).toBeVisible();
    await ensureActivityImported(page, testInfo.project.name, true);
    await expect(page.locator(".activity-card-list:visible")).toBeVisible();
    await expect(page.locator(".activity-table-desktop")).toBeHidden();
    await expect(page.locator(".mobile-header-title")).toHaveText("Activities");
    await expectNoHorizontalOverflow(page);

    await navigateTo(page, "Settings", true);
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Upload", exact: true })).toBeVisible();
    await expect(page.locator(".sidebar")).toBeHidden();
    await expectNoHorizontalOverflow(page);
  });
});
