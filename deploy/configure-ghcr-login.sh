#!/usr/bin/env bash

set -euo pipefail
umask 077

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run this configurator as root." >&2
  exit 1
fi
if [[ "$#" -ne 1 ]]; then
  echo "usage: configure-ghcr-login.sh GITHUB_USERNAME" >&2
  exit 1
fi

GITHUB_USERNAME="$1"
DEPLOY_USER="runnarr-deploy"
DEPLOY_HOME="/srv/runnarr"
DOCKER_CONFIG="${DEPLOY_HOME}/.docker"
[[ "${GITHUB_USERNAME}" =~ ^[A-Za-z0-9]([A-Za-z0-9-]{0,37}[A-Za-z0-9])?$ ]] || {
  echo "invalid GitHub username" >&2
  exit 1
}
id "${DEPLOY_USER}" >/dev/null 2>&1 || {
  echo "the ${DEPLOY_USER} account does not exist" >&2
  exit 1
}

read -rsp "GitHub read:packages token: " token
echo
[[ -n "${token}" && "${token}" != *$'\n'* ]] || {
  unset token
  echo "the token must contain exactly one non-empty line" >&2
  exit 1
}

install -d -o "${DEPLOY_USER}" -g "${DEPLOY_USER}" -m 0700 "${DOCKER_CONFIG}"
if ! printf '%s' "${token}" | runuser --user "${DEPLOY_USER}" -- \
  env HOME="${DEPLOY_HOME}" DOCKER_CONFIG="${DOCKER_CONFIG}" \
  docker login ghcr.io --username "${GITHUB_USERNAME}" --password-stdin; then
  unset token
  echo "GHCR login failed" >&2
  exit 1
fi
unset token

CONFIG_FILE="${DOCKER_CONFIG}/config.json"
[[ -f "${CONFIG_FILE}" ]] || {
  echo "Docker did not create ${CONFIG_FILE}" >&2
  exit 1
}
chown "${DEPLOY_USER}:${DEPLOY_USER}" "${CONFIG_FILE}"
chmod 0600 "${CONFIG_FILE}"
jq -e '.auths["ghcr.io"].auth | type == "string" and length > 0' \
  "${CONFIG_FILE}" >/dev/null || {
  echo "the Docker config does not contain a GHCR credential" >&2
  exit 1
}

echo "Installed the read-only GHCR login for ${DEPLOY_USER}."
