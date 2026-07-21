#!/usr/bin/env bash

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ROOT}/.env"
ENV_TEMPLATE="${ROOT}/.env.example"
LOG_DIR="${ROOT}/tmp"
mkdir -p "${LOG_DIR}"

write_env_line() {
  local key="$1"
  local value="$2"
  local tmp_file
  if grep -q "^${key}=" "${ENV_FILE}"; then
    tmp_file="$(mktemp "${ROOT}/tmp/.runnarr-env-XXXXXX")"
    while IFS= read -r line || [ -n "${line}" ]; do
      if [[ "${line}" == "${key}="* ]]; then
        printf '%s=%s\n' "${key}" "${value}"
      else
        printf '%s\n' "${line}"
      fi
    done < "${ENV_FILE}" > "${tmp_file}"
    mv "${tmp_file}" "${ENV_FILE}"
    return
  fi
  printf '%s=%s\n' "${key}" "${value}" >> "${ENV_FILE}"
}

random_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 32 | tr -d '\n'
    return
  fi
  tr -dc 'A-Za-z0-9+/=' </dev/urandom | head -c 44
}

random_password() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 16
    return
  fi
  tr -dc 'A-Za-z0-9' </dev/urandom | head -c 24
}

random_high_port() {
  printf "%d" $((49152 + (RANDOM % 16384)))
}

find_free_frontend_port() {
  local start_port="${RUNNARR_FRONTEND_PORT:-5173}"
  local port="${start_port}"
  for attempt in $(seq 1 30); do
    if is_port_free "${port}"; then
      printf "%s\n" "${port}"
      return 0
    fi
    port=$((port + 1))
  done
  echo "No free local TCP port found for frontend near ${start_port}" >&2
  return 1
}

terminate_stale_frontend_5173() {
  if [ "${RUNNARR_KEEP_LEGACY_FRONTEND:-0}" = "1" ]; then
    return
  fi
  if ! command -v lsof >/dev/null 2>&1; then
    return
  fi
  if ! command -v ps >/dev/null 2>&1; then
    return
  fi

  local pids
  local to_kill=""
  pids="$(lsof -nP -iTCP:5173 -sTCP:LISTEN -t 2>/dev/null || true)"
  if [ -z "${pids}" ]; then
    return
  fi

  for pid in ${pids}; do
    if ps -p "${pid}" -o command= | rg -q "${ROOT}/web\\|runnarr-web@0.1.0 dev\\|vite --host 127.0.0.1"; then
      to_kill="${to_kill} ${pid}"
    fi
  done

  if [ -z "${to_kill}" ]; then
    return
  fi

  echo "Stopping stale Runnarr dev frontend process(es) on 5173:${to_kill}"
  kill ${to_kill} 2>/dev/null || true
  sleep 1
}

is_port_free() {
  local port="$1"
  ! ss -ltn | awk '{print $4}' | grep -q ":${port}$"
}

postgres_host_and_port() {
  local url="$1"
  local hostport
  hostport="${url#*://}"
  hostport="${hostport#*@}"
  hostport="${hostport%%/*}"
  hostport="${hostport%%\?*}"
  db_host="${hostport%%:*}"
  db_port="${hostport##*:}"
  if [ "${db_host}" = "${hostport}" ]; then
    db_port="5432"
  fi
}

postgres_ready() {
  local host="$1"
  local port="$2"
  if command -v pg_isready >/dev/null 2>&1; then
    pg_isready -h "${host}" -p "${port}" >/dev/null 2>&1
    return
  fi
  if command -v nc >/dev/null 2>&1; then
    nc -z "${host}" "${port}" </dev/null >/dev/null 2>&1
    return
  fi
  return 1
}

docker_db_host_port() {
  local mapped
  mapped="$(docker compose port db 5432 2>/dev/null | tr -d '[:space:]')"
  if [ -z "${mapped}" ]; then
    return 1
  fi
  printf "%s\n" "${mapped##*:}"
}

