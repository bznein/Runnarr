#!/usr/bin/env bash

set -euo pipefail
umask 077

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run this installer as root." >&2
  exit 1
fi
if [[ "$#" -ne 0 && "$#" -ne 1 && "$#" -ne 5 ]]; then
  echo "usage: configure-preview-routing.sh [VALHALLA_TILE_URL [FROM_LAT FROM_LON TO_LAT TO_LON]]" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ASSET_ROOT="/opt/runnarr-deploy"
CONFIG_ROOT="/etc/runnarr"
STATE_ROOT="/srv/runnarr"
DEPLOY_USER="runnarr-deploy"
ROUTING_ENV="${CONFIG_ROOT}/preview-routing.env"
ROUTING_COMPOSE="${ASSET_ROOT}/docker-compose.routing.yml"
TILE_URL="${1:-https://download.geofabrik.de/europe/ireland-and-northern-ireland-latest.osm.pbf}"
SMOKE_FROM_LAT="${2:-53.3438}"
SMOKE_FROM_LON="${3:--6.2546}"
SMOKE_TO_LAT="${4:-53.3382}"
SMOKE_TO_LON="${5:--6.2591}"
VALHALLA_IMAGE="ghcr.io/valhalla/valhalla-scripted:3.6.3@sha256:e688a89f7a86880aabcc8b2eec9bcefdcb639d603ee90a928a6d3c7d92d58486"

[[ "${TILE_URL}" =~ ^https://[-A-Za-z0-9._~:/%+]+\.osm\.pbf([?][-A-Za-z0-9._~:/?\&=%+]*)?$ ]] || {
  echo "VALHALLA_TILE_URL must be an HTTPS .osm.pbf URL." >&2
  exit 1
}
for coordinate in "${SMOKE_FROM_LAT}" "${SMOKE_FROM_LON}" "${SMOKE_TO_LAT}" "${SMOKE_TO_LON}"; do
  [[ "${coordinate}" =~ ^-?[0-9]{1,3}([.][0-9]+)?$ ]] || {
    echo "routing smoke coordinates must be decimal numbers" >&2
    exit 1
  }
done
getent group "${DEPLOY_USER}" >/dev/null || {
  echo "the ${DEPLOY_USER} group does not exist; provision the deployment host first" >&2
  exit 1
}

declare -a sources=(
  "${ROOT}/docker-compose.nonprod.yml"
  "${ROOT}/deploy/docker-compose.routing.yml"
  "${ROOT}/deploy/runnarr-deploy"
)
for source in "${sources[@]}"; do
  [[ -f "${source}" && ! -L "${source}" ]] || {
    echo "required reviewed source is missing or is a symbolic link: ${source}" >&2
    exit 1
  }
done

install -d -m 0755 "${ASSET_ROOT}" "${ASSET_ROOT}/deploy" "${CONFIG_ROOT}"
install -d -m 0700 "${STATE_ROOT}/backups"
exec 9>"${STATE_ROOT}/deploy.lock"
flock 9

backup="${STATE_ROOT}/backups/preview-routing-$(date -u +%Y%m%dT%H%M%SZ)"
install -d -m 0700 "${backup}"
for target in \
  "${ASSET_ROOT}/docker-compose.nonprod.yml" \
  "${ROUTING_COMPOSE}" \
  "${ASSET_ROOT}/deploy/runnarr-deploy" \
  "${ROUTING_ENV}"; do
  if [[ -e "${target}" ]]; then
    [[ -f "${target}" && ! -L "${target}" ]] || {
      echo "refusing to replace non-regular target: ${target}" >&2
      exit 1
    }
    cp --preserve=mode,ownership,timestamps "${target}" "${backup}/"
  fi
done

temporary="$(mktemp "${CONFIG_ROOT}/preview-routing.env.XXXXXX")"
trap 'rm -f -- "${temporary}"' EXIT
cat > "${temporary}" <<EOF
RUNNARR_PREVIEW_ROUTING_ENABLED=true
RUNNARR_PREVIEW_ROUTING_CONTAINER=runnarr-nonprod-valhalla
RUNNARR_PREVIEW_ROUTING_SMOKE_FROM_LAT=${SMOKE_FROM_LAT}
RUNNARR_PREVIEW_ROUTING_SMOKE_FROM_LON=${SMOKE_FROM_LON}
RUNNARR_PREVIEW_ROUTING_SMOKE_TO_LAT=${SMOKE_TO_LAT}
RUNNARR_PREVIEW_ROUTING_SMOKE_TO_LON=${SMOKE_TO_LON}
RUNNARR_VALHALLA_IMAGE=${VALHALLA_IMAGE}
VALHALLA_TILE_URL='${TILE_URL}'
VALHALLA_BUILD_ELEVATION=False
VALHALLA_BUILD_ADMINS=True
VALHALLA_BUILD_TIME_ZONES=True
VALHALLA_SERVER_THREADS=2
RUNNARR_VALHALLA_CPU_LIMIT=2.0
RUNNARR_VALHALLA_MEMORY_LIMIT=6g
RUNNARR_VALHALLA_PIDS_LIMIT=512
EOF
chown root:"${DEPLOY_USER}" "${temporary}"
chmod 0640 "${temporary}"

install -o root -g root -m 0644 \
  "${ROOT}/deploy/docker-compose.routing.yml" \
  "${ROUTING_COMPOSE}"
docker compose \
  --project-name runnarr-preview-routing \
  --env-file "${temporary}" \
  --file "${ROUTING_COMPOSE}" \
  config --quiet
docker compose \
  --project-name runnarr-preview-routing \
  --env-file "${temporary}" \
  --file "${ROUTING_COMPOSE}" \
  up --detach

# Activate routing for subsequent preview deployments only after the shared
# service has been created successfully. Existing preview containers are not
# restarted by this installer.
mv "${temporary}" "${ROUTING_ENV}"
trap - EXIT
install -o root -g root -m 0644 \
  "${ROOT}/docker-compose.nonprod.yml" \
  "${ASSET_ROOT}/docker-compose.nonprod.yml"
install -o root -g root -m 0755 \
  "${ROOT}/deploy/runnarr-deploy" \
  "${ASSET_ROOT}/deploy/runnarr-deploy"

cat <<EOF
Shared preview Valhalla has been started.

Installed-asset backup: ${backup}
Routing configuration:  ${ROUTING_ENV}

Wait for the graph to build and the container to become healthy before
rerunning previews:
  docker compose --project-name runnarr-preview-routing \\
    --env-file ${ROUTING_ENV} \\
    --file ${ROUTING_COMPOSE} logs --follow valhalla
  docker inspect runnarr-nonprod-valhalla \\
    --format '{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{end}}'
EOF
