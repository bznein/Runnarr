#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  realpathSync,
  statSync,
  writeFileSync
} from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const MAX_VIDEO_BYTES = 25 * 1024 * 1024;
const MAX_POSTER_BYTES = 1024 * 1024;
const MAX_DURATION_SECONDS = 60;
const MAX_ENTRIES = 4;
const ARTIFACT_PATTERN = /^runnarr-visual-(before|after)-pr-([1-9][0-9]*)-([0-9a-f]{40})$/;
const WEBM_MAGIC = Buffer.from([0x1a, 0x45, 0xdf, 0xa3]);
const PNG_MAGIC = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);

function argumentValue(name, argv = process.argv.slice(2)) {
  const index = argv.indexOf(name);
  if (index === -1 || index + 1 >= argv.length) return undefined;
  return argv[index + 1];
}

function inside(parent, candidate) {
  const relative = path.relative(parent, candidate);
  return relative !== "" && !relative.startsWith("..") && !path.isAbsolute(relative);
}

function specsIn(suites) {
  return suites.flatMap((suite) => [
    ...(suite.specs ?? []),
    ...specsIn(suite.suites ?? [])
  ]);
}

function safeAttachmentPath(artifactRoot, attachmentPath) {
  if (typeof attachmentPath !== "string") throw new Error("Video attachment is missing its path.");
  const normalized = attachmentPath.replaceAll("\\", "/");
  const marker = "/test-results/";
  const markerIndex = normalized.lastIndexOf(marker);
  if (markerIndex === -1) throw new Error(`Video attachment is outside test-results: ${attachmentPath}`);
  const suffix = normalized.slice(markerIndex + marker.length);
  if (!suffix || suffix.split("/").some((part) => part === ".." || part === "")) {
    throw new Error(`Unsafe video attachment path: ${attachmentPath}`);
  }
  const candidate = path.resolve(artifactRoot, "test-results", suffix);
  if (!existsSync(candidate)) throw new Error(`Video attachment was not included in the artifact: ${suffix}`);
  const resolvedRoot = realpathSync(artifactRoot);
  const resolvedCandidate = realpathSync(candidate);
  if (!inside(resolvedRoot, resolvedCandidate)) throw new Error(`Video attachment escapes its artifact: ${suffix}`);
  return resolvedCandidate;
}

function run(command, args) {
  const result = spawnSync(command, args, { encoding: "utf8" });
  if (result.error && result.status === null) throw result.error;
  if (result.status !== 0) {
    throw new Error(`${command} failed: ${(result.stderr || result.stdout || `exit ${result.status}`).trim()}`);
  }
  return result.stdout;
}

function probeVideo(videoPath) {
  const raw = run("ffprobe", [
    "-v", "error",
    "-select_streams", "v:0",
    "-show_entries", "stream=codec_name,width,height,duration:format=duration",
    "-of", "json",
    videoPath
  ]);
  const parsed = JSON.parse(raw);
  const stream = parsed.streams?.[0];
  if (!stream) throw new Error(`No video stream found in ${videoPath}`);
  const duration = Number(stream.duration ?? parsed.format?.duration);
  const width = Number(stream.width);
  const height = Number(stream.height);
  if (!Number.isFinite(duration) || duration <= 0 || duration > MAX_DURATION_SECONDS) {
    throw new Error(`Video duration ${duration} is outside 0-${MAX_DURATION_SECONDS} seconds.`);
  }
  if (!Number.isInteger(width) || !Number.isInteger(height) || width <= 0 || height <= 0 || width > 800 || height > 800) {
    throw new Error(`Video dimensions ${width}x${height} exceed the 800px recording bound.`);
  }
  if (!["vp8", "vp9"].includes(stream.codec_name)) {
    throw new Error(`Unexpected source video codec: ${stream.codec_name}`);
  }
  return { codec: stream.codec_name, duration, width, height };
}

function assertMagic(filePath, expected, description) {
  const bytes = readFileSync(filePath).subarray(0, Math.max(expected.length, 12));
  if (description === "MP4") {
    if (bytes.length < 12 || bytes.subarray(4, 8).toString("ascii") !== "ftyp") {
      throw new Error(`${filePath} is not an MP4 file.`);
    }
    return;
  }
  if (!bytes.subarray(0, expected.length).equals(expected)) {
    throw new Error(`${filePath} is not a ${description} file.`);
  }
}

function validateProfiles(profiles) {
  if (!Array.isArray(profiles) || profiles.length < 1 || profiles.length > 2) {
    throw new Error("Media sanitization requires one or two visual profiles.");
  }
  for (const profile of profiles) {
    if (!profile || !/^visual:(desktop|mobile):[a-z0-9]+(?:-[a-z0-9]+)*$/.test(profile.label)) {
      throw new Error("Invalid visual profile in publisher input.");
    }
    if (!/^@visual-[a-z0-9]+(?:-[a-z0-9]+)*$/.test(profile.tag)) {
      throw new Error(`Invalid visual tag for ${profile.label}.`);
    }
    const expectedProject = profile.viewport === "mobile" ? "mobile-chromium" : "chromium";
    if (
      profile.label !== `visual:${profile.viewport}:${profile.id}` ||
      profile.tag !== `@visual-${profile.id}` ||
      profile.project !== expectedProject
    ) {
      throw new Error(`Inconsistent visual profile fields for ${profile.label}.`);
    }
  }
}

