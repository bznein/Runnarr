import { expect, test, type Page } from "@playwright/test";
import { readFile } from "node:fs/promises";

const username = process.env.RUNNARR_E2E_USERNAME ?? "e2e-admin";
const password = process.env.RUNNARR_E2E_PASSWORD ?? "e2e-password-123";
const basemap = `<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256"><rect width="256" height="256" fill="#e9ede7"/><path d="M0 55H256M0 140H256M65 0V256M185 0V256" stroke="#fff" stroke-width="8"/><path d="M0 205Q120 110 256 195" fill="none" stroke="#9bc4d0" stroke-width="14"/></svg>`;

async function openHeatmap(page: Page) {
  await page.route("https://tile.openstreetmap.org/**", (route) => route.fulfill({ contentType: "image/svg+xml", headers: { "access-control-allow-origin": "*" }, body: basemap }));
  await page.goto("/login");
  await page.getByLabel("Username").fill(username);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Log in", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
  if ((page.viewportSize()?.width ?? 1280) < 768) await page.getByRole("button", { name: "More", exact: true }).click();
  await page.getByRole("link", { name: "Heatmap", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Heatmap", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Download PNG", exact: true })).toBeEnabled({ timeout: 30_000 });
  await expect(page).toHaveURL(/[?&]zoom=/);
}

test("personal heatmap filters, appearance, fullscreen, and map/artwork PNGs @visual-heatmap", async ({ page }, info) => {
  await openHeatmap(page);
  const downloadButton = page.getByRole("button", { name: "Download PNG", exact: true });
  await page.getByRole("combobox", { name: "Appearance", exact: true }).selectOption("dark");
  await expect(page.getByRole("region", { name: "Personal activity heatmap" })).toHaveAttribute("data-appearance", "dark");
  await expect(downloadButton).toBeEnabled();
  await page.screenshot({ path: info.outputPath("heatmap-dark.png"), fullPage: true });
  const before = new URL(page.url());
  await page.getByRole("button", { name: "Last 12 months", exact: true }).click();
  await expect(downloadButton).toBeEnabled();
  expect(new URL(page.url()).searchParams.get("lat")).toBe(before.searchParams.get("lat"));
  expect(new URL(page.url()).searchParams.get("zoom")).toBe(before.searchParams.get("zoom"));
  await page.getByRole("button", { name: "Fullscreen", exact: true }).click();
  await expect(page.getByRole("button", { name: "Exit fullscreen" })).toBeVisible();
  await page.getByRole("button", { name: "Exit fullscreen" }).click();
  await expect(downloadButton).toBeEnabled();
  await downloadButton.click();
  const dialog = page.getByRole("dialog", { name: "Download heatmap" });
  await expect(dialog.getByAltText("Map export preview")).toBeVisible();
  for (const mode of ["map", "artwork"] as const) {
    if (mode === "artwork") await dialog.getByLabel("Artwork", { exact: true }).check();
    await expect(dialog.getByAltText(mode === "map" ? "Map export preview" : "Artwork export preview")).toBeVisible();
    const pending = page.waitForEvent("download");
    await dialog.getByRole("button", { name: "Save PNG", exact: true }).click();
    const download = await pending;
    const output = info.outputPath(`heatmap-${mode}.png`);
    await download.saveAs(output);
    const bytes = await readFile(output);
    expect(bytes.subarray(1, 4).toString()).toBe("PNG");
    expect(bytes.readUInt32BE(16)).toBeGreaterThan(500);
    expect(bytes.readUInt32BE(16)).toBeLessThanOrEqual(4096);
    expect(bytes.readUInt32BE(20)).toBeLessThanOrEqual(4096);
    expect(bytes.length).toBeGreaterThan(3000);
  }
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await page.getByRole("combobox", { name: "Appearance", exact: true }).selectOption("light");
  await expect(downloadButton).toBeEnabled();
  await page.screenshot({ path: info.outputPath("heatmap-light.png"), fullPage: true });
  await page.reload();
  await expect(page.getByRole("combobox", { name: "Appearance", exact: true })).toHaveValue("light");
  await expect(downloadButton).toBeEnabled();
  await page.getByLabel("Sports", { exact: true }).selectOption("Strength Training");
  await expect(page.getByText("These activities have no usable GPS routes.", { exact: false })).toBeVisible();
  await expect(downloadButton).toBeDisabled();
  const widths = await page.evaluate(() => [document.documentElement.scrollWidth, document.documentElement.clientWidth]);
  expect(widths[0]).toBeLessThanOrEqual(widths[1]);
});

test("heatmap empty/error states and export provider restriction", async ({ page }) => {
  await openHeatmap(page);
  await page.route("**/api/heatmap?**", (route) => route.fulfill({ status: 500, json: { error: "Heatmap temporarily unavailable" } }));
  await page.getByRole("button", { name: "This year", exact: true }).click();
  await expect(page.getByText("Heatmap temporarily unavailable")).toBeVisible({ timeout: 15_000 });
  await page.unroute("**/api/heatmap?**");
  await page.getByRole("button", { name: "Retry", exact: true }).first().click();
  await expect(page.getByRole("button", { name: "Download PNG", exact: true })).toBeEnabled();
  await page.getByRole("combobox", { name: "Appearance", exact: true }).selectOption("light");
  await expect(page.getByRole("button", { name: "Download PNG", exact: true })).toBeEnabled();
  await page.route("**/api/heatmap/tiles/**", (route) => route.fulfill({ status: 503, json: { error: "Temporary tile failure" } }));
  await page.getByRole("combobox", { name: "Appearance", exact: true }).selectOption("dark");
  await expect(page.getByText("Some routes could not be loaded.", { exact: false })).toBeVisible();
  await page.unroute("**/api/heatmap/tiles/**");
  await page.getByRole("button", { name: "Retry", exact: true }).click();
  await expect(page.getByRole("button", { name: "Download PNG", exact: true })).toBeEnabled();
  await page.route("**/api/heatmap/tiles/**", (route) => route.fulfill({ status: 503, json: { error: "Temporary export failure" } }));
  await page.getByRole("button", { name: "Download PNG", exact: true }).click();
  const retryDialog = page.getByRole("dialog", { name: "Download heatmap" });
  await expect(retryDialog.getByRole("alert")).toContainText("Could not load heatmap tiles");
  await page.unroute("**/api/heatmap/tiles/**");
  await retryDialog.getByRole("button", { name: "Retry export", exact: true }).click();
  await expect(retryDialog.getByAltText("Map export preview")).toBeVisible();
  await retryDialog.getByRole("button", { name: "Close", exact: true }).click();
  await page.evaluate(() => {
    CanvasRenderingContext2D.prototype.getImageData = () => { throw new DOMException("Tainted canvas", "SecurityError"); };
  });
  await page.getByRole("button", { name: "Download PNG", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Download heatmap" });
  await expect(dialog.getByRole("alert")).toContainText("does not allow image export");
  await dialog.getByLabel("Artwork", { exact: true }).check();
  await expect(dialog.getByAltText("Artwork export preview")).toBeVisible();
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await page.getByLabel("From", { exact: true }).fill("1900-01-01");
  await page.getByLabel("To", { exact: true }).fill("1900-01-02");
  await expect(page.getByText("No activities match these filters.", { exact: false })).toBeVisible();
});
