#!/usr/bin/env bash

set -euo pipefail
umask 077

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run this configurator as root." >&2
  exit 1
fi
if [[ "$#" -ne 0 ]]; then
  echo "usage: configure-tunnel-ssh.sh" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ASSET_ROOT="/opt/runnarr-deploy"
CONFIG_ROOT="/etc/runnarr"
DEPLOY_CONFIG="${CONFIG_ROOT}/deploy.conf"
TUNNEL_CONFIG="${CONFIG_ROOT}/ingress/cloudflared.yml"
GATEWAY_CONFIG="${CONFIG_ROOT}/ingress/default.conf"
COMPOSE_ASSET="${ASSET_ROOT}/docker-compose.ingress.yml"

for required in \
  "${DEPLOY_CONFIG}" \
  "${GATEWAY_CONFIG}" \
  "${TUNNEL_CONFIG}" \
  "${ROOT}/deploy/docker-compose.ingress.yml" \
  "${ROOT}/deploy/ingress/cloudflared.yml.template"; do
  [[ -f "${required}" ]] || {
    echo "required file is missing: ${required}" >&2
    exit 1
  }
done

if docker inspect --format '{{.State.Running}}' runnarr-nonprod-cloudflared \
  2>/dev/null | grep -qx true; then
  echo "Refusing to replace the configuration while ingress is running." >&2
  exit 1
fi

BASE_DOMAIN="$(sed -n 's/^RUNNARR_DEPLOY_BASE_DOMAIN=//p' "${DEPLOY_CONFIG}")"
TUNNEL_ID="$(sed -n 's/^tunnel:[[:space:]]*//p' "${TUNNEL_CONFIG}")"
[[ "${BASE_DOMAIN}" =~ ^[A-Za-z0-9.-]+$ && "${BASE_DOMAIN}" != .* && "${BASE_DOMAIN}" != *. ]] || {
  echo "deploy.conf contains an invalid base domain" >&2
  exit 1
}
[[ "${TUNNEL_ID}" =~ ^[0-9a-fA-F-]{36}$ ]] || {
  echo "the installed tunnel configuration has an invalid tunnel id" >&2
  exit 1
}
ss -ltn | grep -Eq '(^|[[:space:]])[^[:space:]]*:22[[:space:]]' || {
  echo "the host SSH service is not listening on port 22" >&2
  exit 1
}

RENDERED="$(mktemp "${TMPDIR:-/tmp}/runnarr-cloudflared.XXXXXX")"
trap 'rm -f -- "${RENDERED}"' EXIT
sed \
  -e "s/__RUNNARR_BASE_DOMAIN__/${BASE_DOMAIN}/g" \
  -e "s/__RUNNARR_TUNNEL_ID__/${TUNNEL_ID}/g" \
  "${ROOT}/deploy/ingress/cloudflared.yml.template" > "${RENDERED}"
! grep -q '__RUNNARR_' "${RENDERED}" || {
  echo "the tunnel template was not fully rendered" >&2
  exit 1
}
grep -Fq "hostname: runnarr-deploy.${BASE_DOMAIN}" "${RENDERED}"
grep -Fq 'service: ssh://host.docker.internal:22' "${RENDERED}"
grep -Fq 'host.docker.internal:host-gateway' \
  "${ROOT}/deploy/docker-compose.ingress.yml"

if [[ ! -e "${TUNNEL_CONFIG}.before-deploy-ssh" ]]; then
  install -m 0600 "${TUNNEL_CONFIG}" "${TUNNEL_CONFIG}.before-deploy-ssh"
fi
if [[ -f "${COMPOSE_ASSET}" && ! -e "${COMPOSE_ASSET}.before-deploy-ssh" ]]; then
  install -m 0644 "${COMPOSE_ASSET}" "${COMPOSE_ASSET}.before-deploy-ssh"
fi
install -m 0644 "${ROOT}/deploy/docker-compose.ingress.yml" "${COMPOSE_ASSET}"
install -m 0600 "${RENDERED}" "${TUNNEL_CONFIG}"
chmod 0644 "${GATEWAY_CONFIG}"

docker compose \
  --env-file "${CONFIG_ROOT}/ingress/.env" \
  --file "${COMPOSE_ASSET}" \
  config >/dev/null

echo "Installed the passwordless deployment SSH tunnel route; ingress remains stopped."
