#!/usr/bin/env bash

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
E2E_PROJECT="${RUNNARR_E2E_PROJECT:-runnarr-e2e-${BASHPID}}"
E2E_NETWORK="${E2E_PROJECT}_network"
E2E_USERNAME="${RUNNARR_E2E_USERNAME:-e2e-admin}"
E2E_PASSWORD="${RUNNARR_E2E_PASSWORD:-e2e-password-123}"
COMPOSE_OVERRIDE="$(mktemp "${TMPDIR:-/tmp}/runnarr-e2e-compose.XXXXXX.yml")"
NETWORK_CREATED=0

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

compose() {
  docker compose \
    --project-name "${E2E_PROJECT}" \
    --file "${ROOT}/docker-compose.yml" \
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
  if [ "${status}" -ne 0 ]; then
    compose logs --no-color || true
  fi
  compose down --volumes --remove-orphans || true
  if [ "${NETWORK_CREATED}" -eq 1 ]; then
    docker network rm "${E2E_NETWORK}" >/dev/null 2>&1 || true
  fi
  rm -f "${COMPOSE_OVERRIDE}"
  exit "${status}"
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

compose exec --no-TTY db psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -v "e2e_username=${E2E_USERNAME}" < "${ROOT}/web/e2e/seed.sql"

cd "${ROOT}/web"
npx playwright test "$@"
