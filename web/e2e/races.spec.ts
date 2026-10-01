import {
  expect,
  test,
  type Page,
  type APIRequestContext,
} from "@playwright/test";
import { readFile } from "node:fs/promises";
import type { Race, RaceInput } from "../src/races/types";
const username = process.env.RUNNARR_E2E_USERNAME ?? "e2e-admin";
const password = process.env.RUNNARR_E2E_PASSWORD ?? "e2e-password-123";
const today =
  process.env.RUNNARR_E2E_FIXTURE_DATE ?? new Date().toISOString().slice(0, 10);
const fixture = "00000000-0000-4000-8000-000000005110";
const future = "00000000-0000-4000-8000-000000005111";
const suggestion = "00000000-0000-4000-8000-000000005103";
function input(r: Race): RaceInput {
  const {
    id: _id,
    createdAt: _created,
    updatedAt: _updated,
    courseSnapshot: _course,
    ...draft
  } = r;
  return draft;
}
const draft = (name: string): RaceInput => ({
  revision: 0,
  name,
  timezone: "Europe/Dublin",
  status: "planned",
  discipline: "road",
  kind: "race",
  raceOnly: false,
  goals: [],
  checklist: [],
  result: { confirmed: false, excluded: false, checkpoints: [] },
});
function authorized(api: APIRequestContext): APIRequestContext {
  return new Proxy(api, {
    get(target, key) {
      if (["post", "put", "patch", "delete"].includes(String(key)))
        return async (url: string, options: Record<string, unknown> = {}) => {
          const session = await (await target.get("/api/session")).json();
          return target[key as "post"](url, {
            ...options,
            headers: {
              "X-CSRF-Token": session.csrfToken ?? "",
              ...((options.headers as Record<string, string>) ?? {}),
            },
          });
        };
      const value = Reflect.get(target, key);
      return typeof value === "function" ? value.bind(target) : value;
    },
  });
}
async function login(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Username").fill(username);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Log in", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
}
async function assertNoOverflow(page: Page) {
  const sizes = await page.evaluate(() => [
    document.documentElement.scrollWidth,
    document.documentElement.clientWidth,
  ]);
  expect(sizes[0]).toBeLessThanOrEqual(sizes[1]);
}
async function race(request: APIRequestContext, id: string): Promise<Race> {
  const response = await request.get(`/api/races/${id}`);
  expect(response.ok(), await response.text()).toBeTruthy();
  return (await response.json()).race;
}

test("race planning, calendar, templates and activity review @visual-races-planning", async ({
  page,
}, info) => {
  await login(page);
  if (info.project.name === "mobile-chromium")
    await page.getByRole("button", { name: "More", exact: true }).click();
  await page.getByRole("link", { name: "Races", exact: true }).click();
  await expect(
    page.getByRole("link", { name: "E2E Trail ultra wishlist", exact: true }),
  ).toBeVisible();
  await page.getByRole("link", { name: "New race", exact: true }).click();
  const name = `E2E ${info.project.name} planned race`;
  await page.getByLabel("Race name", { exact: true }).fill(name);
  await page.getByLabel("Race date", { exact: true }).fill(today);
  await page.getByLabel("Distance preset").selectOption("10000");
  await page
    .getByLabel("Copy checklist template")
    .selectOption("00000000-0000-4000-8000-000000005101");
  await page
    .getByLabel("Recurring event")
    .selectOption("00000000-0000-4000-8000-000000005100");
  await page.getByRole("button", { name: "Add goal", exact: true }).click();
  await page.getByLabel("Goal 1 time (optional)").fill("40:00");
  await assertNoOverflow(page);
  await page.getByRole("button", { name: "Save race", exact: true }).click();
  await expect(page.getByRole("heading", { name, exact: true })).toBeVisible();
  const id = page.url().split("/").pop()!;
  await page.getByLabel("Pack race shoes", { exact: true }).check();
  await expect
    .poll(async () => (await race(page.request, id)).checklist[1].done)
    .toBe(true);
  await page.screenshot({
    path: info.outputPath("race-planning.png"),
    fullPage: true,
  });
  await page.goto(`/calendar/day/${today}`);
  await expect(
    page.getByRole("link", { name: new RegExp(name) }),
  ).toBeVisible();
  await page.goto(`/races/${id}`);
  await page
    .getByLabel("Find recorded activity")
    .fill("E2E Parkrun race suggestion");
  await page
    .getByRole("button", { name: "Confirm activity link", exact: true })
    .click();
  await expect(
    page.getByRole("link", {
      name: "E2E Parkrun race suggestion",
      exact: true,
    }),
  ).toBeVisible();
  await expect
    .poll(async () => (await race(page.request, id)).activityId)
    .toBe(suggestion);
  const linked = await race(page.request, id);
  await authorized(page.request).put(`/api/races/${id}`, {
    data: { ...input(linked), activityId: undefined },
  });
  await page.goto("/races/review");
  await page
    .getByRole("button", { name: "Scan historical activities" })
    .click();
  await expect(page.getByText(/Historical race scan: completed/)).toBeVisible({
    timeout: 30000,
  });
  await assertNoOverflow(page);
  const latest = await race(page.request, id);
  expect(
    (
      await authorized(page.request).delete(
        `/api/races/${id}?revision=${latest.revision}`,
      )
    ).ok(),
  ).toBeTruthy();
});

