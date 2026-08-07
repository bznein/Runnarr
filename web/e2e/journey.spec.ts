import { expect, test, type Page } from "@playwright/test";
import { readFile } from "node:fs/promises";
import path from "node:path";

const username = process.env.RUNNARR_E2E_USERNAME ?? "e2e-admin";
const password = process.env.RUNNARR_E2E_PASSWORD ?? "e2e-password-123";
const fixtureDate = process.env.RUNNARR_E2E_FIXTURE_DATE ?? new Date().toISOString().slice(0, 10);
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
  } else {
    await page.locator(".sidebar").getByRole("button", { name: "Log out", exact: true }).click();
  }
  await expect(page).toHaveURL(/\/login(?:\?|$)/);
}

async function ensureActivityImported(page: Page, projectName: string, mobile: boolean, requestedName = activityName(projectName)) {
  const name = requestedName;
  await page.goto("/activities");
  await expect(page.getByRole("heading", { name: "Activities" })).toBeVisible();
  await expect(visibleActivityContainer(page, mobile)).toBeVisible();

  if (await visibleActivityLink(page, name, mobile).count() === 0) {
    const original = await readFile(gpxPath, "utf8");
    // Keep the sequential browser projects at distinct times so navigation
    // never falls back to UUID ordering for the imported activities.
    const fixtureMinuteOffset = mobile ? 30 : 0;
    const fixture = original
      .replace(/2026-07-01T06:(\d{2}):00Z/g, (_, minute) => {
        const shiftedMinute = Number(minute) + fixtureMinuteOffset;
        return `${fixtureDate}T06:${String(shiftedMinute).padStart(2, "0")}:00Z`;
      })
      .replaceAll("53.349800", mobile ? "53.359800" : "53.349800")
      .replaceAll("53.350800", mobile ? "53.360800" : "53.350800")
      .replaceAll("53.351600", mobile ? "53.361600" : "53.351600")
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
  test("redirects unauthenticated users, logs in, and logs out", { tag: "@visual-auth" }, async ({ page }, testInfo) => {
    const mobile = isMobileProject(testInfo.project.name);
    await page.goto("/");
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.getByText("Runnarr", { exact: true })).toBeVisible();
    await login(page, mobile);

    await logout(page, mobile);
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.getByRole("button", { name: "Log in" })).toBeVisible();
  });

  test("provides a complete training-sheet-only matching mode", { tag: "@visual-simple-matching" }, async ({ page }, testInfo) => {
    const mobile = isMobileProject(testInfo.project.name);
    const name = activityName(testInfo.project.name);
    let matchState: "unmatched" | "attention" | "complete" = "unmatched";
    let initialCandidates: Record<string, unknown> | undefined;
    const plannedDate = fixtureDate;
    const matchedPlan = {
      id: "00000000-0000-4000-8000-000000000203",
      source: "training_sheet",
      sourceId: "e2e-simple-plan",
      workbookId: "e2e-workbook",
      sheetId: "e2e-sheet",
      sheetTitle: "E2E Plan",
      planCell: "A1",
      feedbackCell: "C19",
      plannedDate,
      name: "2mins E2E Planned Run",
      sportType: "Run",
      status: "completed",
      sourceUrl: "https://docs.google.com/spreadsheets/d/e2e-workbook/edit#gid=e2e-sheet"
    };

    await page.route("**/api/providers/google/status", async (route) => {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ configured: true, connected: true, writeReady: true, provider: "google_sheets" }) });
    });
    await page.route("**/api/activities/*/planned-match-candidates?*", async (route) => {
      if (matchState === "unmatched") {
        if (!initialCandidates) {
          const response = await route.fetch();
          initialCandidates = await response.json() as Record<string, unknown>;
        }
        await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(initialCandidates) });
        return;
      }
      const complete = matchState === "complete";
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          candidates: [],
          hasMore: false,
          matched: matchedPlan,
          writeback: {
            plannedActivityId: matchedPlan.id,
            activityId: "e2e-simple-activity",
            jobStatus: complete ? "completed" : "failed",
            summaryStatus: complete ? "completed" : "failed",
            summaryError: complete ? undefined : "E2E writeback failed",
            intervalsStatus: "not_applicable",
            feedbackStatus: "not_provided"
          }
        })
      });
    });
    await page.route("**/api/activities/*/planned-match-preview", async (route) => {
      const requestBody = route.request().postDataJSON() as { plannedActivityId: string };
      const activityId = new URL(route.request().url()).pathname.split("/").filter(Boolean).at(-2)!;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ preview: {
          activityId,
          plannedActivityId: requestBody.plannedActivityId,
          sheetTitle: "E2E Plan",
          sheetUrl: "https://docs.google.com/spreadsheets/d/e2e-workbook/edit",
          fingerprint: "e2e-simple-preview",
          changes: [{ range: "A1", section: "summary", label: "Summary", currentValue: "", proposedValue: "Completed", status: "write" }],
          grid: { startRow: 1, endRow: 1, startColumn: 1, endColumn: 1, formattingAvailable: false, columns: [], rows: [] },
          writeCount: 1,
          conflictCount: 0
        } })
      });
    });
    await page.route("**/api/activities/*/planned-match-apply", async (route) => {
      matchState = "attention";
      await route.fulfill({ status: 202, contentType: "application/json", body: JSON.stringify({ planned: matchedPlan, writebackJobId: "e2e-writeback", status: "running" }) });
    });
    await page.route("**/api/activities/*/planned-writeback", async (route) => {
      matchState = "complete";
      await route.fulfill({ status: 202, contentType: "application/json", body: JSON.stringify({ jobId: "e2e-retry", status: "running" }) });
    });
    await page.route("**/api/activities/*/planned-match", async (route) => {
      if (route.request().method() !== "DELETE") {
        await route.continue();
        return;
      }
      matchState = "unmatched";
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ matched: false }) });
    });

    await login(page, mobile);
    await ensureActivityImported(page, testInfo.project.name, mobile);
    await page.goto("/settings");
    const experience = page.locator(".simple-mode-settings");
    await expect(experience.getByRole("link", { name: "Open simple mode", exact: true })).toHaveAttribute("href", "/simple");
    await Promise.all([
      page.waitForResponse((response) => response.url().includes("/api/preferences") && response.request().method() === "PATCH" && response.ok()),
      experience.getByLabel("Use simple mode by default").check()
    ]);

    await page.goto("/");
    await expect(page).toHaveURL(/\/simple$/);
    await expect(page.getByRole("heading", { name: "Match completed runs" })).toBeVisible();
    await expect(page.locator(".sidebar")).toHaveCount(0);
    await expect(page.locator(".mobile-bottom-nav")).toHaveCount(0);
    await expect(page.getByText("E2E Cycling Activity", { exact: true })).toHaveCount(0);
    const matchedRow = page.locator(".simple-activity-row").filter({ hasText: "E2E Calendar Matched Run" });
    await expect(matchedRow.getByText("Needs attention", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Needs attention", exact: true }).click();
    await expect(page).toHaveURL(/matchState=attention/);
    await expect(matchedRow).toBeVisible();
    await page.getByRole("button", { name: "All", exact: true }).click();

    await page.getByRole("link", { name: new RegExp(name) }).click();
    await expect(page.getByRole("heading", { name })).toBeVisible();
    await expect(page.getByText("Continuous run", { exact: true })).toBeVisible();
    await expect(page.getByRole("dialog", { name: "Match planned run" })).toHaveCount(0);
    const inlineMatch = page.locator(".simple-match-form");
    await expect(inlineMatch.getByRole("heading", { name: "Match planned run" })).toBeVisible();
    await expect(inlineMatch.getByText("2mins E2E Planned Run", { exact: true })).toBeVisible();
    await inlineMatch.getByRole("button", { name: "Preview changes", exact: true }).click();
    await expect(inlineMatch.getByText("Sheet preview", { exact: true })).toBeVisible();
    await inlineMatch.getByRole("button", { name: "Apply match & write back", exact: true }).click();

    const matchedPanel = page.locator(".simple-matched-panel");
    await expect(matchedPanel.getByText("Matched planned run", { exact: true })).toBeVisible();
    const simpleTrainingSheetLink = matchedPanel.getByRole("link", { name: "Training sheet", exact: true });
    await expect(simpleTrainingSheetLink).toHaveAttribute("href", "https://docs.google.com/spreadsheets/d/e2e-workbook/edit#gid=e2e-sheet");
    await expect(simpleTrainingSheetLink).toHaveAttribute("target", "_blank");
    await expect(simpleTrainingSheetLink).toHaveAttribute("rel", "noreferrer");
    await expect(matchedPanel.getByText("E2E writeback failed", { exact: false })).toBeVisible();
    await matchedPanel.getByRole("button", { name: "Retry writeback", exact: true }).click();
    await expect(matchedPanel.getByText("Complete", { exact: true }).first()).toBeVisible();
    await expect(matchedPanel.getByText("Awaiting reflection", { exact: true })).toBeVisible();

    const simpleDetailPath = new URL(page.url()).pathname;
    await page.goto(`${simpleDetailPath.replace(/^\/simple/, "")}#check-in`);
    await expect(page.getByRole("dialog", { name: "RPE & feedback" })).toBeVisible();
    await page.getByRole("button", { name: "Close RPE and feedback" }).click();
    await page.goto(simpleDetailPath);

    page.once("dialog", (dialog) => void dialog.accept());
    await matchedPanel.getByRole("button", { name: "Unmatch", exact: true }).click();
    await expect(inlineMatch.getByRole("heading", { name: "Match planned run" })).toBeVisible();

    await page.getByRole("button", { name: "Exit simple mode", exact: true }).click();
    await expect.poll(() => new URL(page.url()).pathname).toMatch(/^\/activities\//);
    await page.goto("/");
    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByRole("heading", { name: "Dashboard" })).toBeVisible();
    if (mobile) await expectNoHorizontalOverflow(page);
  });

  test("imports and inspects an activity, media, and export", { tag: "@visual-activity-inspection" }, async ({ page }, testInfo) => {
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
    await expect(agendaDays.nth(1).getByText("2mins E2E Planned Run", { exact: true })).toBeVisible();
    await expect(agendaDays.nth(1).getByText("E2E Planned Speed Work", { exact: true })).toBeVisible();
    await expect(agendaDays.nth(1).getByRole("radio")).toHaveCount(2);
    const strongCandidate = agendaDays.nth(1).locator(".planned-match-candidate").filter({ hasText: "2mins E2E Planned Run" });
    await expect(strongCandidate.locator(".planned-match-score--strong")).toHaveText("100/100");
    await expect(strongCandidate.getByText("Suggested", { exact: true })).toBeVisible();
    await expect(strongCandidate.getByText("2 min activity vs 2 min plan", { exact: true })).toBeVisible();
    await expect(strongCandidate.getByText("Both continuous runs", { exact: true })).toBeVisible();
    await expect(strongCandidate.getByRole("radio")).toBeChecked();
    const structuredMismatchCandidate = agendaDays.nth(1).locator(".planned-match-candidate").filter({ hasText: "E2E Planned Speed Work" });
    await expect(structuredMismatchCandidate.locator(".planned-match-score--weak")).toHaveText("50/100");
    await expect(structuredMismatchCandidate.getByText("Planned intervals; activity is continuous", { exact: true })).toBeVisible();
    await expect(structuredMismatchCandidate.getByText("Suggested", { exact: true })).toHaveCount(0);
    await expect(structuredMismatchCandidate.getByRole("link", { name: "View workout", exact: true })).toHaveAttribute("href", "/workouts/00000000-0000-4000-8000-000000000170");
    const durationMismatchCandidate = agendaDays.nth(2).locator(".planned-match-candidate").filter({ hasText: "2 hours E2E Planned Long Run" });
    await expect(durationMismatchCandidate).toBeVisible();
    await expect(durationMismatchCandidate.locator(".planned-match-score--weak")).toHaveText("48/100");
    await expect(durationMismatchCandidate.getByText("2 min activity vs 2 hr plan", { exact: true })).toBeVisible();
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
    await expect(initialFailurePlannedMatchDialog.getByText("2 hours E2E Planned Long Run", { exact: true })).toBeVisible();
    await page.unroute("**/api/activities/*/planned-match-candidates?windowDays=7");
    await initialFailurePlannedMatchDialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(initialFailurePlannedMatchDialog).toBeHidden();

    await expect(page.getByText("Route", { exact: true })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Stats" })).toHaveAttribute("aria-selected", "true");
    await expect(page.getByRole("tab", { name: "Intervals" })).toHaveCount(0);
    await expect(page.getByText("Activity graph", { exact: true })).toBeVisible();
    const previousActivity = page.getByRole("button", { name: "Previous activity" });
    await expect(previousActivity).toBeEnabled();

    await previousActivity.click();
    await expect(page.getByRole("heading", { name: "E2E Cycling Activity" })).toBeVisible();
    const cyclingIntervalsTab = page.getByRole("tab", { name: "Intervals" });
    await expect(cyclingIntervalsTab).toBeVisible();
    await cyclingIntervalsTab.click();
    await expect(cyclingIntervalsTab).toHaveAttribute("aria-selected", "true");
    await expect(page.locator(".activity-intervals-panel").getByText("Intervals", { exact: true })).toBeVisible();
    const cyclingNextActivity = page.getByRole("button", { name: "Next activity" });
    await expect(cyclingNextActivity).toBeEnabled();
    await cyclingNextActivity.click();
    await expect(page.getByRole("heading", { name })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Intervals" })).toHaveCount(0);
    await expect(page.getByRole("tab", { name: "Stats" })).toHaveAttribute("aria-selected", "true");
    await expect(page.getByText("Activity graph", { exact: true })).toBeVisible();
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
    await expect(page.locator(".activity-navigation")).toHaveAttribute("aria-busy", "false");
    if (mobile) {
      await expect(page.locator(".mobile-header-title")).toHaveText("Activity");
      await expectNoHorizontalOverflow(page);
    }

    const png = Buffer.from(
      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
      "base64"
    );
    const activityId = new URL(page.url()).pathname.split("/").pop();
    const fileChooserPromise = page.waitForEvent("filechooser");
    await page.getByRole("button", { name: "Add photos" }).click();
    const fileChooser = await fileChooserPromise;
    const uploadResponsePromise = page.waitForResponse((response) =>
      response.url().includes(`/api/activities/${activityId}/media`) && response.request().method() === "POST"
    );
    await fileChooser.setFiles({
      name: "e2e-photo.png",
      mimeType: "image/png",
      buffer: png
    });
    const uploadResponse = await uploadResponsePromise;
    expect(uploadResponse.ok()).toBeTruthy();
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
    const swimmingIntervalsTab = page.getByRole("tab", { name: "Intervals" });
    await expect(swimmingIntervalsTab).toBeVisible();
    await swimmingIntervalsTab.click();
    await expect(page.getByText("Laps", { exact: true })).toBeVisible();
    await expect(page.getByText("No structured workout steps were provided; showing recorded laps.", { exact: true })).toBeVisible();
  });

  test("uses the course library, saves an activity route, and reviews a GPX import", { tag: "@visual-courses" }, async ({ page }, testInfo) => {
    const mobile = isMobileProject(testInfo.project.name);
    await login(page, mobile);

    await navigateTo(page, "Courses", mobile);
    await expect(page.getByRole("heading", { name: "Courses", exact: true })).toBeVisible();
    const seededCourse = page.getByRole("link", { name: "E2E Riverside Loop", exact: true });
    await expect(seededCourse).toBeVisible();
    await seededCourse.click();
    await expect(page.getByRole("heading", { name: "E2E Riverside Loop", exact: true })).toBeVisible();
    await expect(page.getByText("Elevation profile", { exact: true })).toBeVisible();
    await expect(page.locator(".course-elevation-coverage-notice")).toHaveCount(0);
    await expect(page.locator(".course-map-frame .leaflet-container")).toBeVisible();
    await expect(page.getByRole("link", { name: "GPX", exact: true })).toHaveAttribute("href", "/api/courses/00000000-0000-4000-8000-000000000180/gpx");

    const activityCourseName = `E2E ${testInfo.project.name} Activity Course`;
    await ensureActivityImported(page, testInfo.project.name, mobile);
    await visibleActivityLink(page, activityName(testInfo.project.name), mobile).click();
    await page.getByRole("button", { name: "Activity actions" }).click();
    await page.getByRole("menuitem", { name: "Save as course" }).click();
    const saveDialog = page.getByRole("dialog", { name: "Save as course" });
    await saveDialog.getByLabel("Name").fill(activityCourseName);
    await Promise.all([
      page.waitForResponse((response) => response.url().includes("/course") && response.request().method() === "POST" && response.status() === 201),
      saveDialog.getByRole("button", { name: "Save course" }).click()
    ]);
    await expect(page.getByRole("heading", { name: activityCourseName, exact: true })).toBeVisible();

    await navigateTo(page, "Courses", mobile);
    await expect(page.getByRole("link", { name: activityCourseName, exact: true })).toBeVisible();
    await page.getByRole("link", { name: "Upload GPX", exact: true }).click();
    const routeName = `E2E ${testInfo.project.name} GPX Course`;
    const latitude = mobile ? 53.39 : 53.38;
    const gpx = `<?xml version="1.0" encoding="UTF-8"?>
      <gpx version="1.1" creator="Runnarr E2E">
        <trk><name>${routeName}</name><type>Run</type><trkseg>
          <trkpt lat="${latitude.toFixed(4)}" lon="-6.3100"><ele>20</ele></trkpt>
          <trkpt lat="${(latitude + 0.005).toFixed(4)}" lon="-6.3000"><ele>36</ele></trkpt>
          <trkpt lat="${(latitude + 0.009).toFixed(4)}" lon="-6.2920"><ele>27</ele></trkpt>
        </trkseg></trk>
      </gpx>`;
    await page.locator('input[type="file"][accept*=".gpx"]').setInputFiles({
      name: `${projectSlug(testInfo.project.name)}-course.gpx`,
      mimeType: "application/gpx+xml",
      buffer: Buffer.from(gpx)
    });
    await expect(page.getByText(routeName, { exact: true })).toBeVisible();
    await expect(page.locator(".course-import-review .leaflet-container")).toBeVisible();
    await expect(page.getByText("Elevation profile", { exact: true })).toBeVisible();
    const details = page.locator(".course-import-fields");
    await details.getByLabel("Notes").fill("Imported through the reviewed GPX flow.");
    await Promise.all([
      page.waitForResponse((response) => response.url().endsWith("/api/course-imports/commit") && response.request().method() === "POST" && response.status() === 201),
      page.getByRole("button", { name: "Import 1 course", exact: true }).click()
    ]);
    await expect(page.getByRole("heading", { name: "Import complete", exact: true })).toBeVisible();
    await page.getByRole("link", { name: new RegExp(routeName) }).click();
    await expect(page.getByRole("heading", { name: routeName, exact: true })).toBeVisible();
    await expect(page.getByText("Imported through the reviewed GPX flow.", { exact: true })).toBeVisible();

    await navigateTo(page, "Courses", mobile);
    await page.getByRole("link", { name: "New course", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Plan a course", exact: true })).toBeVisible();
    await expect(page.getByText("Routing is not enabled.", { exact: true })).toBeVisible();
    const plannerMap = page.locator(".course-planner-map .leaflet-container");
    const waypointRows = page.locator(".course-waypoint-list li");
    await expect(waypointRows).toHaveCount(1);
    await expect(waypointRows.first().locator("small")).toHaveText(`${latitude.toFixed(5)}, -6.31000`);
    await expect(page.getByText("Starting location", { exact: true })).toHaveCount(0);
    await plannerMap.click({ position: { x: 90, y: 110 } });
    await plannerMap.click({ position: { x: mobile ? 220 : 330, y: 230 } });
    if (mobile) await plannerMap.click({ position: { x: 140, y: 300 } });
    await expect(waypointRows).toHaveCount(mobile ? 4 : 3);
    const startCoordinates = await waypointRows.first().locator("small").innerText();
    const backToStart = page.getByRole("button", { name: "Back to start", exact: true });
    await expect(backToStart).toBeEnabled();
    await backToStart.click();
    await expect(waypointRows).toHaveCount(mobile ? 5 : 4);
    await expect(waypointRows.last().locator("small")).toHaveText(startCoordinates);
    await expect(backToStart).toBeDisabled();
    await expect(page.getByText("Elevation profile", { exact: true })).toBeVisible();
    const plannerMetrics = page.locator(".course-planner-elevation-metrics .metric");
    await expect(plannerMetrics).toHaveCount(3);
    await expect(plannerMetrics.first()).toContainText("Distance");
    await expect(page.getByText("Elevation covers 0% of this route", { exact: false })).toBeVisible();
    await page.context().grantPermissions(["geolocation"], { origin: new URL(page.url()).origin });
    await page.context().setGeolocation({ latitude: 53.2707, longitude: -9.0568, accuracy: 12 });
    await page.getByRole("button", { name: "Current location", exact: true }).click();
    const locationMarker = page.locator(".course-planner-map .course-location-marker-icon");
    await expect(locationMarker).toBeVisible();
    await expect.poll(async () => {
      const mapBounds = await plannerMap.boundingBox();
      const markerBounds = await locationMarker.boundingBox();
      if (!mapBounds || !markerBounds) return Number.POSITIVE_INFINITY;
      const horizontalOffset = Math.abs(markerBounds.x + markerBounds.width / 2 - (mapBounds.x + mapBounds.width / 2));
      const verticalOffset = Math.abs(markerBounds.y + markerBounds.height / 2 - (mapBounds.y + mapBounds.height / 2));
      return Math.max(horizontalOffset, verticalOffset);
    }).toBeLessThan(8);
    const plannedName = `E2E ${testInfo.project.name} Planned Course`;
    await page.locator(".course-planner-sidebar").getByLabel("Name").fill(plannedName);
    page.once("dialog", (dialog) => void dialog.accept());
    await Promise.all([
      page.waitForResponse((response) => response.url().endsWith("/api/courses") && response.request().method() === "POST" && response.status() === 201),
      page.getByRole("button", { name: "Save course", exact: true }).click()
    ]);
    await expect(page.getByRole("heading", { name: plannedName, exact: true })).toBeVisible();
    await expect(page.getByText(/direct leg/).first()).toBeVisible();
    if (mobile) await expectNoHorizontalOverflow(page);
  });

  test("expands the course planner map while selecting waypoints", { tag: "@visual-course-planner-fullscreen" }, async ({ page }, testInfo) => {
    const mobile = isMobileProject(testInfo.project.name);
    await login(page, mobile);
    await navigateTo(page, "Courses", mobile);
    await page.getByRole("link", { name: "New course", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Plan a course", exact: true })).toBeVisible();

    const plannerMap = page.locator(".course-planner-map .leaflet-container");
    const waypointRows = page.locator(".course-waypoint-list li");
    await expect(waypointRows).toHaveCount(1);
    await page.getByRole("button", { name: "Enter fullscreen map", exact: true }).click();
    const fullscreenPanel = page.getByRole("region", { name: "Course route map" });
    await expect(page.getByRole("button", { name: "Exit fullscreen map", exact: true })).toBeVisible();
    await expect(fullscreenPanel).toHaveClass(/course-planner-map-fullscreen/);
    const fullscreenBounds = await fullscreenPanel.boundingBox();
    const viewport = page.viewportSize();
    expect(fullscreenBounds).not.toBeNull();
    expect(viewport).not.toBeNull();
    expect(fullscreenBounds!.width).toBeGreaterThanOrEqual(viewport!.width - 1);
    expect(fullscreenBounds!.height).toBeGreaterThanOrEqual(viewport!.height - 1);

    await plannerMap.click({ position: { x: 90, y: 110 } });
    await expect(waypointRows).toHaveCount(2);
    await page.keyboard.press("Escape");
    await expect(page.getByRole("button", { name: "Enter fullscreen map", exact: true })).toBeVisible();
    await expect(fullscreenPanel).not.toHaveClass(/course-planner-map-fullscreen/);
    if (mobile) await expectNoHorizontalOverflow(page);
  });

  test("excludes a matched planned run from all candidate windows until unmatch", { tag: "@visual-match-candidates" }, async ({ page }, testInfo) => {
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

  test("exits support view to the dashboard from an activity", { tag: "@visual-support-exit" }, async ({ page }, testInfo) => {
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

  test("pins an activity photo to a map location", { tag: "@visual-activity-media" }, async ({ page }, testInfo) => {
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

  test("covers calendar, health, gear, tools, and settings", { tag: "@visual-app-settings" }, async ({ page }, testInfo) => {
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
    const visibleCalendarLists = page.locator(".calendar-day-list:visible");
    await expect(visibleCalendarLists.getByRole("link", { name: "E2E Calendar Matched Run", exact: true })).toBeVisible();
    await expect(visibleCalendarLists.getByRole("link", { name: "E2E Calendar Planned Run", exact: true })).toHaveCount(0);
    await expect(page.locator(".calendar-day-match-meta:visible").filter({ hasText: "Matched plan: E2E Calendar Planned Run · Planned for" })).toBeVisible();

    const todayDayLink = page.locator(".calendar-day-link:visible").filter({ hasText: String(new Date().getDate()) }).first();
    await expect(todayDayLink).toBeVisible();
    await todayDayLink.click();
    await expect(page.getByRole("heading", { name: "Day view" })).toBeVisible();
    await expect(page.getByText("Daily Garmin metrics", { exact: true })).toBeVisible();
    await expect(page.locator(".metric-grid strong").filter({ hasText: "12,450" })).toBeVisible();
    await expect(page.getByRole("link", { name: "E2E Pool Swim", exact: true })).toBeVisible();
    await expect(page.getByRole("link", { name: "E2E Calendar Matched Run", exact: true })).toBeVisible();
    await expect(page.getByRole("link", { name: "E2E Calendar Planned Run", exact: true })).toHaveCount(0);
    await expect(page.locator(".calendar-day-match-meta").filter({ hasText: "Matched plan: E2E Calendar Planned Run · Planned for" })).toBeVisible();
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

  test("keeps navigation and key controls usable on mobile", { tag: "@visual-mobile-navigation" }, async ({ page }, testInfo) => {
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
    await expect(strengthSummary.getByText("Elevation", { exact: true })).toHaveCount(0);
    await expect(strengthSummary.getByText("Moving Time", { exact: true })).toBeVisible();

    await navigateTo(page, "Settings", true);
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Upload", exact: true })).toBeVisible();
    await expect(page.locator(".sidebar")).toBeHidden();
    await expectNoHorizontalOverflow(page);
  });

  test("creates, inspects, and opens planned workouts from the calendar", { tag: "@visual-planned-workouts" }, async ({ page }, testInfo) => {
    const mobile = isMobileProject(testInfo.project.name);
    await login(page, mobile);

    await page.goto("/activities");
    await visibleActivityLink(page, "E2E Calendar Matched Run", mobile).click();
    await expect(page.getByRole("heading", { name: "E2E Calendar Matched Run" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Workout", exact: true })).toHaveAttribute("href", "/workouts/00000000-0000-4000-8000-000000000173");
    const trainingSheetLink = page.getByRole("link", { name: "Training sheet", exact: true });
    await expect(trainingSheetLink).toHaveAttribute("href", "https://docs.google.com/spreadsheets/d/e2e-workbook/edit#gid=e2e-sheet");
    await expect(trainingSheetLink).toHaveAttribute("target", "_blank");
    await expect(trainingSheetLink).toHaveAttribute("rel", "noreferrer");

    await navigateTo(page, "Settings", mobile);
    if (await page.getByText("Connected as Offline Garmin Testbed", { exact: true }).count() === 0) {
      await page.getByPlaceholder("Garmin email").fill("offline@example.test");
      await page.getByPlaceholder("Garmin password").fill("offline-testbed");
      await page.getByRole("button", { name: "Connect", exact: true }).click();
      await expect(page.getByText("Connected as Offline Garmin Testbed", { exact: true })).toBeVisible();
    }
    const workoutSettings = page.locator(".workout-settings-panel");
    const enableScheduling = workoutSettings.getByRole("checkbox", { name: "Enable Garmin workout scheduling" });
    if (!(await enableScheduling.isChecked())) {
      await enableScheduling.check();
    }
    await workoutSettings.getByLabel("Workout timezone").fill("UTC");
    await Promise.all([
      page.waitForResponse((response) => response.url().endsWith("/api/config/workouts") && response.request().method() === "PATCH" && response.ok()),
      workoutSettings.getByRole("button", { name: "Save workout settings", exact: true }).click()
    ]);
    await expect.poll(async () => page.evaluate(async () => {
      const response = await fetch("/api/sync-jobs");
      const payload = await response.json();
      return payload.jobs?.find((job: { provider: string; kind: string }) => job.provider === "garmin" && job.kind === "workouts")?.status;
    }), { timeout: 15_000 }).toBe("completed");

    await navigateTo(page, "Workouts", mobile);
    await expect(page.getByRole("heading", { name: "Workouts" })).toBeVisible();
    await expect(page.getByRole("link", { name: /E2E Manual Surges/ })).toBeVisible();
    const scheduledWorkout = page.locator(".workout-list-row").filter({ hasText: "E2E Planned Speed Work" });
    await expect(scheduledWorkout.getByText("Scheduled", { exact: true })).toBeVisible();
    await scheduledWorkout.click();
    await expect(page.locator(".workout-garmin-summary")).toContainText("scheduled");
    await expect(page.getByRole("link", { name: /Open in Garmin/ })).toHaveCount(0);

    await page.getByRole("link", { name: "Back", exact: true }).click();
    await page.getByRole("link", { name: /E2E Manual Surges/ }).click();
    await expect(page.getByRole("heading", { name: "E2E Manual Surges" })).toBeVisible();
    await expect(page.getByLabel("Prescription")).toHaveValue("47mins with surges");
    const seededSurges = page.locator(".workout-step.repeat");
    await expect(seededSurges.getByText("9× repeat", { exact: true })).toBeVisible();
    await expect(seededSurges.locator(".workout-step.work").nth(0)).toContainText("4:30");
    await expect(seededSurges.locator(".workout-step.work").nth(1)).toContainText("0:30");
    await expect(page.locator(".workout-step-list > .workout-step.work")).toContainText("2:00");

    await page.getByRole("link", { name: "Back", exact: true }).click();
    await page.getByRole("link", { name: "New workout", exact: true }).click();
    const createdName = `E2E ${testInfo.project.name} Surges`;
    const scheduled = new Date();
    scheduled.setDate(scheduled.getDate() + 40);
    const scheduledDate = scheduled.toISOString().slice(0, 10);
    await page.getByLabel("Name").fill(createdName);
    await page.getByLabel("Date").fill(scheduledDate);
    await page.getByLabel("Prescription").fill("47mins with surges");
    await page.getByRole("button", { name: "Preview prescription", exact: true }).click();
    const parsedSurges = page.locator(".workout-step.repeat");
    await expect(parsedSurges.getByText("9× repeat", { exact: true })).toBeVisible();
    await expect(parsedSurges.locator(".workout-step.work").nth(0)).toContainText("4:30");
    await expect(parsedSurges.locator(".workout-step.work").nth(1)).toContainText("0:30");
    await expect(page.locator(".workout-step-list > .workout-step.work")).toContainText("2:00");
    await expect(page.locator(".workout-step.warmup, .workout-step.recovery, .workout-step.cooldown")).toHaveCount(0);
    await page.getByRole("button", { name: "Save", exact: true }).click();
    await expect(page.getByRole("heading", { name: createdName })).toBeVisible();

    const seededDate = new Date();
    seededDate.setDate(seededDate.getDate() + 40);
    const seededDateText = seededDate.toISOString().slice(0, 10);
    await page.goto(`/calendar/day/${seededDateText}`);
    const seededWorkoutLink = page.getByRole("link", { name: "E2E Manual Surges", exact: true });
    await expect(seededWorkoutLink).toHaveAttribute("href", "/workouts/00000000-0000-4000-8000-000000000172");
    await seededWorkoutLink.click();
    await expect(page.getByRole("heading", { name: "E2E Manual Surges" })).toBeVisible();
    await expectNoHorizontalOverflow(page);
  });

  test("uses notification inbox, links, and category settings", { tag: "@visual-notifications" }, async ({ page }, testInfo) => {
    const mobile = isMobileProject(testInfo.project.name);
    const notificationID = "00000000-0000-4000-8000-000000000174";
    const timestamp = "2026-07-30T12:00:00Z";
    const notification = {
      id: notificationID,
      category: "workout_changes",
      kind: "push_test",
      severity: "success",
      title: "Runnarr notifications are working",
      body: "This event opens notification settings.",
      actionPath: "/settings?section=notifications",
      createdAt: timestamp,
      lastEventAt: timestamp,
      eventCount: 1
    };
    let notificationRead = false;
    let notificationExists = true;

    await page.route(/\/api\/notifications(?:\/[^?]+)?(?:\?.*)?$/, async (route) => {
      const request = route.request();
      const url = new URL(request.url());
      if (request.method() === "PATCH") {
        notificationRead = JSON.parse(request.postData() ?? "{}").read === true;
        await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ updated: true }) });
        return;
      }
      if (request.method() === "POST" && url.pathname === "/api/notifications/read-all") {
        notificationRead = true;
        await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ updated: true }) });
        return;
      }
      if (request.method() === "DELETE" && url.pathname === "/api/notifications") {
        if (url.searchParams.get("scope") === "all" || notificationRead) notificationExists = false;
        await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ deleted: true }) });
        return;
      }
      if (url.pathname === `/api/notifications/${notificationID}`) {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            ...notification,
            ...(notificationRead ? { readAt: timestamp } : {}),
            events: [{
              id: "00000000-0000-4000-8000-000000000175",
              category: notification.category,
              kind: notification.kind,
              severity: notification.severity,
              title: notification.title,
              body: notification.body,
              actionPath: notification.actionPath,
              createdAt: timestamp
            }]
          })
        });
        return;
      }
      if (url.searchParams.get("limit") === "5") {
        expect(url.searchParams.get("unread")).toBe("true");
      }
      const visibleNotification = {
        ...notification,
        ...(notificationRead ? { readAt: timestamp } : {})
      };
      const includeNotification = notificationExists && (!url.searchParams.has("unread") || !notificationRead);
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          notifications: includeNotification ? [visibleNotification] : [],
          unreadCount: notificationExists && !notificationRead ? 1 : 0
        })
      });
    });

    await login(page, mobile);
    await page.getByRole("button", { name: "1 unread notifications", exact: true }).click();
    const popover = page.getByRole("dialog", { name: "Recent notifications" });
    await expect(popover.getByText(notification.title, { exact: true })).toBeVisible();
    await expect(popover.locator(".notification-severity.success")).toBeVisible();
    await expect(popover.locator(".notification-dot")).toHaveCount(0);
    await popover.getByRole("button").filter({ hasText: notification.title }).click();
    await expect(page).toHaveURL(/\/settings\?section=notifications$/);
    const settings = page.locator("#notifications");
    await expect(settings).toBeVisible();

    const activityMatching = settings.getByLabel(/Activity matching/);
    await Promise.all([
      page.waitForResponse((response) => response.url().endsWith("/api/notification-settings") && response.request().method() === "PATCH" && response.ok()),
      activityMatching.selectOption("off")
    ]);
    await expect(activityMatching).toHaveValue("off");
    await Promise.all([
      page.waitForResponse((response) => response.url().endsWith("/api/notification-settings") && response.request().method() === "PATCH" && response.ok()),
      activityMatching.selectOption("in_app")
    ]);
    await expect(activityMatching).toHaveValue("in_app");

    await page.goto("/notifications");
    await expect(page.getByRole("heading", { name: "Notifications" })).toBeVisible();
    await expect(page.locator(".notification-card .notification-severity.success")).toBeVisible();
    const markAllRead = page.getByRole("button", { name: "Mark all read", exact: true });
    await expect(markAllRead).toBeDisabled();
    await page.getByText(notification.title, { exact: true }).click();
    await expect(page.locator(".notification-timeline-event").getByText(notification.body, { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Mark unread", exact: true }).click();
    await expect(markAllRead).toBeEnabled();
    await markAllRead.click();
    await expect(page.getByRole("status")).toHaveText("All notifications marked as read.");
    page.once("dialog", (dialog) => dialog.accept());
    await page.getByRole("button", { name: "Clear read", exact: true }).click();
    await expect(page.getByRole("status")).toHaveText("Read notifications cleared.");
    await expect(page.getByText(notification.title, { exact: true })).toHaveCount(0);
    await expect(markAllRead).toBeDisabled();
    await expectNoHorizontalOverflow(page);
  });
});
