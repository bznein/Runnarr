#!/usr/bin/env bash

set -euo pipefail
umask 022

readonly DATA_ROOT=/data
readonly GRAPH_ROOT="${DATA_ROOT}/graph-cache"
readonly PBF_FILE="${DATA_ROOT}/region.osm.pbf"
readonly MARKER="${DATA_ROOT}/.runnarr-graphhopper-source"
readonly CONFIG=/runnarr/graphhopper.yml
readonly SOURCE_URL="${GRAPHHOPPER_PBF_URL:?GRAPHHOPPER_PBF_URL is required}"

[[ "${SOURCE_URL}" =~ ^https://[-A-Za-z0-9._~:/%+]+\.osm\.pbf([?][-A-Za-z0-9._~:/?\&=%+]*)?$ ]] || {
  echo "GRAPHHOPPER_PBF_URL must be an HTTPS .osm.pbf URL." >&2
  exit 1
}
[[ -f "${CONFIG}" && ! -L "${CONFIG}" ]] || {
  echo "GraphHopper configuration is missing or is a symbolic link: ${CONFIG}" >&2
  exit 1
}

config_hash="$(sha256sum "${CONFIG}" | awk '{print $1}')"
expected_marker="source=${SOURCE_URL}
config_sha256=${config_hash}"

if [[ -f "${MARKER}" ]]; then
  [[ "$(cat "${MARKER}")" == "${expected_marker}" && -d "${GRAPH_ROOT}" ]] || {
    echo "The GraphHopper graph source or configuration changed." >&2
    echo "Create a fresh graphhopper-data volume and retry; existing graph data was not modified." >&2
    exit 1
  }
elif [[ -d "${GRAPH_ROOT}" && -n "$(find "${GRAPH_ROOT}" -mindepth 1 -maxdepth 1 -print -quit)" ]]; then
  echo "GraphHopper graph data exists without a completed Runnarr import marker." >&2
  echo "Create a fresh graphhopper-data volume and retry; existing graph data was not modified." >&2
  exit 1
else
  temporary_pbf="${PBF_FILE}.download"
  trap 'rm -f -- "${temporary_pbf}"' EXIT
  rm -f -- "${temporary_pbf}"
  curl --fail --show-error --location --proto '=https' --proto-redir '=https' --output "${temporary_pbf}" "${SOURCE_URL}"
  [[ -s "${temporary_pbf}" ]] || {
    echo "Downloaded GraphHopper PBF is empty." >&2
    exit 1
  }
  mv "${temporary_pbf}" "${PBF_FILE}"
  /graphhopper/graphhopper.sh --import --input "${PBF_FILE}" --graph-cache "${GRAPH_ROOT}" --config "${CONFIG}"
  temporary_marker="${MARKER}.tmp"
  printf '%s\n' "${expected_marker}" > "${temporary_marker}"
  mv "${temporary_marker}" "${MARKER}"
  trap - EXIT
fi

exec /graphhopper/graphhopper.sh --input "${PBF_FILE}" --graph-cache "${GRAPH_ROOT}" --config "${CONFIG}" --host 0.0.0.0 --port 8989
