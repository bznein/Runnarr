#!/usr/bin/env bash

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_ROOT="$(cd "${RUNNARR_E2E_APP_ROOT:-${ROOT}}" && pwd)"
DRIVER_ROOT="$(cd "${RUNNARR_E2E_DRIVER_ROOT:-${ROOT}}" && pwd)"
SEED_FILE="${RUNNARR_E2E_SEED_FILE:-${DRIVER_ROOT}/web/e2e/seed.sql}"
TESTBED_SEED_FILE="${RUNNARR_E2E_TESTBED_SEED_FILE:-${DRIVER_ROOT}/web/e2e/testbed-seed.sql}"
ARTIFACT_DIR="${RUNNARR_E2E_ARTIFACT_DIR:-}"
E2E_PROJECT="${RUNNARR_E2E_PROJECT:-runnarr-e2e-${BASHPID}}"
E2E_NETWORK="${E2E_PROJECT}_network"
E2E_USERNAME="${RUNNARR_E2E_USERNAME:-e2e-admin}"
E2E_PASSWORD="${RUNNARR_E2E_PASSWORD:-e2e-password-123}"
E2E_FIXTURE_DATE="${RUNNARR_E2E_FIXTURE_DATE:-$(TZ=Europe/Dublin date +%F)}"
E2E_FIXTURE_TIMESTAMP="${RUNNARR_E2E_FIXTURE_TIMESTAMP:-${E2E_FIXTURE_DATE}T12:00:00Z}"
COMPOSE_OVERRIDE=""
NETWORK_CREATED=0
TESTBED_MODE=0

if [ "${1:-}" = "--testbed" ]; then
  TESTBED_MODE=1
  shift
fi

if [[ ! "${E2E_PROJECT}" =~ ^[a-z0-9][a-z0-9_-]*$ ]]; then
  echo "Invalid Docker Compose project name: ${E2E_PROJECT}" >&2
  exit 1
