import assert from "node:assert/strict";
import test from "node:test";

import { catalogLabels, loadCatalog, resolveProfiles } from "./visual-review-profiles.mjs";

const catalog = loadCatalog();

test("catalog exposes unique profile labels", () => {
  const labels = catalogLabels(catalog);
  assert.equal(new Set(labels).size, labels.length);
  assert.ok(labels.includes("visual:desktop:activity-inspection"));
  assert.ok(labels.includes("visual:mobile:activity-inspection"));
  assert.ok(labels.includes("visual:mobile:mobile-navigation"));
  assert.ok(!labels.includes("visual:desktop:mobile-navigation"));
});

test("resolver ignores ordinary labels and selects exact projects", () => {
  assert.deepEqual(resolveProfiles(["type:enhancement"], catalog), []);
  assert.deepEqual(
    resolveProfiles([
      "visual:mobile:activity-inspection",
      "priority:medium",
      "visual:desktop:auth"
    ], catalog),
    [
      {
        label: "visual:desktop:auth",
        id: "auth",
        tag: "@visual-auth",
        viewport: "desktop",
        project: "chromium"
      },
      {
        label: "visual:mobile:activity-inspection",
        id: "activity-inspection",
        tag: "@visual-activity-inspection",
        viewport: "mobile",
        project: "mobile-chromium"
      }
    ]
  );
});

test("resolver rejects malformed, unsupported, duplicate, and excessive profiles", () => {
  assert.throws(() => resolveProfiles(["visual:desktop:missing"], catalog), /Unknown visual review scenario/);
  assert.throws(() => resolveProfiles(["visual:tablet:auth"], catalog), /Invalid visual review profile label/);
  assert.throws(() => resolveProfiles(["visual:desktop:mobile-navigation"], catalog), /does not support/);
  assert.throws(() => resolveProfiles(["visual:desktop:auth", "visual:desktop:auth"], catalog), /Duplicate/);
  assert.throws(
    () => resolveProfiles([
      "visual:desktop:auth",
      "visual:desktop:courses",
      "visual:mobile:notifications"
    ], catalog),
    /at most two/
  );
});