ensure_local_postgres() {
  postgres_host_and_port "${DATABASE_URL}"
  if [ "${db_host}" != "localhost" ] && [ "${db_host}" != "127.0.0.1" ] && [ "${db_host}" != "::1" ]; then
    return
  fi
  if postgres_ready "${db_host}" "${db_port}"; then
    return
  fi
  echo "PostgreSQL is not reachable at ${db_host}:${db_port}. Attempting to start dockerized postgres..."
  if [ "${RUNNARR_SKIP_DB_START:-0}" = "1" ]; then
    echo "Set RUNNARR_SKIP_DB_START=0 (default) or start PostgreSQL manually."
    exit 1
  fi
  if command -v docker >/dev/null 2>&1; then
    (cd "${ROOT}" && docker compose up -d --force-recreate db)
    for attempt in $(seq 1 30); do
      if postgres_ready "${db_host}" "${db_port}"; then
        return
      fi
      sleep 1
    done
    mapped_port="$(docker_db_host_port || true)"
    if [ -z "${mapped_port:-}" ]; then
      echo "PostgreSQL is running in docker but not exposed on localhost."
      echo "Set RUNNARR_DB_HOST_PORT in .env (for example RUNNARR_DB_HOST_PORT=5432) and rerun."
      echo "Then restart db: docker compose up -d --force-recreate db"
      exit 1
    fi
    if [ "${db_port}" != "${mapped_port}" ]; then
      echo "PostgreSQL host-port mismatch: DATABASE_URL expects ${db_host}:${db_port}, docker db is mapped to localhost:${mapped_port}."
      echo "Set DATABASE_URL to use localhost:${mapped_port} and rerun."
      exit 1
    fi
    echo "PostgreSQL did not become ready at ${db_host}:${db_port}."
    echo "Check docker compose state and logs: docker compose ps db && docker compose logs db"
    exit 1
  fi
  echo "Docker not available. Start PostgreSQL manually and ensure ${DATABASE_URL} is reachable."
  exit 1
}

bootstrap_env() {
  if [ ! -f "${ENV_FILE}" ]; then
    if [ ! -f "${ENV_TEMPLATE}" ]; then
      echo "Cannot start: .env.example not found."
      echo "Expected template at: ${ENV_TEMPLATE}"
      exit 1
    fi
    cp "${ENV_TEMPLATE}" "${ENV_FILE}"
    echo "Created ${ENV_FILE} from ${ENV_TEMPLATE}"
  fi

  set -a
  # shellcheck disable=SC1090
  source "${ENV_FILE}"
  set +a

  local updated=0
  local persisted=0
  local persist_runtime_defaults="${RUNNARR_PERSIST_RUNTIME_DEFAULTS:-0}"
  if [ -z "${DATABASE_URL:-}" ] || [ "${DATABASE_URL}" = "postgres://runnarr:runnarr@db:5432/runnarr?sslmode=disable" ]; then
    DATABASE_URL="postgres://runnarr:runnarr@localhost:5432/runnarr?sslmode=disable"
    if [ "${persist_runtime_defaults}" = "1" ]; then
      write_env_line "DATABASE_URL" "${DATABASE_URL}"
      persisted=1
    fi
    export DATABASE_URL
    updated=1
  fi

  if [ -z "${RUNNARR_HTTP_ADDR:-}" ] || [ "${RUNNARR_HTTP_ADDR}" = ":8080" ] || [ "${RUNNARR_HTTP_ADDR}" = "127.0.0.1:8080" ]; then
    for _ in $(seq 1 50); do
      backend_port="$(random_high_port)"
      if is_port_free "${backend_port}"; then
        break
      fi
      backend_port=""
    done
    if [ -z "${backend_port:-}" ]; then
      backend_port=59000
    fi
    RUNNARR_HTTP_ADDR="127.0.0.1:${backend_port}"
    if [ "${persist_runtime_defaults}" = "1" ]; then
      write_env_line "RUNNARR_HTTP_ADDR" "${RUNNARR_HTTP_ADDR}"
      persisted=1
    fi
    export RUNNARR_HTTP_ADDR
    updated=1
  fi

  if [ -z "${RUNNARR_SECRET_KEY:-}" ] || [ "${RUNNARR_SECRET_KEY}" = "change-this-to-a-long-random-secret-with-at-least-32-bytes" ]; then
    RUNNARR_SECRET_KEY="$(random_secret)"
    write_env_line "RUNNARR_SECRET_KEY" "${RUNNARR_SECRET_KEY}"
    export RUNNARR_SECRET_KEY
    updated=1
    persisted=1
    echo "Generated RUNNARR_SECRET_KEY in .env"
  fi

  if [ -z "${RUNNARR_ADMIN_PASSWORD_HASH:-}" ] && [ -z "${RUNNARR_ADMIN_PASSWORD:-}" ]; then
    RUNNARR_ADMIN_PASSWORD="$(random_password)"
    write_env_line "RUNNARR_ADMIN_PASSWORD" "${RUNNARR_ADMIN_PASSWORD}"
    export RUNNARR_ADMIN_PASSWORD
    updated=1
    persisted=1
    echo "Generated RUNNARR_ADMIN_PASSWORD in .env: ${RUNNARR_ADMIN_PASSWORD}"
  fi

  if [ "${persisted}" -eq 1 ]; then
    echo "Updated ${ENV_FILE} with runtime defaults."
  elif [ "${updated}" -eq 1 ]; then
    echo "Using runtime defaults in this session from ${ENV_FILE}."
  fi

  set -a
  # shellcheck disable=SC1090
  source "${ENV_FILE}"
  set +a
}

