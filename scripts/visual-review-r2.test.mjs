import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { presignGet, publishMedia } from "./visual-review-r2.mjs";

const fixedNow = new Date("2026-08-05T12:00:00.000Z");
const env = {
  R2_ACCOUNT_ID: "a".repeat(32),
  R2_ACCESS_KEY_ID: "AKIDEXAMPLE123456",
  R2_SECRET_ACCESS_KEY: "example-secret-key-for-tests",
  R2_BUCKET: "runnarr-visual-review"
};
const credentials = {
  account: env.R2_ACCOUNT_ID,
  accessKey: env.R2_ACCESS_KEY_ID,
  secret: env.R2_SECRET_ACCESS_KEY,
  bucket: env.R2_BUCKET
};
const profile = {
  label: "visual:desktop:auth",
  id: "auth",
  tag: "@visual-auth",
  viewport: "desktop",
  project: "chromium"
};

function fixture() {
  const root = mkdtempSync(path.join(os.tmpdir(), "runnarr-r2-test."));
  for (const revision of ["before", "after"]) {
    const stem = `${revision}--desktop--auth`;
    const mp4 = Buffer.alloc(16);
    mp4.write("ftyp", 4, "ascii");
    writeFileSync(path.join(root, `${stem}.mp4`), mp4);
    writeFileSync(
      path.join(root, `${stem}.png`),
      Buffer.from("89504e470d0a1a0a00000000", "hex")
    );
  }
  writeFileSync(path.join(root, "media-manifest.json"), JSON.stringify({
    version: 1,
    status: "success",
    entries: ["before", "after"].map((revision) => ({
      revision,
      pullNumber: 254,
      sha: (revision === "before" ? "a" : "b").repeat(40),
      profile,
      video: `${revision}--desktop--auth.mp4`,
      poster: `${revision}--desktop--auth.png`
    }))
  }));
  return root;
}

function lifecycleResponse() {
  return new Response(null, {
    status: 200,
    headers: {
      "x-amz-expiration": `expiry-date="${new Date(fixedNow.getTime() + 7 * 86400000).toUTCString()}", rule-id="visual"`
    }
  });
}

test("creates stable seven-day SigV4 GET links", () => {
  const link = presignGet({
    key: "visual/pr-254/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/after/desktop-auth.mp4",
    credentials,
    now: fixedNow
  });
  assert.match(link, /^https:\/\/a{32}\.r2\.cloudflarestorage\.com\/runnarr-visual-review\/visual\/pr-254\//);
  assert.match(link, /X-Amz-Expires=604800/);
  assert.match(link, /X-Amz-SignedHeaders=host/);
  assert.match(link, /X-Amz-Signature=[0-9a-f]{64}$/);
  assert.equal(link, presignGet({
    key: "visual/pr-254/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/after/desktop-auth.mp4",
    credentials,
    now: fixedNow
  }));
  assert.ok(!link.includes(env.R2_SECRET_ACCESS_KEY));
});

test("uploads only validated media and emits direct stream links", async () => {
  const root = fixture();
  const output = path.join(root, "links", "stream-links.json");
  const requests = [];
  const fetchImpl = async (url, options) => {
    requests.push({ url, options });
    return options.method === "HEAD" ? lifecycleResponse() : new Response(null, { status: 200 });
  };
  try {
    const manifest = await publishMedia({
      mediaDirectory: root,
      outputFile: output,
      env,
      fetchImpl,
      now: () => fixedNow
    });
    assert.equal(manifest.status, "success");
    assert.equal(manifest.entries.length, 2);
    assert.equal(requests.filter((request) => request.options.method === "PUT").length, 4);
    assert.equal(requests.filter((request) => request.options.method === "HEAD").length, 4);
    assert.ok(requests.every((request) => !request.url.includes(env.R2_SECRET_ACCESS_KEY)));
    assert.ok(manifest.entries.every((entry) => entry.videoUrl.includes("X-Amz-Signature=")));
    assert.equal(JSON.parse(readFileSync(output, "utf8")).status, "success");
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("rolls back and fails when lifecycle expiration is not verifiable", async () => {
  const root = fixture();
  const output = path.join(root, "stream-links.json");
  const methods = [];
  const fetchImpl = async (_url, options) => {
    methods.push(options.method);
    return new Response(null, { status: 200 });
  };
  try {
    await assert.rejects(
      publishMedia({ mediaDirectory: root, outputFile: output, env, fetchImpl, now: () => fixedNow }),
      /lifecycle expiration/
    );
    assert.deepEqual(methods, ["PUT", "HEAD", "DELETE"]);
    assert.equal(JSON.parse(readFileSync(output, "utf8")).status, "error");
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("rejects media that does not match its declared type before upload", async () => {
  const root = fixture();
  const output = path.join(root, "stream-links.json");
  writeFileSync(path.join(root, "after--desktop--auth.mp4"), Buffer.alloc(16));
  let requested = false;
  try {
    await assert.rejects(
      publishMedia({
        mediaDirectory: root,
        outputFile: output,
        env,
        fetchImpl: async () => {
          requested = true;
          return new Response(null, { status: 200 });
        },
        now: () => fixedNow
      }),
      /not an MP4/
    );
    assert.equal(requested, false);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
