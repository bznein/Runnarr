#!/usr/bin/env bash

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

while IFS= read -r profile_label; do
  profiles_json="$(node "${ROOT}/scripts/visual-review-profiles.mjs" resolve --labels "${profile_label}")"
  expected_project="$(node -e 'process.stdout.write(JSON.parse(process.argv[1])[0].project)' "${profiles_json}")"
  listing="$(
    cd "${ROOT}/web"
    RUNNARR_VISUAL_PROFILES_JSON="${profiles_json}" \
      npx playwright test --list --config playwright.visual.config.ts
  )"
  test_count="$(awk '/^Total:/ { print $2 }' <<< "${listing}")"
  if [ "${test_count}" != "1" ]; then
    printf '%s\n' "${listing}" >&2
    echo "Profile ${profile_label} must select exactly one test; selected ${test_count:-none}." >&2
    exit 1
  fi
  if ! grep -Fq "[${expected_project}]" <<< "${listing}"; then
    printf '%s\n' "${listing}" >&2
    echo "Profile ${profile_label} did not select project ${expected_project}." >&2
    exit 1
  fi
done < <(node "${ROOT}/scripts/visual-review-profiles.mjs" labels)

echo "Validated every visual review profile against one Playwright test and viewport."
