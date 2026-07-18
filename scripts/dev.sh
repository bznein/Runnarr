#!/usr/bin/env bash

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="${ROOT}/tmp"
mkdir -p "${LOG_DIR}"

required_vars=(
  "DATABASE_URL"
  "RUNNARR_ADMIN_PASSWORD:RUNNARR_ADMIN_PASSWORD or RUNNARR_ADMIN_PASSWORD_HASH"
  "RUNNARR_SECRET_KEY"
)
missing_vars=()
for entry in "${required_vars[@]}"; do
  IFS=: read -r var desc <<< "${entry}"
  value="${!var:-}"
  if [ -z "${value}" ]; then
    if [ "${var}" = "RUNNARR_ADMIN_PASSWORD" ]; then
      if [ -z "${RUNNARR_ADMIN_PASSWORD_HASH:-}" ]; then
        missing_vars+=("${desc}")
      fi
    else
      missing_vars+=("${desc}")
    fi
  fi
done

if [ "${#missing_vars[@]}" -gt 0 ]; then
  echo "Missing required environment variables:"
  for item in "${missing_vars[@]}"; do
    echo "- ${item}"
  done
  echo
  echo "Tip: source your .env file with: source .env"
  exit 1
fi

if [ ! -d "${ROOT}/web/node_modules" ]; then
  echo "Installing frontend dependencies..."
  (cd "${ROOT}/web" && npm install)
fi

if [ ! -f "${ROOT}/.env" ]; then
  echo "No .env file found. Copy .env.example and adjust values if needed:"
  echo "cp .env.example .env && source .env"
  echo
fi

echo "Starting backend on http://localhost:${RUNNARR_HTTP_ADDR:-8080} and frontend on http://localhost:5173 ..."

backend_pid=""
frontend_pid=""
cleanup() {
  if [ -n "${frontend_pid}" ] && kill -0 "${frontend_pid}" 2>/dev/null; then
    kill "${frontend_pid}" 2>/dev/null || true
  fi
  if [ -n "${backend_pid}" ] && kill -0 "${backend_pid}" 2>/dev/null; then
    kill "${backend_pid}" 2>/dev/null || true
  fi
  wait "${backend_pid}" 2>/dev/null || true
  wait "${frontend_pid}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

(
  cd "${ROOT}"
  go run ./cmd/runnarr >"${LOG_DIR}/runnarr-backend.log" 2>&1
) &
backend_pid=$!

(
  cd "${ROOT}/web"
  npm run dev
) >"${LOG_DIR}/runnarr-frontend.log" 2>&1 &
frontend_pid=$!

sleep 1
echo "Backend PID: ${backend_pid} | log: ${LOG_DIR}/runnarr-backend.log"
echo "Frontend PID: ${frontend_pid} | log: ${LOG_DIR}/runnarr-frontend.log"
echo "Frontend: http://localhost:5173"
backend_host="localhost"
backend_addr="${RUNNARR_HTTP_ADDR:-:8080}"
if [[ "${backend_addr}" == *":"* ]]; then
  backend_port="${backend_addr##*:}"
else
  backend_port="${backend_addr}"
fi
echo "Backend:  http://localhost:${backend_port}"
echo "Press Ctrl+C to stop both processes."

wait "${backend_pid}"
