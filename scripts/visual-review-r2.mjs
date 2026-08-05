#!/usr/bin/env node

import { createHash, createHmac } from "node:crypto";
import {
  lstatSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  statSync,
  writeFileSync
} from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const MAX_VIDEO_BYTES = 25 * 1024 * 1024;
const MAX_POSTER_BYTES = 1024 * 1024;
const MAX_ENTRIES = 4;
const LINK_SECONDS = 7 * 24 * 60 * 60;
const MIN_LIFECYCLE_MILLISECONDS = 6 * 24 * 60 * 60 * 1000;
const MAX_LIFECYCLE_MILLISECONDS = 8 * 24 * 60 * 60 * 1000;
const EMPTY_HASH = createHash("sha256").update("").digest("hex");
const PNG_MAGIC = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);

function argumentValue(name, argv = process.argv.slice(2)) {
  const index = argv.indexOf(name);
  if (index === -1 || index + 1 >= argv.length) return undefined;
  return argv[index + 1];
}

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

function hmac(key, value) {
  return createHmac("sha256", key).update(value).digest();
}

function encode(value) {
  return encodeURIComponent(value).replace(/[!'()*]/g, (character) =>
    `%${character.charCodeAt(0).toString(16).toUpperCase()}`
  );
}

function canonicalPath(bucket, key) {
  return `/${[bucket, ...key.split("/")].map(encode).join("/")}`;
}

function timestamp(date) {
  return date.toISOString().replace(/[:-]|\.\d{3}/g, "");
}

function normalizedHeader(value) {
  return String(value).trim().replace(/\s+/g, " ");
}

function canonicalHeaders(headers) {
  const pairs = Object.entries(headers)
    .map(([name, value]) => [name.toLowerCase(), normalizedHeader(value)])
    .sort(([left], [right]) => left < right ? -1 : left > right ? 1 : 0);
  return {
    text: `${pairs.map(([name, value]) => `${name}:${value}`).join("\n")}\n`,
    names: pairs.map(([name]) => name).join(";")
  };
}

function signingKey(secret, dateStamp) {
  const date = hmac(`AWS4${secret}`, dateStamp);
  const region = hmac(date, "auto");
  const service = hmac(region, "s3");
  return hmac(service, "aws4_request");
}

function signature({ secret, dateStamp, scope, canonicalRequest, amzDate }) {
  const stringToSign = [
    "AWS4-HMAC-SHA256",
    amzDate,
    scope,
    sha256(canonicalRequest)
  ].join("\n");
  return createHmac("sha256", signingKey(secret, dateStamp)).update(stringToSign).digest("hex");
}

function validateCredentials(env) {
  const credentials = {
    account: env.R2_ACCOUNT_ID,
    accessKey: env.R2_ACCESS_KEY_ID,
    secret: env.R2_SECRET_ACCESS_KEY,
    bucket: env.R2_BUCKET
  };
  if (!/^[0-9a-f]{32}$/.test(credentials.account ?? "")) {
    throw new Error("R2_ACCOUNT_ID must be a 32-character lowercase hexadecimal account ID.");
  }
  if (!/^[A-Za-z0-9]{16,128}$/.test(credentials.accessKey ?? "")) {
    throw new Error("R2_ACCESS_KEY_ID has an unexpected format.");
  }
  if (typeof credentials.secret !== "string" || credentials.secret.length < 16 || credentials.secret.length > 256) {
    throw new Error("R2_SECRET_ACCESS_KEY has an unexpected format.");
  }
  if (credentials.bucket !== "runnarr-visual-review") {
    throw new Error("R2_BUCKET must name the dedicated runnarr-visual-review bucket.");
  }
  return credentials;
}

function requestDetails({ method, key, body, contentType, credentials, now }) {
  const host = `${credentials.account}.r2.cloudflarestorage.com`;
  const uri = canonicalPath(credentials.bucket, key);
  const amzDate = timestamp(now);
  const dateStamp = amzDate.slice(0, 8);
  const payloadHash = body ? sha256(body) : EMPTY_HASH;
  const headers = {
    host,
    "x-amz-content-sha256": payloadHash,
    "x-amz-date": amzDate
  };
  if (contentType) {
    headers["cache-control"] = "private, max-age=0, no-store";
    headers["content-disposition"] = "inline";
    headers["content-type"] = contentType;
  }
  const canonical = canonicalHeaders(headers);
  const canonicalRequest = [
    method,
    uri,
    "",
    canonical.text,
    canonical.names,
    payloadHash
  ].join("\n");
  const scope = `${dateStamp}/auto/s3/aws4_request`;
  const signed = signature({
    secret: credentials.secret,
    dateStamp,
    scope,
    canonicalRequest,
    amzDate
  });
  const authorization = `AWS4-HMAC-SHA256 Credential=${credentials.accessKey}/${scope}, SignedHeaders=${canonical.names}, Signature=${signed}`;
  const fetchHeaders = { ...headers, authorization };
  delete fetchHeaders.host;
  return {
    url: `https://${host}${uri}`,
    options: {
      method,
      headers: fetchHeaders,
      ...(body ? { body } : {})
    }
  };
}

async function request({ method, key, body, contentType, credentials, now, fetchImpl }) {
  const details = requestDetails({ method, key, body, contentType, credentials, now });
  try {
    return await fetchImpl(details.url, details.options);
  } catch {
    throw new Error(`R2 ${method} request failed before receiving a response.`);
  }
}

export function presignGet({ key, credentials, now, expires = LINK_SECONDS }) {
  if (!Number.isInteger(expires) || expires < 1 || expires > LINK_SECONDS) {
    throw new Error("R2 links may be signed for at most seven days.");
  }
  const host = `${credentials.account}.r2.cloudflarestorage.com`;
  const uri = canonicalPath(credentials.bucket, key);
  const amzDate = timestamp(now);
  const dateStamp = amzDate.slice(0, 8);
  const scope = `${dateStamp}/auto/s3/aws4_request`;
  const query = {
    "X-Amz-Algorithm": "AWS4-HMAC-SHA256",
    "X-Amz-Content-Sha256": "UNSIGNED-PAYLOAD",
    "X-Amz-Credential": `${credentials.accessKey}/${scope}`,
    "X-Amz-Date": amzDate,
    "X-Amz-Expires": String(expires),
    "X-Amz-SignedHeaders": "host"
  };
  const canonicalQuery = Object.entries(query)
    .sort(([left], [right]) => left < right ? -1 : left > right ? 1 : 0)
    .map(([name, value]) => `${encode(name)}=${encode(value)}`)
    .join("&");
  const canonicalRequest = [
    "GET",
    uri,
    canonicalQuery,
    `host:${host}\n`,
    "host",
    "UNSIGNED-PAYLOAD"
  ].join("\n");
  const signed = signature({
    secret: credentials.secret,
    dateStamp,
    scope,
    canonicalRequest,
    amzDate
  });
  return `https://${host}${uri}?${canonicalQuery}&X-Amz-Signature=${signed}`;
}

function inside(parent, candidate) {
  const relative = path.relative(parent, candidate);
  return relative !== "" && !relative.startsWith("..") && !path.isAbsolute(relative);
}

function safeFile(root, name, maximumBytes, type) {
  if (typeof name !== "string" || path.basename(name) !== name) {
    throw new Error(`Unsafe ${type} filename in media manifest.`);
  }
  const filePath = path.join(root, name);
  if (!lstatSync(filePath).isFile()) throw new Error(`${name} is not a regular file.`);
  const resolvedRoot = realpathSync(root);
  const resolvedFile = realpathSync(filePath);
  if (!inside(resolvedRoot, resolvedFile)) throw new Error(`${name} escapes the media directory.`);
  const size = statSync(resolvedFile).size;
  if (size < 8 || size > maximumBytes) throw new Error(`${name} has an invalid size.`);
  const bytes = readFileSync(resolvedFile);
  if (type === "MP4" && (bytes.length < 12 || bytes.subarray(4, 8).toString("ascii") !== "ftyp")) {
    throw new Error(`${name} is not an MP4 file.`);
  }
  if (type === "PNG" && !bytes.subarray(0, PNG_MAGIC.length).equals(PNG_MAGIC)) {
    throw new Error(`${name} is not a PNG file.`);
  }
  return bytes;
}

function validateManifest(mediaDirectory) {
  const manifest = JSON.parse(readFileSync(path.join(mediaDirectory, "media-manifest.json"), "utf8"));
  if (manifest?.version !== 1 || manifest.status !== "success" || !Array.isArray(manifest.entries)) {
    throw new Error("Media sanitization did not produce a successful version 1 manifest.");
  }
  if (![2, 4].includes(manifest.entries.length) || manifest.entries.length > MAX_ENTRIES) {
    throw new Error("Media manifest must contain one or two before/after profile pairs.");
  }
  const entries = manifest.entries.map((entry) => {
    if (!entry || !["before", "after"].includes(entry.revision)) throw new Error("Invalid media revision.");
    if (!Number.isInteger(entry.pullNumber) || entry.pullNumber < 1) throw new Error("Invalid pull request number.");
    if (!/^[0-9a-f]{40}$/.test(entry.sha ?? "")) throw new Error("Invalid media commit SHA.");
    const profile = entry.profile;
    if (!profile || !/^visual:(desktop|mobile):[a-z0-9]+(?:-[a-z0-9]+)*$/.test(profile.label ?? "")) {
      throw new Error("Invalid media profile.");
    }
    if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(profile.id ?? "") || !["desktop", "mobile"].includes(profile.viewport)) {
      throw new Error("Invalid media profile identifier.");
    }
    if (
      profile.label !== `visual:${profile.viewport}:${profile.id}` ||
      profile.tag !== `@visual-${profile.id}` ||
      profile.project !== (profile.viewport === "mobile" ? "mobile-chromium" : "chromium")
    ) {
      throw new Error("Media profile fields are inconsistent.");
    }
    const stem = `${entry.revision}--${profile.viewport}--${profile.id}`;
    if (entry.video !== `${stem}.mp4` || entry.poster !== `${stem}.png`) {
      throw new Error("Media filenames do not match their manifest entry.");
    }
    return {
      revision: entry.revision,
      pullNumber: entry.pullNumber,
      sha: entry.sha,
      profile,
      video: entry.video,
      poster: entry.poster,
      videoBytes: safeFile(mediaDirectory, entry.video, MAX_VIDEO_BYTES, "MP4"),
      posterBytes: safeFile(mediaDirectory, entry.poster, MAX_POSTER_BYTES, "PNG")
    };
  });
  if (new Set(entries.map((entry) => entry.pullNumber)).size !== 1) {
    throw new Error("Media entries refer to different pull requests.");
  }
  const revisionShas = Object.fromEntries(["before", "after"].map(revision => [
    revision,
    new Set(entries.filter((entry) => entry.revision === revision).map((entry) => entry.sha))
  ]));
  if (revisionShas.before.size !== 1 || revisionShas.after.size !== 1) {
    throw new Error("Media entries have inconsistent revision commit SHAs.");
  }
  const pairs = new Map();
  for (const entry of entries) {
    const key = entry.profile.label;
    const revisions = pairs.get(key) ?? new Set();
    if (revisions.has(entry.revision)) throw new Error(`Duplicate ${entry.revision} media for ${key}.`);
    revisions.add(entry.revision);
    pairs.set(key, revisions);
  }
  if ([...pairs.values()].some((revisions) => revisions.size !== 2) || pairs.size * 2 !== entries.length) {
    throw new Error("Every visual profile needs exactly one before and one after recording.");
  }
  return { entries, afterSha: [...revisionShas.after][0] };
}

