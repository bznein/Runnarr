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

async function loginAs(page: Page, username: string, password: string, mobile: boolean) {
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

async function login(page: Page, mobile: boolean) {
  await loginAs(page, username, password, mobile);
}

async function logout(page: Page, mobile: boolean) {
  if (mobile) {
    const menu = await openMobileMenu(page);
    await menu.getByRole("button", { name: "Log out", exact: true }).click();
    return;
  }
  await page.locator(".sidebar").getByRole("button", { name: "Log out", exact: true }).click();
}

async function ensureActivityImported(page: Page, projectName: string, mobile: boolean, requestedName = activityName(projectName)) {
  const name = requestedName;
  await page.goto("/activities");
  await expect(page.getByRole("heading", { name: "Activities" })).toBeVisible();
  await expect(visibleActivityContainer(page, mobile)).toBeVisible();

  if (await visibleActivityLink(page, name, mobile).count() === 0) {
    const original = await readFile(gpxPath, "utf8");
    const fixtureDate = new Date().toISOString().slice(0, 10);
    // Keep the sequential browser projects at distinct times so navigation
    // never falls back to UUID ordering for the imported activities.
    const fixtureMinuteOffset = mobile ? 30 : 0;
    const fixture = original
      .replace(/2026-07-01T06:(\d{2}):00Z/g, (_, minute) => {
        const shiftedMinute = Number(minute) + fixtureMinuteOffset;
        return `${fixtureDate}T06:${String(shiftedMinute).padStart(2, "0")}:00Z`;
      })
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

    await page.getByRole("button", { name: "Match", exact: true }).click();
    const plannedMatchDialog = page.getByRole("dialog", { name: "Match planned run" });
    await expect(plannedMatchDialog).toBeVisible();
    const agendaDays = plannedMatchDialog.locator(".planned-match-agenda-day");
    await expect(agendaDays).toHaveCount(3);
    await expect(agendaDays.nth(1)).toHaveClass(/planned-match-agenda-day--target/);
    await expect(agendaDays.nth(0).locator(".planned-match-agenda-full-date")).toHaveText(/2026/);
    await expect(agendaDays.nth(1).getByText("E2E Planned Run", { exact: true })).toBeVisible();
    await expect(agendaDays.nth(1).getByText("E2E Planned Speed Work", { exact: true })).toBeVisible();
    await expect(agendaDays.nth(1).getByRole("radio")).toHaveCount(2);
    await expect(agendaDays.nth(2).getByText("E2E Planned Long Run", { exact: true })).toBeVisible();
    let releasePlannedCandidates = () => {};
    const plannedCandidatesGate = new Promise<void>((resolve) => {
      releasePlannedCandidates = resolve;
    });
    await page.route("**/api/activities/*/planned-match-candidates?windowDays=30", async (route) => {
      await plannedCandidatesGate;
      await route.continue();
    });
    await plannedMatchDialog.getByRole("button", { name: "Load more plans", exact: true }).click();
    await expect(plannedMatchDialog).toBeVisible();
    await expect(plannedMatchDialog.getByRole("status")).toHaveText("Loading more plans…");
    releasePlannedCandidates();
    await expect(agendaDays).toHaveCount(4);
    await expect(plannedMatchDialog.getByText("E2E Planned Far Run", { exact: true })).toBeVisible();
    await page.unroute("**/api/activities/*/planned-match-candidates?windowDays=30");
    if (mobile) {
      await expectNoHorizontalOverflow(page);
    }
    await plannedMatchDialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(plannedMatchDialog).toBeHidden();

    await page.reload();
    await expect(page.getByRole("heading", { name })).toBeVisible();
    await page.route("**/api/activities/*/planned-match-candidates?windowDays=30", async (route) => {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "planned candidates unavailable" })
      });
    });
    await page.getByRole("button", { name: "Match", exact: true }).click();
    const failedPlannedMatchDialog = page.getByRole("dialog", { name: "Match planned run" });
    await failedPlannedMatchDialog.getByRole("button", { name: "Load more plans", exact: true }).click();
    await expect(failedPlannedMatchDialog).toBeVisible();
    await expect(failedPlannedMatchDialog.getByText("planned candidates unavailable", { exact: true })).toBeVisible({ timeout: 15_000 });
    await page.unroute("**/api/activities/*/planned-match-candidates?windowDays=30");
    await failedPlannedMatchDialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(failedPlannedMatchDialog).toBeHidden();
    await page.getByRole("button", { name: "Match", exact: true }).click();
    await expect(failedPlannedMatchDialog.getByRole("button", { name: "Retry loading plans", exact: true })).toBeVisible();
    await failedPlannedMatchDialog.getByRole("button", { name: "Retry loading plans", exact: true }).click();
    await expect(failedPlannedMatchDialog.getByText("E2E Planned Far Run", { exact: true })).toBeVisible();
    await failedPlannedMatchDialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(failedPlannedMatchDialog).toBeHidden();

    await page.route("**/api/activities/*/planned-match-candidates?windowDays=7", async (route) => {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "initial planned candidates unavailable" })
      });
    });
    await page.reload();
    await expect(page.getByRole("heading", { name })).toBeVisible();
    await page.getByRole("button", { name: "Match", exact: true }).click();
    const initialFailurePlannedMatchDialog = page.getByRole("dialog", { name: "Match planned run" });
    await expect(initialFailurePlannedMatchDialog.getByText("initial planned candidates unavailable", { exact: true })).toBeVisible();
    await expect(initialFailurePlannedMatchDialog.getByRole("button", { name: "Retry loading plans", exact: true })).toBeVisible();
    await page.unroute("**/api/activities/*/planned-match-candidates?windowDays=7");
    let releaseInitialRetry = () => {};
    const initialRetryGate = new Promise<void>((resolve) => {
      releaseInitialRetry = resolve;
    });
    await page.route("**/api/activities/*/planned-match-candidates?windowDays=7", async (route) => {
      await initialRetryGate;
      await route.continue();
    });
    await initialFailurePlannedMatchDialog.getByRole("button", { name: "Retry loading plans", exact: true }).click();
    const retryingPlans = initialFailurePlannedMatchDialog.getByRole("button", { name: "Retrying plans…", exact: true });
    await expect(retryingPlans).toBeDisabled();
    await expect(initialFailurePlannedMatchDialog.getByRole("status")).toHaveText("Retrying planned runs…");
    releaseInitialRetry();
    await expect(initialFailurePlannedMatchDialog.getByText("E2E Planned Long Run", { exact: true })).toBeVisible();
    await page.unroute("**/api/activities/*/planned-match-candidates?windowDays=7");
    await initialFailurePlannedMatchDialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(initialFailurePlannedMatchDialog).toBeHidden();

    await expect(page.getByText("Route", { exact: true })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Stats" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Intervals" })).toBeVisible();
    const previousActivity = page.getByRole("button", { name: "Previous activity" });
    await expect(previousActivity).toBeEnabled();

    await previousActivity.click();
    await expect(page.getByRole("heading", { name: "E2E Cycling Activity" })).toBeVisible();
    const cyclingNextActivity = page.getByRole("button", { name: "Next activity" });
    await expect(cyclingNextActivity).toBeEnabled();
    await cyclingNextActivity.click();
    await expect(page.getByRole("heading", { name })).toBeVisible();
    await expect(previousActivity).toBeEnabled();

    let releaseNavigation = () => {};
    const navigationGate = new Promise<void>((resolve) => {
      releaseNavigation = resolve;
    });
    await page.route("**/api/activities/*/navigation**", async (route) => {
      await navigationGate;
      await route.continue();
    });
    await previousActivity.click();
    await expect(page.getByRole("heading", { name: "E2E Cycling Activity" })).toBeVisible();
    await expect(cyclingNextActivity).toBeDisabled();
    releaseNavigation();
    await expect(cyclingNextActivity).toBeEnabled();
    await page.unroute("**/api/activities/*/navigation**");

    await cyclingNextActivity.click();
    await expect(page.getByRole("heading", { name })).toBeVisible();
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
    await expect(page.getByRole("button", { name: "Match", exact: true })).toBeVisible();
    await expect(page.locator(".planned-match-panel")).toHaveCount(0);

    await page.getByRole("button", { name: "Activity actions" }).click();
    await page.getByRole("menuitem", { name: "Export GPX" }).click();
    await expect(page.getByRole("dialog", { name: "Export GPX" })).toBeVisible();
    const downloadPromise = page.waitForEvent("download");
    await page.getByRole("dialog", { name: "Export GPX" }).getByRole("button", { name: "Download" }).click();
    const download = await downloadPromise;
    expect(download.suggestedFilename()).toMatch(/\.gpx$/);
    await page.goto("/activities");
    const cyclingActivity = visibleActivityLink(page, "E2E Cycling Activity", mobile);
    await expect(cyclingActivity).toBeVisible();
    await cyclingActivity.click();

    await expect(page.getByRole("heading", { name: "E2E Cycling Activity" })).toBeVisible();
    await expect(page.locator(".planned-match-panel")).toHaveCount(0);

    await page.goto("/activities");
    const swimmingActivity = visibleActivityLink(page, "E2E Pool Swim", mobile);
    await expect(swimmingActivity).toBeVisible();
    await swimmingActivity.click();

    await expect(page.getByRole("heading", { name: "E2E Pool Swim" })).toBeVisible();
    await expect(page.locator(".climbs-panel")).toHaveCount(0);
    await expect(page.locator(".climb-sensitivity-details")).toHaveCount(0);
  });

  test("excludes a matched planned run from all candidate windows until unmatch", async ({ page }, testInfo) => {
    const mobile = isMobileProject(testInfo.project.name);
    const plannedName = `E2E ${testInfo.project.name} Match Candidate`;
    const activityAName = `E2E ${testInfo.project.name} Match Activity A`;
    const activityCName = `E2E ${testInfo.project.name} Match Activity C`;

    await login(page, mobile);
    await ensureActivityImported(page, `${testInfo.project.name}-match-a`, mobile, activityAName);
    await visibleActivityLink(page, activityAName, mobile).click();
    const activityAID = new URL(page.url()).pathname.split("/").filter(Boolean).at(-1);
    expect(activityAID).toBeTruthy();

    await ensureActivityImported(page, `${testInfo.project.name}-match-c`, mobile, activityCName);
    await visibleActivityLink(page, activityCName, mobile).click();
    const activityCID = new URL(page.url()).pathname.split("/").filter(Boolean).at(-1);
    expect(activityCID).toBeTruthy();

    const sessionResponse = await page.request.get("/api/session");
    expect(sessionResponse.ok()).toBe(true);
    const session = await sessionResponse.json() as { csrfToken: string };
    const mutationHeaders = { "X-CSRF-Token": session.csrfToken };

    const candidates = async (windowDays: 7 | 30) => {
      const response = await page.request.get(`/api/activities/${activityCID}/planned-match-candidates?windowDays=${windowDays}`);
      expect(response.ok()).toBe(true);
      return response.json() as Promise<{ candidates: Array<{ id: string; name: string }> }>;
    };

    await page.request.delete(`/api/activities/${activityAID}/planned-match`, { headers: mutationHeaders });
    const beforeMatch = await candidates(7);
    const planned = beforeMatch.candidates.find((candidate) => candidate.name === plannedName);
    expect(planned, `${plannedName} should start as an eligible candidate`).toBeTruthy();

    try {
      const matchResponse = await page.request.post(`/api/activities/${activityAID}/planned-match`, {
        headers: mutationHeaders,
        data: { plannedActivityId: planned!.id }
      });
      expect(matchResponse.ok()).toBe(true);

      for (const windowDays of [7, 30] as const) {
        const afterMatch = await candidates(windowDays);
        expect(afterMatch.candidates.map((candidate) => candidate.id)).not.toContain(planned!.id);
      }
    } finally {
      const unmatchResponse = await page.request.delete(`/api/activities/${activityAID}/planned-match`, { headers: mutationHeaders });
      expect(unmatchResponse.ok()).toBe(true);
    }

    for (const windowDays of [7, 30] as const) {
      const afterUnmatch = await candidates(windowDays);
      expect(afterUnmatch.candidates.map((candidate) => candidate.id)).toContain(planned!.id);
    }
  });

  test("exits support view to the dashboard from an activity", async ({ page }, testInfo) => {
    const mobile = isMobileProject(testInfo.project.name);
    const supportUsername = `e2e-support-${projectSlug(testInfo.project.name)}`;
    const supportPassword = "e2e-support-password-123";
    const supportActivity = `E2E ${testInfo.project.name} Support Activity`;

    await login(page, mobile);
    await page.goto("/settings");
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();

    const userManagement = page.locator(".user-management-panel");
    const supportUserRow = userManagement.locator("tbody tr").filter({ hasText: supportUsername });
    if (await supportUserRow.count() === 0) {
      await userManagement.getByPlaceholder("Username").fill(supportUsername);
      await userManagement.getByPlaceholder("Display name").fill("E2E Support User");
      await userManagement.getByPlaceholder("Temporary password").fill(supportPassword);
      await userManagement.getByRole("button", { name: "Create", exact: true }).click();
    }
    await expect(supportUserRow).toBeVisible();

    await logout(page, mobile);
    await loginAs(page, supportUsername, supportPassword, mobile);
    await ensureActivityImported(page, testInfo.project.name, mobile, supportActivity);
    await logout(page, mobile);
    await login(page, mobile);

    await page.goto("/settings");
    const adminSupportUserRow = page.locator(".user-management-panel tbody tr").filter({ hasText: supportUsername });
    await adminSupportUserRow.getByRole("button", { name: "Support view", exact: true }).click();
    await expect(page.getByText("Read-only support view: E2E Support User", { exact: true })).toBeVisible();

    await page.goto("/activities");
    await visibleActivityLink(page, supportActivity, mobile).click();
    await expect(page.getByRole("heading", { name: supportActivity })).toBeVisible();
    await page.getByRole("button", { name: "Exit support view", exact: true }).click();

    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByRole("heading", { name: "Dashboard" })).toBeVisible();
    await expect(page.locator(".support-banner")).toHaveCount(0);
    await expect(page.getByText("Activity not found", { exact: true })).toHaveCount(0);
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

    await navigateTo(page, "Activities", mobile);
    await expect(page.getByRole("heading", { name: "Activities" })).toBeVisible();
    await page.getByRole("button", { name: "Filter", exact: true }).click();
    const activityFilters = page.getByRole("dialog", { name: "Activities" });
    await expect(activityFilters.getByLabel("Search by name")).toBeVisible();
    await expect(activityFilters.getByText("Activity types", { exact: true })).toBeVisible();
    await expect(activityFilters.getByText("Show only", { exact: true })).toHaveCount(0);
    await expect(activityFilters.getByText("Exclude", { exact: true })).toHaveCount(0);
    await expect(activityFilters.getByRole("button", { name: "Select all", exact: true })).toBeDisabled();
    await activityFilters.getByRole("button", { name: "Clear all", exact: true }).click();
    await activityFilters.getByRole("button", { name: "Apply", exact: true }).click();
    await expect(page.getByText("No activities match these filters", { exact: true })).toBeVisible();

    await page.getByRole("button", { name: /^Filter/ }).click();
    const resetActivityFilters = page.getByRole("dialog", { name: "Activities" });
    await resetActivityFilters.getByRole("button", { name: "Select all", exact: true }).click();
    await resetActivityFilters.getByRole("button", { name: "Apply", exact: true }).click();

    await navigateTo(page, "Calendar", mobile);
    await expect(page.getByRole("heading", { name: "Calendar" })).toBeVisible();
    await expect(page.getByText("Monthly activity calendar", { exact: true })).toBeVisible();
    if (mobile) {
      await expect(page.locator(".mobile-calendar-agenda:visible")).toBeVisible();
      await expect(page.locator(".calendar-grid")).toBeHidden();
      await expect(page.locator(".mobile-header-title")).toHaveText("Calendar");
    }

    const todayDayLink = page.locator(".calendar-day-link:visible").filter({ hasText: String(new Date().getDate()) }).first();
    await expect(todayDayLink).toBeVisible();
    await todayDayLink.click();
    await expect(page.getByRole("heading", { name: "Day view" })).toBeVisible();
    await expect(page.getByText("Daily Garmin metrics", { exact: true })).toBeVisible();
    await expect(page.locator(".metric-grid strong").filter({ hasText: "12,450" })).toBeVisible();
    await expect(page.getByRole("link", { name: "E2E Pool Swim", exact: true })).toBeVisible();
    await page.getByRole("link", { name: "Back to calendar", exact: true }).click();
    await expect(page.getByText("Monthly activity calendar", { exact: true })).toBeVisible();

    await navigateTo(page, "Health", mobile);
    await expect(page.getByRole("heading", { name: "Health" })).toBeVisible();
    await expect(page.getByText(/^Data for /)).toBeVisible();
    await expect(page.getByText("Data for Today", { exact: true })).toHaveCount(0);
    await expect(page.locator(".health-summary .health-controls-panel")).toBeVisible();
    await expect(page.getByText("Daily metrics", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Sync health", exact: true })).toHaveCount(0);
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
    await expect(page.getByRole("button", { name: "Sync health", exact: true })).toBeVisible();
    await expect(page.getByText("Health from", { exact: true })).toBeVisible();
    if (mobile) {
      await expect(page.locator(".mobile-header-title")).toHaveText("Settings");
      await expect(page.locator('input[type="file"][accept=".gpx,.tcx,.fit"]')).toBeVisible();
    }
    const themePicker = page.getByRole("group", { name: "Color theme" });
    await expect(themePicker.getByRole("radio", { name: "Ocean" })).toBeVisible();
    await Promise.all([
      page.waitForResponse((response) => response.url().includes("/api/preferences") && response.request().method() === "PATCH" && response.ok()),
      themePicker.getByRole("radio", { name: "Ocean" }).check()
    ]);
    await expect(page.locator("html")).toHaveAttribute("data-theme", "ocean");
    await page.reload();
    await expect(page.locator("html")).toHaveAttribute("data-theme", "ocean");
    await Promise.all([
      page.waitForResponse((response) => response.url().includes("/api/preferences") && response.request().method() === "PATCH" && response.ok()),
      themePicker.getByRole("radio", { name: "Midnight" }).check()
    ]);
    await expect(page.locator("html")).toHaveAttribute("data-theme", "midnight");
    await expect(themePicker.getByRole("radio", { name: "Midnight" })).toBeChecked();
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

    const strengthCard = page.locator(".activity-card-list:visible .activity-card").filter({
      has: page.getByRole("link", { name: "E2E Strength Training", exact: true })
    });
    await expect(strengthCard).toBeVisible();
    await expect(strengthCard.getByText("Distance", { exact: true })).toHaveCount(0);
    await expect(strengthCard.getByText("Time", { exact: true })).toBeVisible();
    await strengthCard.getByRole("link", { name: "E2E Strength Training", exact: true }).click();

    const strengthSummary = page.locator(".metric-grid").first();
    await expect(strengthSummary.getByText("Distance", { exact: true })).toHaveCount(0);
    await expect(strengthSummary.getByText("Pace", { exact: true })).toHaveCount(0);
    await expect(strengthSummary.getByText("GAP", { exact: true })).toHaveCount(0);
    await expect(strengthSummary.getByText("Moving Time", { exact: true })).toBeVisible();

    await navigateTo(page, "Settings", true);
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Upload", exact: true })).toBeVisible();
    await expect(page.locator(".sidebar")).toBeHidden();
    await expectNoHorizontalOverflow(page);
  });
});