fi
if [[ ! "${E2E_FIXTURE_DATE}" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
  echo "RUNNARR_E2E_FIXTURE_DATE must use YYYY-MM-DD." >&2
  exit 1
fi
if [[ ! "${E2E_FIXTURE_TIMESTAMP}" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]]; then
  echo "RUNNARR_E2E_FIXTURE_TIMESTAMP must be a UTC timestamp ending in Z." >&2
  exit 1
fi
if [ ! -f "${APP_ROOT}/docker-compose.yml" ]; then
  echo "No docker-compose.yml found under app root ${APP_ROOT}." >&2
  exit 1
fi
if [ ! -f "${SEED_FILE}" ]; then
  echo "E2E seed file not found: ${SEED_FILE}" >&2
  exit 1
fi

pick_port() {
  local start_port="$1"
  local candidate
  for candidate in $(seq "${start_port}" "$((start_port + 100))"); do
    if ! ss -ltn | awk '{print $4}' | grep -q ":${candidate}$"; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  done
  echo "No free port found near ${start_port}" >&2
  return 1
}

E2E_APP_PORT="${RUNNARR_E2E_PORT:-$(pick_port 37617)}"
E2E_DB_PORT="${RUNNARR_E2E_DB_PORT:-$(pick_port 35432)}"
COMPOSE_OVERRIDE="$(mktemp "${TMPDIR:-/tmp}/runnarr-e2e-compose.XXXXXX.yml")"
export COMPOSE_PROJECT_NAME="${E2E_PROJECT}"
export DATABASE_URL="postgres://runnarr:runnarr@db:5432/runnarr?sslmode=disable"
export POSTGRES_USER="runnarr"
export POSTGRES_PASSWORD="runnarr"
export POSTGRES_DB="runnarr"
export RUNNARR_PORT="${E2E_APP_PORT}"
export RUNNARR_DB_HOST_PORT="${E2E_DB_PORT}"
export RUNNARR_BASE_URL="http://127.0.0.1:${E2E_APP_PORT}"
export RUNNARR_ADMIN_USERNAME="${E2E_USERNAME}"
export RUNNARR_ADMIN_PASSWORD="${E2E_PASSWORD}"
export RUNNARR_SECRET_KEY="runnarr-e2e-secret-key-change-me"
export RUNNARR_PUBLIC_MODE="false"
export RUNNARR_LOCAL_AUTH_ENABLED="true"
export PLAYWRIGHT_BASE_URL="${RUNNARR_BASE_URL}"
export RUNNARR_GARMIN_BRIDGE_SCRIPT="/app/garmin_bridge_testbed.py"
export RUNNARR_E2E_FIXTURE_DATE="${E2E_FIXTURE_DATE}"
export RUNNARR_E2E_FIXTURE_TIMESTAMP="${E2E_FIXTURE_TIMESTAMP}"

compose() {
  docker compose \
    --project-name "${E2E_PROJECT}" \
    --file "${APP_ROOT}/docker-compose.yml" \
    --file "${COMPOSE_OVERRIDE}" \
    "$@"
}

create_network() {
  local first_octet
  local second_octet
  local subnet
  for first_octet in 240 241 242 243 244 245 246 247 248 249 250 251 252 253 254; do
    for second_octet in $(seq 0 255); do
      subnet="10.${first_octet}.${second_octet}.0/24"
      if docker network create --driver bridge --subnet "${subnet}" "${E2E_NETWORK}" >/dev/null 2>&1; then
        return 0
      fi
    done
  done
  echo "No non-overlapping Docker subnet found for the E2E network" >&2
  return 1
}

cleanup() {
  local status="$?"
  if [ -n "${ARTIFACT_DIR}" ]; then
    mkdir -p "${ARTIFACT_DIR}"
    compose ps --all > "${ARTIFACT_DIR}/compose-status.txt" 2>&1 || true
    compose logs --no-color > "${ARTIFACT_DIR}/compose.log" 2>&1 || true
    {
      printf 'compose_project=%s\n' "${E2E_PROJECT}"
      printf 'app_root=%s\n' "${APP_ROOT}"
      printf 'fixture_date=%s\n' "${E2E_FIXTURE_DATE}"
      printf 'fixture_timestamp=%s\n' "${E2E_FIXTURE_TIMESTAMP}"
      printf 'playwright_status=%s\n' "${status}"
    } > "${ARTIFACT_DIR}/run-metadata.txt"
  fi
  if [ "${status}" -ne 0 ]; then
    compose logs --no-color || true
  fi
  compose down --volumes --remove-orphans || true
  if [ "${NETWORK_CREATED}" -eq 1 ]; then
    docker network rm "${E2E_NETWORK}" >/dev/null 2>&1 || true
  fi
  if [ -n "${COMPOSE_OVERRIDE}" ]; then
    rm -f "${COMPOSE_OVERRIDE}"
  fi
  exit "${status}"
}

stop_testbed() {
  printf '\nStopping the testbed...\n'
  exit 0
}

trap cleanup EXIT

printf '%s\n' \
  'services:' \
  '  db:' \
  '    networks: !override' \
  '      - e2e' \
  '  app:' \
  '    networks: !override' \
  '      - e2e' \
  'networks:' \
  '  e2e:' \
  '    external: true' \
  "    name: ${E2E_NETWORK}" > "${COMPOSE_OVERRIDE}"

create_network
NETWORK_CREATED=1
compose up --build --detach

for attempt in $(seq 1 60); do
  if curl --fail --silent "${RUNNARR_BASE_URL}/healthz" >/dev/null; then
    break
  fi
  if [ "${attempt}" -eq 60 ]; then
    echo "Runnarr did not become healthy" >&2
    exit 1
  fi
  sleep 2
done

compose exec --no-TTY db psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" \
  -v "e2e_username=${E2E_USERNAME}" \
  -v "e2e_date=${E2E_FIXTURE_DATE}" \
  -v "e2e_now=${E2E_FIXTURE_TIMESTAMP}" < "${SEED_FILE}"

if [ "${TESTBED_MODE}" -eq 1 ]; then
  if [ ! -f "${TESTBED_SEED_FILE}" ]; then
    echo "E2E testbed seed file not found: ${TESTBED_SEED_FILE}" >&2
    exit 1
  fi
  compose exec --no-TTY db psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" \
    -v "e2e_username=${E2E_USERNAME}" \
    -v "e2e_date=${E2E_FIXTURE_DATE}" \
    -v "e2e_now=${E2E_FIXTURE_TIMESTAMP}" < "${TESTBED_SEED_FILE}"
  printf '\nRunnarr testbed is ready.\n'
  printf 'URL:      %s\n' "${RUNNARR_BASE_URL}"
  printf 'Username: %s\n' "${E2E_USERNAME}"
  printf 'Password: %s\n' "${E2E_PASSWORD}"
  printf '\nThis isolated environment contains synthetic activities, plans, workouts, health, and gear data.\n'
  printf 'Garmin operations use an offline fake account containing one protected foreign workout.\n'
  printf 'Press Ctrl-C to stop it and remove its containers, network, and volumes.\n\n'
  trap stop_testbed HUP INT TERM
  while true; do
    sleep 60
  done
fi

cd "${DRIVER_ROOT}/web"
npx playwright test "$@"