function expirationFrom(response, now) {
  const header = response.headers.get("x-amz-expiration") ?? "";
  const match = /(?:^|,)\s*expiry-date="([^"]+)"/.exec(header);
  const expiration = match ? new Date(match[1]) : undefined;
  if (!expiration || !Number.isFinite(expiration.getTime())) {
    throw new Error("R2 did not report a lifecycle expiration for an uploaded object.");
  }
  const remaining = expiration.getTime() - now.getTime();
  if (remaining < MIN_LIFECYCLE_MILLISECONDS || remaining > MAX_LIFECYCLE_MILLISECONDS) {
    throw new Error("R2 object lifecycle must expire visual media in approximately seven days.");
  }
}

async function rollback(keys, options) {
  let failures = 0;
  for (const key of [...keys].reverse()) {
    try {
      const response = await request({ ...options, method: "DELETE", key });
      if (!response.ok && response.status !== 404) failures += 1;
    } catch {
      failures += 1;
    }
  }
  return failures;
}

export async function publishMedia({
  mediaDirectory,
  outputFile,
  env = process.env,
  fetchImpl = globalThis.fetch,
  now = () => new Date()
}) {
  mkdirSync(path.dirname(outputFile), { recursive: true });
  const uploaded = [];
  try {
    if (typeof fetchImpl !== "function") throw new Error("This Node version does not provide fetch.");
    const credentials = validateCredentials(env);
    const media = validateManifest(mediaDirectory);
    const published = [];
    const requestTime = now();
    if (!(requestTime instanceof Date) || !Number.isFinite(requestTime.getTime())) throw new Error("Invalid publisher clock.");
    for (const entry of media.entries) {
      const prefix = `visual/pr-${entry.pullNumber}/${media.afterSha}/${entry.revision}`;
      const stem = `${entry.profile.viewport}-${entry.profile.id}`;
      const objects = [
        { kind: "video", key: `${prefix}/${stem}.mp4`, body: entry.videoBytes, contentType: "video/mp4" },
        { kind: "poster", key: `${prefix}/${stem}.png`, body: entry.posterBytes, contentType: "image/png" }
      ];
      const links = {};
      for (const object of objects) {
        const put = await request({
          method: "PUT",
          key: object.key,
          body: object.body,
          contentType: object.contentType,
          credentials,
          now: requestTime,
          fetchImpl
        });
        if (!put.ok) throw new Error(`R2 rejected a bounded ${object.kind} upload with status ${put.status}.`);
        uploaded.push(object.key);
        const head = await request({
          method: "HEAD",
          key: object.key,
          credentials,
          now: requestTime,
          fetchImpl
        });
        if (!head.ok) throw new Error(`R2 could not verify an uploaded ${object.kind} object (status ${head.status}).`);
        expirationFrom(head, requestTime);
        links[`${object.kind}Url`] = presignGet({ key: object.key, credentials, now: requestTime });
      }
      published.push({
        revision: entry.revision,
        pullNumber: entry.pullNumber,
        sha: entry.sha,
        profile: entry.profile,
        ...links
      });
    }
    const manifest = {
      version: 1,
      status: "success",
      expiresAt: new Date(requestTime.getTime() + LINK_SECONDS * 1000).toISOString(),
      entries: published
    };
    writeFileSync(outputFile, `${JSON.stringify(manifest, null, 2)}\n`);
    return manifest;
  } catch (error) {
    let rollbackFailures = 0;
    if (uploaded.length > 0) {
      try {
        const credentials = validateCredentials(env);
        const requestTime = now();
        rollbackFailures = await rollback(uploaded, { credentials, now: requestTime, fetchImpl });
      } catch {
        rollbackFailures = uploaded.length;
      }
    }
    const message = error instanceof Error ? error.message : String(error);
    const manifest = {
      version: 1,
      status: "error",
      error: rollbackFailures > 0 ? `${message} Cleanup failed for ${rollbackFailures} object(s).` : message,
      entries: []
    };
    writeFileSync(outputFile, `${JSON.stringify(manifest, null, 2)}\n`);
    throw error;
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const mediaDirectory = argumentValue("--media");
  const outputFile = argumentValue("--output");
  if (!mediaDirectory || !outputFile) {
    process.stderr.write("Usage: visual-review-r2.mjs --media DIR --output FILE\n");
    process.exitCode = 1;
  } else {
    try {
      await publishMedia({
        mediaDirectory: path.resolve(mediaDirectory),
        outputFile: path.resolve(outputFile)
      });
    } catch (error) {
      process.stderr.write(`${error instanceof Error ? error.message : String(error)}\n`);
      process.exitCode = 1;
    }
  }
}
