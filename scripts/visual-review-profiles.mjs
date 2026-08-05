#!/usr/bin/env node

import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
const repositoryRoot = path.resolve(scriptDirectory, "..");
const defaultCatalogPath = path.join(repositoryRoot, ".github", "visual-review-profiles.json");
const profilePattern = /^visual:(desktop|mobile):([a-z0-9]+(?:-[a-z0-9]+)*)$/;

export function loadCatalog(catalogPath = defaultCatalogPath) {
  const catalog = JSON.parse(readFileSync(catalogPath, "utf8"));
  if (catalog.version !== 1 || !Array.isArray(catalog.scenarios)) {
    throw new Error("Visual review catalog must use version 1 with a scenarios array.");
  }
  return catalog;
}

export function catalogLabels(catalog = loadCatalog()) {
  return catalog.scenarios.flatMap((scenario) =>
    scenario.viewports.map((viewport) => `visual:${viewport}:${scenario.id}`)
  );
}

export function resolveProfiles(labels, catalog = loadCatalog()) {
  if (!Array.isArray(labels) || labels.some((label) => typeof label !== "string")) {
    throw new Error("Visual review labels must be a JSON array of strings.");
  }

  const scenarios = new Map(catalog.scenarios.map((scenario) => [scenario.id, scenario]));
  const selectedLabels = labels.filter((label) => label.startsWith("visual:")).sort();
  const duplicate = selectedLabels.find((label, index) => selectedLabels.indexOf(label) !== index);
  if (duplicate) throw new Error(`Duplicate visual review profile: ${duplicate}`);
  if (selectedLabels.length > 2) {
    throw new Error(`Select at most two visual review profiles; received ${selectedLabels.length}.`);
  }

  return selectedLabels.map((label) => {
    const match = profilePattern.exec(label);
    if (!match) throw new Error(`Invalid visual review profile label: ${label}`);
    const [, viewport, id] = match;
    const scenario = scenarios.get(id);
    if (!scenario) throw new Error(`Unknown visual review scenario: ${id}`);
    if (!scenario.viewports.includes(viewport)) {
      throw new Error(`Scenario ${id} does not support the ${viewport} viewport.`);
    }
    return {
      label,
      id,
      tag: scenario.tag,
      viewport,
      project: viewport === "mobile" ? "mobile-chromium" : "chromium"
    };
  });
}

function argumentValue(name) {
  const index = process.argv.indexOf(name);
  if (index === -1 || index + 1 >= process.argv.length) return undefined;
  return process.argv[index + 1];
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  try {
    const command = process.argv[2];
    if (command === "labels") {
      process.stdout.write(`${catalogLabels().join("\n")}\n`);
    } else if (command === "resolve") {
      const labelsJSON = argumentValue("--labels-json") ?? process.env.RUNNARR_VISUAL_LABELS_JSON;
      const labelsList = argumentValue("--labels");
      if (!labelsJSON && !labelsList) {
        throw new Error("resolve requires --labels-json, --labels, or RUNNARR_VISUAL_LABELS_JSON.");
      }
      const labels = labelsJSON
        ? JSON.parse(labelsJSON)
        : labelsList.split(",").map((label) => label.trim()).filter(Boolean);
      process.stdout.write(`${JSON.stringify(resolveProfiles(labels))}\n`);
    } else {
      throw new Error("Usage: visual-review-profiles.mjs labels | resolve (--labels-json JSON | --labels CSV)");
    }
  } catch (error) {
    process.stderr.write(`${error instanceof Error ? error.message : String(error)}\n`);
    process.exitCode = 1;
  }
}