test("race results, official halfway, performance and report exports @visual-races-analysis", async ({
  page,
}, info) => {
  await login(page);
  await page.goto(`/races/${fixture}`);
  const original = await race(page.request, fixture);
  expect(
    (
      await authorized(page.request).put(`/api/races/${fixture}`, {
        data: {
          ...input(original),
          raceOnly: true,
          result: { ...original.result, checkpoints: [], splitBasis: "" },
        },
      })
    ).ok(),
  ).toBeTruthy();
  await page.reload();
  await expect(
    page.getByText(/Estimated midpoint of the complete recorded distance/),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "12-week training build-up" }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Edit race", exact: true }).click();
  await page
    .getByLabel("Official checkpoint timing basis")
    .selectOption("chip");
  await page.getByRole("button", { name: "Add halfway checkpoint" }).click();
  await page.getByLabel("Checkpoint 1 cumulative time").fill("19:45");
  await page.getByRole("button", { name: "Save race", exact: true }).click();
  await expect(page.getByText(/Official midpoint checkpoint/)).toBeVisible();
  await page.screenshot({
    path: info.outputPath("race-analysis.png"),
    fullPage: true,
  });
  await assertNoOverflow(page);
  await page.getByRole("link", { name: "Performance", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Current equivalents" }),
  ).toBeVisible();
  await expect(
    page.getByRole("cell", { name: "PB", exact: false }).first(),
  ).toBeVisible();
  await page.goto(`/races/${fixture}/report`);
  await page
    .getByLabel("Race experience", { exact: true })
    .fill(`A controlled start and strong finish (${info.project.name}).`);
  await page.getByLabel("Training build-up", { exact: true }).check();
  await page.getByRole("button", { name: "Save report", exact: true }).click();
  await expect(page.getByRole("status")).toHaveText("Report saved");
  const preview = page.getByRole("article", { name: "Report preview" });
  await expect(preview).toContainText("A controlled start and strong finish");
  await expect(preview).not.toContainText("PRIVATE fixture hotel reference");
  const pending = page.waitForEvent("download");
  await page.getByRole("button", { name: "Download Markdown" }).click();
  const download = await pending;
  const file = info.outputPath("race-report.md");
  await download.saveAs(file);
  const markdown = await readFile(file, "utf8");
  expect(markdown).toContain("Official checkpoints (chip time)");
  expect(markdown).toContain("19:45");
  expect(markdown).not.toContain("PRIVATE");
  await assertNoOverflow(page);
  // Exercise the print button without opening a native dialog in headless Chrome.
  await page.evaluate(() => {
    window.print = () => {};
  });
  await page.getByRole("button", { name: "Print / save PDF" }).click();
  await page.emulateMedia({ media: "print" });
  await expect(page.locator(".race-report-controls")).toBeHidden();
  await expect(page.locator(".race-tabs")).toBeHidden();
  await expect(page.locator(".sidebar")).toBeHidden();
  await expect(preview).toBeVisible();
  if (info.project.name === "chromium") {
    const pdf = await page.pdf({
      path: info.outputPath("race-report.pdf"),
      format: "A4",
      printBackground: true,
    });
    expect(pdf.subarray(0, 4).toString()).toBe("%PDF");
    expect(pdf.length).toBeGreaterThan(1000);
  }
  await page.evaluate(() => window.dispatchEvent(new Event("afterprint")));
  await page.emulateMedia({ media: "screen" });
  await expect(page.locator(".race-report-controls")).toBeVisible();
});

test("race APIs preserve ownership, revisions, links, dismissals, and opt-in boundaries", async ({
  page,
  playwright,
}, info) => {
  await login(page);
  const api = authorized(page.request);
  const a = await race(api, fixture);
  expect(
    (
      await api.put(`/api/races/${fixture}`, {
        data: { ...input(a), revision: a.revision - 1 },
      })
    ).status(),
  ).toBe(409);
  const duplicate = await api.post("/api/races", {
    data: { ...draft("Duplicate activity"), activityId: a.activityId },
  });
  expect(duplicate.status()).toBe(409);
  const malformed = await api.post("/api/races", {
    data: {
      ...draft("Bad result"),
      date: today,
      result: { confirmed: true, chipTimeMs: 0 },
    },
  });
  expect(malformed.status()).toBe(400);
  const group = await api.post("/api/race-groups", {
    data: { name: `E2E ${info.project.name} independent group` },
  });
  expect(group.ok()).toBeTruthy();
  const g = await group.json();
  const template = await api.post("/api/race-checklists", {
    data: {
      name: `E2E ${info.project.name} template`,
      items: [{ label: "Check registration", done: true, daysBefore: 7 }],
    },
  });
  expect(template.ok()).toBeTruthy();
  const t = await template.json();
  expect(t.items[0].done).toBe(false);
  const created = await api.post("/api/races", {
    data: { ...draft("Independent old race"), groupId: g.id },
  });
  expect(created.ok()).toBeTruthy();
  const r = await created.json();
  expect(
    (await api.delete(`/api/race-groups/${g.id}?revision=${g.revision}`)).ok(),
  ).toBeTruthy();
  expect((await race(api, r.id)).groupId).toBeUndefined();
  const otherUsername = `races-other-${info.project.name}`;
  const otherPassword = "races-test-password-123";
  const userResponse = await api.post("/api/users", {
    data: {
      username: otherUsername,
      password: otherPassword,
      displayName: "Other race account",
      role: "user",
    },
  });
  expect(userResponse.ok()).toBeTruthy();
  const user = (await userResponse.json()).user;
  const other = authorized(
    await playwright.request.newContext({
      baseURL: new URL(page.url()).origin,
    }),
  );
  try {
    expect(
      (
        await other.post("/api/session/login", {
          data: { username: otherUsername, password: otherPassword },
        })
      ).ok(),
    ).toBeTruthy();
    expect((await other.get(`/api/races/${fixture}`)).status()).toBe(404);
    expect(
      (
        await other.put(`/api/races/${fixture}`, {
          data: { ...draft("Foreign race"), revision: a.revision },
        })
      ).status(),
    ).toBe(404);
    expect(
      (
        await other.post("/api/races", {
          data: { ...draft("Foreign activity"), activityId: a.activityId },
        })
      ).status(),
    ).toBe(400);
    expect(
      (
        await other.post(`/api/races/${fixture}/report/preview`, {
          data: {
            revision: 0,
            sections: ["facts"],
            training: "",
            preparation: "",
            experience: "",
            reflections: "",
          },
        })
      ).status(),
    ).toBe(404);
    expect((await other.post("/api/races/predictions/sync")).status()).toBe(
      400,
    );
  } finally {
    await other.dispose();
  }
  expect(
    (
      await api.post("/api/session/support", { data: { userId: user.id } })
    ).ok(),
  ).toBeTruthy();
  expect(
    (
      await api.post("/api/races", { data: draft("Forbidden support write") })
    ).status(),
  ).toBe(403);
  expect((await api.delete("/api/session/support")).ok()).toBeTruthy();
  const review = await api.get("/api/races/review");
  expect(review.ok()).toBeTruthy();
  const disabled = await api.post(`/api/races/${future}/forecast`);
  expect(disabled.ok()).toBeTruthy();
  expect((await disabled.json()).reason).toContain("Enable race weather");
  expect(
    (
      await api.delete(`/api/race-checklists/${t.id}?revision=${t.revision}`)
    ).ok(),
  ).toBeTruthy();
  const courseResponse = await api.get(
    "/api/courses/00000000-0000-4000-8000-000000000180",
  );
  const sourceCourse = await courseResponse.json();
  const copiedResponse = await api.post(
    `/api/courses/${sourceCourse.id}/duplicate`,
    {
      data: {
        revision: sourceCourse.revision,
        name: "Race snapshot source",
        notes: "",
      },
    },
  );
  expect(copiedResponse.ok(), await copiedResponse.text()).toBeTruthy();
  const copied = await copiedResponse.json();
  const planID = "00000000-0000-4000-8000-000000002684";
  const beforePlans = await (await api.get("/api/planned-activities")).json();
  const linkedResponse = await api.put(`/api/races/${r.id}`, {
    data: {
      ...input(await race(api, r.id)),
      plannedActivityId: planID,
      courseId: copied.id,
    },
  });
  expect(linkedResponse.ok(), await linkedResponse.text()).toBeTruthy();
  const linked = await linkedResponse.json();
  expect(linked.courseSnapshot.legs[0].encodedPolyline).toBeTruthy();
  const editedResponse = await api.patch(`/api/courses/${copied.id}/details`, {
    data: {
      revision: copied.revision,
      name: "Renamed course library entry",
      sportType: "Run",
      notes: "Updated later",
    },
  });
  expect(editedResponse.ok()).toBeTruthy();
  const edited = await editedResponse.json();
  expect((await race(api, r.id)).courseSnapshot?.name).toBe(
    "Race snapshot source",
  );
  expect(
    (
      await api.delete(`/api/courses/${copied.id}?revision=${edited.revision}`)
    ).ok(),
  ).toBeTruthy();
  const retained = await race(api, r.id);
  expect(retained.courseId).toBeUndefined();
  expect(retained.courseSnapshot?.legs[0].encodedPolyline).toBe(
    linked.courseSnapshot.legs[0].encodedPolyline,
  );
  const afterPlans = await (await api.get("/api/planned-activities")).json();
  expect(
    afterPlans.planned.find((p: { id: string }) => p.id === planID),
  ).toEqual(beforePlans.planned.find((p: { id: string }) => p.id === planID));
  expect(
    (await api.delete(`/api/races/${r.id}?revision=${retained.revision}`)).ok(),
  ).toBeTruthy();
});

test("Garmin predictions sync through the offline bridge and preserve retrieval provenance", async ({
  page,
}) => {
  await login(page);
  const api = authorized(page.request);
  const settings = await (await api.get("/api/races/settings")).json();
  const connection = await api.post("/api/providers/garmin/connect", {
    data: { email: "races@example.test", password: "offline" },
  });
  expect(connection.ok(), await connection.text()).toBeTruthy();
  expect(
    (
      await api.put("/api/races/settings", {
        data: { ...settings, predictionsEnabled: true },
      })
    ).ok(),
  ).toBeTruthy();
  const sync = await api.post("/api/races/predictions/sync");
  expect(sync.status(), await sync.text()).toBe(202);
  const job = await sync.json();
  await expect
    .poll(
      async () => {
        const response = await api.get("/api/sync-jobs");
        const data = await response.json();
        return data.jobs.find((j: { id: string }) => j.id === job.jobId)
          ?.status;
      },
      { timeout: 30000 },
    )
    .toBe("completed");
  const performance = await (await api.get("/api/races/performance")).json();
  expect(performance.predictions.length).toBeGreaterThan(1000);
  expect(performance.predictions[0]).toHaveProperty("fetchedAt");
  const current = await (await api.get("/api/races/settings")).json();
  expect(current.predictionBackfilled).toBe(true);
  expect(
    (
      await api.put("/api/races/settings", {
        data: { ...current, predictionsEnabled: false },
      })
    ).ok(),
  ).toBeTruthy();
});
