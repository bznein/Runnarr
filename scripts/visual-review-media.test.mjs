import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  copyFileSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync
} from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { collectMediaPlans, sanitizeMedia } from "./visual-review-media.mjs";

const profile = {
  label: "visual:desktop:auth",
  id: "auth",
  tag: "@visual-auth",
  viewport: "desktop",
  project: "chromium"
};

function makeVideo(filePath) {
  const result = spawnSync("ffmpeg", [
    "-nostdin", "-v", "error", "-y",
    "-f", "lavfi", "-i", "color=c=blue:s=800x450:r=10:d=1",
    "-c:v", "libvpx", "-threads", "1", "-an", filePath
  ], { encoding: "utf8" });
  assert.equal(result.status, 0, result.stderr);
}

function report(videoPath, attachments = 1) {
  return {
    suites: [{
      title: "journey.spec.ts",
      specs: [{
        tags: [profile.tag.slice(1)],
        title: "logs in",
        tests: [{
          projectName: profile.project,
          results: [{
            attachments: Array.from({ length: attachments }, () => ({
              name: "video",
              path: videoPath,
              contentType: "video/webm"
            }))
          }]
        }]
      }]
    }]
  };
}

function fixture() {
  const root = mkdtempSync(path.join(os.tmpdir(), "runnarr-visual-media-test."));
  const source = path.join(root, "source.webm");
  makeVideo(source);
  for (const [revision, sha] of [["before", "a".repeat(40)], ["after", "b".repeat(40)]]) {
    const artifact = path.join(root, `runnarr-visual-${revision}-pr-254-${sha}`);
    const resultDirectory = path.join(artifact, "test-results", "auth");
    mkdirSync(resultDirectory, { recursive: true });
    copyFileSync(source, path.join(resultDirectory, "video.webm"));
    writeFileSync(
      path.join(artifact, "playwright-report.json"),
      JSON.stringify(report("/home/runner/work/test-results/auth/video.webm"))
    );
  }
  return root;
}

test("collects one bounded video per profile and revision", () => {
  const root = fixture();
  try {
    const plans = collectMediaPlans(root, [profile]);
    assert.equal(plans.length, 2);
    assert.deepEqual(plans.map((item) => item.revision), ["after", "before"]);
    assert.ok(plans.every((item) => item.pullNumber === 254));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("sanitizes WebM recordings into MP4 videos and PNG posters", () => {
  const root = fixture();
  const output = path.join(root, "output");
  try {
    const manifest = sanitizeMedia({ artifactDirectory: root, outputDirectory: output, profiles: [profile] });
    assert.equal(manifest.status, "success");
    assert.equal(manifest.entries.length, 2);
    for (const entry of manifest.entries) {
      const mp4 = readFileSync(path.join(output, entry.video));
      const png = readFileSync(path.join(output, entry.poster));
      assert.equal(mp4.subarray(4, 8).toString("ascii"), "ftyp");
      assert.equal(png.subarray(0, 8).toString("hex"), "89504e470d0a1a0a");
    }
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("rejects duplicate video attachments", () => {
  const root = fixture();
  try {
    const afterReport = path.join(root, `runnarr-visual-after-pr-254-${"b".repeat(40)}`, "playwright-report.json");
    writeFileSync(afterReport, JSON.stringify(report("/home/runner/work/test-results/auth/video.webm", 2)));
    assert.throws(() => collectMediaPlans(root, [profile]), /exactly one video; found 2/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("rejects attachment paths that escape test-results", () => {
  const root = fixture();
  try {
    const afterReport = path.join(root, `runnarr-visual-after-pr-254-${"b".repeat(40)}`, "playwright-report.json");
    writeFileSync(afterReport, JSON.stringify(report("/home/runner/work/test-results/../source.webm")));
    assert.throws(() => collectMediaPlans(root, [profile]), /Unsafe video attachment path/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