export function collectMediaPlans(artifactDirectory, profiles) {
  validateProfiles(profiles);
  const artifacts = readdirSync(artifactDirectory, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => ({ entry, match: ARTIFACT_PATTERN.exec(entry.name) }))
    .filter(({ match }) => match)
    .map(({ entry, match }) => ({
      root: path.join(artifactDirectory, entry.name),
      name: entry.name,
      revision: match[1],
      pullNumber: Number(match[2]),
      sha: match[3]
    }));
  if (artifacts.length !== 2 || new Set(artifacts.map((item) => item.revision)).size !== 2) {
    throw new Error("Expected exactly one before and one after recording artifact.");
  }

  const pullNumbers = new Set(artifacts.map((item) => item.pullNumber));
  if (pullNumbers.size !== 1) throw new Error("Before and after artifacts refer to different pull requests.");

  const plans = [];
  for (const artifact of artifacts) {
    const reportPath = path.join(artifact.root, "playwright-report.json");
    if (!existsSync(reportPath)) throw new Error(`${artifact.name} does not contain playwright-report.json.`);
    const report = JSON.parse(readFileSync(reportPath, "utf8"));
    const specs = specsIn(report.suites ?? []);
    for (const profile of profiles) {
      const matches = [];
      for (const spec of specs) {
        if (!(spec.tags ?? []).includes(profile.tag.slice(1))) continue;
        for (const test of spec.tests ?? []) {
          if (test.projectName !== profile.project) continue;
          for (const result of test.results ?? []) {
            for (const attachment of result.attachments ?? []) {
              if (attachment.name === "video" && attachment.contentType === "video/webm" && attachment.path) {
                matches.push({ spec, test, result, attachment });
              }
            }
          }
        }
      }
      if (matches.length !== 1) {
        throw new Error(`${artifact.revision}/${profile.label} must contain exactly one video; found ${matches.length}.`);
      }
      const source = safeAttachmentPath(artifact.root, matches[0].attachment.path);
      if (statSync(source).size > MAX_VIDEO_BYTES) {
        throw new Error(`${artifact.revision}/${profile.label} exceeds 25 MB.`);
      }
      assertMagic(source, WEBM_MAGIC, "WebM");
      plans.push({
        revision: artifact.revision,
        pullNumber: artifact.pullNumber,
        sha: artifact.sha,
        profile,
        source
      });
    }
  }
  if (plans.length > MAX_ENTRIES) throw new Error(`At most ${MAX_ENTRIES} videos may be published.`);
  return plans.sort((left, right) =>
    left.revision.localeCompare(right.revision) || left.profile.label.localeCompare(right.profile.label)
  );
}

export function sanitizeMedia({ artifactDirectory, outputDirectory, profiles }) {
  mkdirSync(outputDirectory, { recursive: true });
  const manifestPath = path.join(outputDirectory, "media-manifest.json");
  try {
    const plans = collectMediaPlans(artifactDirectory, profiles);
    const entries = [];
    for (const plan of plans) {
      const probe = probeVideo(plan.source);
      if (plan.profile.viewport === "mobile" ? probe.height <= probe.width : probe.width <= probe.height) {
        throw new Error(`${plan.revision}/${plan.profile.label} has the wrong orientation.`);
      }
      const stem = `${plan.revision}--${plan.profile.viewport}--${plan.profile.id}`;
      const videoName = `${stem}.mp4`;
      const posterName = `${stem}.png`;
      const videoPath = path.join(outputDirectory, videoName);
      const posterPath = path.join(outputDirectory, posterName);
      run("ffmpeg", [
        "-nostdin", "-v", "error", "-y", "-i", plan.source,
        "-map", "0:v:0", "-an", "-c:v", "libx264", "-preset", "medium", "-crf", "26",
        "-threads", "2", "-pix_fmt", "yuv420p", "-movflags", "+faststart",
        "-t", String(MAX_DURATION_SECONDS), videoPath
      ]);
      run("ffmpeg", [
        "-nostdin", "-v", "error", "-y", "-ss", "0.5", "-i", plan.source,
        "-frames:v", "1", "-threads", "2", "-vf", "scale='min(400,iw)':-2", posterPath
      ]);
      assertMagic(videoPath, Buffer.alloc(0), "MP4");
      assertMagic(posterPath, PNG_MAGIC, "PNG");
      if (statSync(videoPath).size > MAX_VIDEO_BYTES) throw new Error(`${videoName} exceeds 25 MB.`);
      if (statSync(posterPath).size > MAX_POSTER_BYTES) throw new Error(`${posterName} exceeds 1 MB.`);
      entries.push({
        revision: plan.revision,
        pullNumber: plan.pullNumber,
        sha: plan.sha,
        profile: plan.profile,
        video: videoName,
        poster: posterName,
        duration: probe.duration,
        width: probe.width,
        height: probe.height
      });
    }
    const manifest = { version: 1, status: "success", entries };
    writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
    return manifest;
  } catch (error) {
    const manifest = {
      version: 1,
      status: "error",
      error: error instanceof Error ? error.message : String(error),
      entries: []
    };
    writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
    throw error;
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const artifactDirectory = argumentValue("--artifacts");
  const outputDirectory = argumentValue("--output");
  const profilesJSON = argumentValue("--profiles-json") ?? process.env.RUNNARR_VISUAL_PROFILES_JSON;
  if (!artifactDirectory || !outputDirectory || !profilesJSON) {
    process.stderr.write("Usage: visual-review-media.mjs --artifacts DIR --output DIR --profiles-json JSON\n");
    process.exitCode = 1;
  } else {
    try {
      sanitizeMedia({
        artifactDirectory: path.resolve(artifactDirectory),
        outputDirectory: path.resolve(outputDirectory),
        profiles: JSON.parse(profilesJSON)
      });
    } catch (error) {
      process.stderr.write(`${error instanceof Error ? error.message : String(error)}\n`);
      process.exitCode = 1;
    }
  }
}