bootstrap_env

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

if [ "${RUNNARR_PUBLIC_MODE:-false}" != "true" ] && [ "${RUNNARR_PUBLIC_MODE:-0}" != "1" ]; then
  case "${RUNNARR_HTTP_ADDR:-}" in
    :*) RUNNARR_HTTP_ADDR="127.0.0.1${RUNNARR_HTTP_ADDR}"; export RUNNARR_HTTP_ADDR ;;
    0.0.0.0:*) RUNNARR_HTTP_ADDR="127.0.0.1${RUNNARR_HTTP_ADDR#0.0.0.0}"; export RUNNARR_HTTP_ADDR ;;
    \[::\]:*) RUNNARR_HTTP_ADDR="127.0.0.1:${RUNNARR_HTTP_ADDR##*:}"; export RUNNARR_HTTP_ADDR ;;
  esac
fi

if [ ! -d "${ROOT}/web/node_modules" ]; then
  echo "Installing frontend dependencies..."
  (cd "${ROOT}/web" && npm install)
fi

ensure_local_postgres

if [ ! -f "${ROOT}/.env" ]; then
  echo "No .env file found. Copy .env.example and adjust values if needed:"
  echo "cp .env.example .env && source .env"
  echo
fi

terminate_stale_frontend_5173

backend_addr="${RUNNARR_HTTP_ADDR:-:8080}"
if [[ "${backend_addr}" == *":"* ]]; then
  backend_port="${backend_addr##*:}"
else
  backend_port="${backend_addr}"
fi
api_target="http://localhost:${backend_port}"

echo "Starting backend on ${api_target} ..."
frontend_port="$(find_free_frontend_port)"

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
  VITE_API_TARGET="${api_target}" npm run dev -- --host 127.0.0.1 --port "${frontend_port}"
) >"${LOG_DIR}/runnarr-frontend.log" 2>&1 &
frontend_pid=$!

sleep 1
echo "Backend PID: ${backend_pid} | log: ${LOG_DIR}/runnarr-backend.log"
echo "Frontend PID: ${frontend_pid} | log: ${LOG_DIR}/runnarr-frontend.log"
echo "Frontend: http://localhost:${frontend_port}"
echo "Backend:  http://localhost:${backend_port}"
echo "API proxy target: ${api_target}"
echo "Press Ctrl+C to stop both processes."

backend_exit=0
wait "${backend_pid}" || backend_exit=$?
if [ "${backend_exit}" -ne 0 ]; then
  echo "Backend exited with status ${backend_exit}. Showing tail of ${LOG_DIR}/runnarr-backend.log"
  tail -n 80 "${LOG_DIR}/runnarr-backend.log"
  exit "${backend_exit}"
fi
