#!/usr/bin/env bash

set -euo pipefail
umask 077

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run this installer as root." >&2
  exit 1
fi
if [[ "$#" -ne 0 && "$#" -ne 1 && "$#" -ne 5 ]]; then
  echo "usage: configure-preview-routing.sh [GRAPHHOPPER_PBF_URL [FROM_LAT FROM_LON TO_LAT TO_LON]]" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ASSET_ROOT="/opt/runnarr-deploy"
CONFIG_ROOT="/etc/runnarr"
STATE_ROOT="/srv/runnarr"
DEPLOY_USER="runnarr-deploy"
ROUTING_ENV="${CONFIG_ROOT}/preview-routing.env"
ROUTING_COMPOSE="${ASSET_ROOT}/docker-compose.routing.yml"
PBF_URL="${1:-https://download.geofabrik.de/europe/ireland-and-northern-ireland-latest.osm.pbf}"
SMOKE_FROM_LAT="${2:-53.3438}"
SMOKE_FROM_LON="${3:--6.2546}"
SMOKE_TO_LAT="${4:-53.3382}"
SMOKE_TO_LON="${5:--6.2591}"
GRAPHHOPPER_IMAGE="israelhikingmap/graphhopper:11.0@sha256:e77e14e48ea69ea7bb0eb71ddc9d583e5ce85dd295475572371f72ed4880a1ff"

[[ "${PBF_URL}" =~ ^https://[-A-Za-z0-9._~:/%+]+\.osm\.pbf([?][-A-Za-z0-9._~:/?\&=%+]*)?$ ]] || {
  echo "GRAPHHOPPER_PBF_URL must be an HTTPS .osm.pbf URL." >&2
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
  "${ROOT}/deploy/graphhopper.yml"
  "${ROOT}/deploy/graphhopper-entrypoint.sh"
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
  "${ASSET_ROOT}/deploy/graphhopper.yml" \
  "${ASSET_ROOT}/deploy/graphhopper-entrypoint.sh" \
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
RUNNARR_PREVIEW_ROUTING_CONTAINER=runnarr-nonprod-graphhopper
RUNNARR_PREVIEW_ROUTING_SUBNET=10.92.0.0/24
RUNNARR_PREVIEW_ROUTING_SMOKE_FROM_LAT=${SMOKE_FROM_LAT}
RUNNARR_PREVIEW_ROUTING_SMOKE_FROM_LON=${SMOKE_FROM_LON}
RUNNARR_PREVIEW_ROUTING_SMOKE_TO_LAT=${SMOKE_TO_LAT}
RUNNARR_PREVIEW_ROUTING_SMOKE_TO_LON=${SMOKE_TO_LON}
RUNNARR_GRAPHHOPPER_IMAGE=${GRAPHHOPPER_IMAGE}
GRAPHHOPPER_PBF_URL='${PBF_URL}'
GRAPHHOPPER_JAVA_OPTS='-Xms1g -Xmx4g'
RUNNARR_GRAPHHOPPER_CPU_LIMIT=2.0
RUNNARR_GRAPHHOPPER_MEMORY_LIMIT=6g
RUNNARR_GRAPHHOPPER_PIDS_LIMIT=512
EOF
chown root:"${DEPLOY_USER}" "${temporary}"
chmod 0640 "${temporary}"

install -o root -g root -m 0644 "${ROOT}/deploy/docker-compose.routing.yml" "${ROUTING_COMPOSE}"
install -o root -g root -m 0644 "${ROOT}/deploy/graphhopper.yml" "${ASSET_ROOT}/deploy/graphhopper.yml"
install -o root -g root -m 0755 "${ROOT}/deploy/graphhopper-entrypoint.sh" "${ASSET_ROOT}/deploy/graphhopper-entrypoint.sh"
docker compose --project-name runnarr-preview-routing --env-file "${temporary}" --file "${ROUTING_COMPOSE}" config --quiet
docker compose --project-name runnarr-preview-routing --env-file "${temporary}" --file "${ROUTING_COMPOSE}" up --detach graphhopper

for _attempt in $(seq 1 180); do
  state="$(docker inspect runnarr-nonprod-graphhopper --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}')"
  [[ "${state}" != "healthy" ]] || break
  [[ "${state}" != "unhealthy" ]] || {
    docker logs --tail=200 runnarr-nonprod-graphhopper >&2 || true
    echo "Shared preview GraphHopper became unhealthy." >&2
    exit 1
  }
  sleep 5
done
[[ "${state:-missing}" == "healthy" ]] || {
  docker logs --tail=200 runnarr-nonprod-graphhopper >&2 || true
  echo "Shared preview GraphHopper did not become healthy." >&2
  exit 1
}
info="$(docker exec runnarr-nonprod-graphhopper curl -fsS http://127.0.0.1:8989/info)"
jq -e '.elevation == true and any(.profiles[]; .name == "foot") and any(.profiles[]; .name == "bike")' <<< "${info}" >/dev/null || {
  echo "GraphHopper does not expose the required elevation-enabled foot and bike profiles." >&2
  exit 1
}
route_payload="$(jq -cn --argjson fromLat "${SMOKE_FROM_LAT}" --argjson fromLon "${SMOKE_FROM_LON}" --argjson toLat "${SMOKE_TO_LAT}" --argjson toLon "${SMOKE_TO_LON}" '{points:[[$fromLon,$fromLat],[$toLon,$toLat]],profile:"foot",points_encoded:false,elevation:true,instructions:false}')"
route_result="$(docker exec -i runnarr-nonprod-graphhopper curl -fsS -H 'Content-Type: application/json' --data-binary @- http://127.0.0.1:8989/route <<< "${route_payload}")"
jq -e '.paths[0].points.coordinates | length >= 2 and all(.[]; length >= 3)' <<< "${route_result}" >/dev/null || {
  echo "GraphHopper route smoke check did not return 3D geometry." >&2
  exit 1
}

# Existing preview app containers are not restarted. Only future deployments
# consume this environment after GraphHopper has passed its smoke checks.
mv "${temporary}" "${ROUTING_ENV}"
trap - EXIT
install -o root -g root -m 0644 "${ROOT}/docker-compose.nonprod.yml" "${ASSET_ROOT}/docker-compose.nonprod.yml"
install -o root -g root -m 0755 "${ROOT}/deploy/runnarr-deploy" "${ASSET_ROOT}/deploy/runnarr-deploy"

cat <<EOF
Shared preview GraphHopper is healthy and ready.

Installed-asset backup: ${backup}
Routing configuration:  ${ROUTING_ENV}

Inspect it with:
  docker compose --project-name runnarr-preview-routing \\
    --env-file ${ROUTING_ENV} \\
    --file ${ROUTING_COMPOSE} logs --follow graphhopper
  docker inspect runnarr-nonprod-graphhopper \\
    --format '{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{end}}'
EOF
