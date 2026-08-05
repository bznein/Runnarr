#!/usr/bin/env bash

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_REF="${VISUAL_BASE_REF:-origin/main}"
PROFILE_LABELS="${VISUAL_PROFILES:-}"
TEMP_ROOT=""
BEFORE_ROOT=""
WORKTREE_ADDED=0

cleanup_worktree() {
  local status="$?"
  trap - EXIT
  if [ "${WORKTREE_ADDED}" -eq 1 ]; then
    if ! git -C "${ROOT}" worktree remove "${BEFORE_ROOT}"; then
      echo "Could not remove generated worktree ${BEFORE_ROOT}; remove it with git worktree remove after inspection." >&2
      exit 1
    fi
  fi
  if [ -n "${TEMP_ROOT}" ] && [ -d "${TEMP_ROOT}" ]; then
    rmdir "${TEMP_ROOT}" 2>/dev/null || true
  fi
  exit "${status}"
}

trap cleanup_worktree EXIT

if [ -z "${PROFILE_LABELS}" ]; then
  echo "Set VISUAL_PROFILES to one or two comma-separated visual profile labels." >&2
  exit 1
fi
if [ -n "$(git -C "${ROOT}" status --porcelain)" ]; then
  echo "Local visual comparisons require a clean, committed worktree." >&2
  exit 1
fi

PROFILES_JSON="$(node "${ROOT}/scripts/visual-review-profiles.mjs" resolve --labels "${PROFILE_LABELS}")"
if [ "$(node -e 'const profiles = JSON.parse(process.argv[1]); process.stdout.write(String(profiles.length));' "${PROFILES_JSON}")" -eq 0 ]; then
  echo "VISUAL_PROFILES did not select a visual review profile." >&2
  exit 1
fi

BASE_SHA="$(git -C "${ROOT}" rev-parse --verify "${BASE_REF}^{commit}")"
HEAD_SHA="$(git -C "${ROOT}" rev-parse --verify HEAD)"
MERGE_BASE_SHA="$(git -C "${ROOT}" merge-base "${BASE_SHA}" "${HEAD_SHA}")"
FIXTURE_DATE="$(TZ=Europe/Dublin date +%F)"
FIXTURE_TIMESTAMP="${FIXTURE_DATE}T12:00:00Z"
RUN_STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUTPUT_ROOT="${ROOT}/web/test-results/visual-review/${RUN_STAMP}-${MERGE_BASE_SHA:0:12}-${HEAD_SHA:0:12}"

TEMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/runnarr-visual-review.XXXXXX")"
BEFORE_ROOT="${TEMP_ROOT}/before"
git -C "${ROOT}" worktree add --detach "${BEFORE_ROOT}" "${MERGE_BASE_SHA}"
WORKTREE_ADDED=1

run_revision() {
  local revision="$1"
  local app_root="$2"
  local artifact_dir="${OUTPUT_ROOT}/${revision}"
  RUNNARR_E2E_APP_ROOT="${app_root}" \
  RUNNARR_E2E_DRIVER_ROOT="${ROOT}" \
  RUNNARR_E2E_SEED_FILE="${ROOT}/web/e2e/seed.sql" \
  RUNNARR_E2E_ARTIFACT_DIR="${artifact_dir}" \
  RUNNARR_E2E_FIXTURE_DATE="${FIXTURE_DATE}" \
  RUNNARR_E2E_FIXTURE_TIMESTAMP="${FIXTURE_TIMESTAMP}" \
  RUNNARR_E2E_PROJECT="runnarr-visual-${BASHPID}-${revision}" \
  RUNNARR_VISUAL_PROFILES_JSON="${PROFILES_JSON}" \
  PLAYWRIGHT_SLOW_MO="200" \
    bash "${ROOT}/scripts/e2e.sh" --config playwright.visual.config.ts
}

before_status=0
after_status=0
if run_revision before "${BEFORE_ROOT}"; then
  :
else
  before_status="$?"
fi
if run_revision after "${ROOT}"; then
  :
else
  after_status="$?"
fi

mkdir -p "${OUTPUT_ROOT}"
{
  printf 'profiles=%s\n' "${PROFILE_LABELS}"
  printf 'merge_base=%s\n' "${MERGE_BASE_SHA}"
  printf 'head=%s\n' "${HEAD_SHA}"
  printf 'fixture_date=%s\n' "${FIXTURE_DATE}"
  printf 'before_status=%s\n' "${before_status}"
  printf 'after_status=%s\n' "${after_status}"
} > "${OUTPUT_ROOT}/comparison-metadata.txt"

printf 'Visual review artifacts: %s\n' "${OUTPUT_ROOT}"
printf 'Before status: %s; after status: %s\n' "${before_status}" "${after_status}"

if [ "${after_status}" -ne 0 ]; then
  exit "${after_status}"
fi
