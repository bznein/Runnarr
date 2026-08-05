#!/usr/bin/env bash

set -euo pipefail
umask 077

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run this installer as root." >&2
  exit 1
fi
if [[ "$#" -ne 3 ]]; then
  echo "usage: install-host.sh BASE_DOMAIN CLOUDFLARE_TUNNEL_ID TUNNEL_CREDENTIALS_JSON" >&2
  exit 1
fi

BASE_DOMAIN="$1"
TUNNEL_ID="$2"
TUNNEL_CREDENTIALS="$3"
[[ "${BASE_DOMAIN}" =~ ^[A-Za-z0-9.-]+$ && "${BASE_DOMAIN}" != .* && "${BASE_DOMAIN}" != *. ]] || {
  echo "invalid base domain" >&2
  exit 1
}
[[ "${TUNNEL_ID}" =~ ^[0-9a-fA-F-]{36}$ ]] || {
  echo "invalid Cloudflare tunnel id" >&2
  exit 1
}
[[ -f "${TUNNEL_CREDENTIALS}" ]] || {
  echo "tunnel credentials file does not exist" >&2
  exit 1
}

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ASSET_ROOT="/opt/runnarr-deploy"
CONFIG_ROOT="/etc/runnarr"
STATE_ROOT="/srv/runnarr"
DEPLOY_USER="runnarr-deploy"
DOMAIN_REGEX="$(printf '%s' "${BASE_DOMAIN}" | sed 's/[.]/\\\\./g')"
NONPROD_INGRESS_SUBNET="${RUNNARR_NONPROD_INGRESS_SUBNET:-10.90.0.0/24}"
[[ "${NONPROD_INGRESS_SUBNET}" =~ ^10\.[0-9]{1,3}\.[0-9]{1,3}\.0/24$ ]] || {
  echo "RUNNARR_NONPROD_INGRESS_SUBNET must be a private 10.x.x.0/24 subnet" >&2
  exit 1
}

getent group docker >/dev/null || {
  echo "the Docker group must exist before provisioning the deploy account" >&2
  exit 1
}
if ! id "${DEPLOY_USER}" >/dev/null 2>&1; then
  useradd --system --home-dir "${STATE_ROOT}" --shell /bin/bash --no-create-home "${DEPLOY_USER}"
fi
usermod -aG docker "${DEPLOY_USER}"

install -d -m 0755 "${ASSET_ROOT}" "${ASSET_ROOT}/deploy" "${CONFIG_ROOT}/ingress"
install -d -m 0700 \
  "${STATE_ROOT}" \
  "${STATE_ROOT}/backups" \
  "${STATE_ROOT}/environments" \
  "${STATE_ROOT}/config" \
  "${STATE_ROOT}/environments/previews" \
  "${STATE_ROOT}/environments/staging" \
  "${STATE_ROOT}/environments/production" \
  "${STATE_ROOT}/backups/production"

install -m 0644 \
  "${ROOT}/docker-compose.yml" \
  "${ROOT}/docker-compose.deploy.yml" \
  "${ROOT}/docker-compose.nonprod.yml" \
  "${ROOT}/docker-compose.public.yml" \
  "${ASSET_ROOT}/"
install -m 0644 "${ROOT}/web/e2e/seed.sql" "${ASSET_ROOT}/seed.sql"
install -m 0644 "${ROOT}/web/e2e/testbed-seed.sql" "${ASSET_ROOT}/testbed-seed.sql"
install -m 0755 "${ROOT}/deploy/runnarr-deploy" "${ASSET_ROOT}/deploy/runnarr-deploy"
install -m 0755 "${ROOT}/deploy/configure-ghcr-login.sh" "${ASSET_ROOT}/deploy/configure-ghcr-login.sh"
install -m 0755 "${ROOT}/deploy/configure-staging.sh" "${ASSET_ROOT}/deploy/configure-staging.sh"
install -m 0755 "${ROOT}/deploy/configure-tunnel-ssh.sh" "${ASSET_ROOT}/deploy/configure-tunnel-ssh.sh"
install -m 0755 "${ROOT}/deploy/install-deploy-keys.sh" "${ASSET_ROOT}/deploy/install-deploy-keys.sh"
install -m 0644 "${ROOT}/deploy/lib.sh" "${ASSET_ROOT}/deploy/lib.sh"
install -m 0644 "${ROOT}/deploy/docker-compose.ingress.yml" "${ASSET_ROOT}/docker-compose.ingress.yml"
install -m 0644 "${ROOT}/deploy/docker-compose.routing.yml" "${ASSET_ROOT}/docker-compose.routing.yml"
install -m 0755 "${ROOT}/deploy/configure-preview-routing.sh" "${ASSET_ROOT}/deploy/configure-preview-routing.sh"

sed \
  -e "s/__RUNNARR_BASE_DOMAIN__/${BASE_DOMAIN}/g" \
  -e "s/__RUNNARR_BASE_DOMAIN_REGEX__/${DOMAIN_REGEX}/g" \
  "${ROOT}/deploy/ingress/default.conf.template" > "${CONFIG_ROOT}/ingress/default.conf"
chmod 0644 "${CONFIG_ROOT}/ingress/default.conf"
sed \
  -e "s/__RUNNARR_BASE_DOMAIN__/${BASE_DOMAIN}/g" \
  -e "s/__RUNNARR_TUNNEL_ID__/${TUNNEL_ID}/g" \
  "${ROOT}/deploy/ingress/cloudflared.yml.template" > "${CONFIG_ROOT}/ingress/cloudflared.yml"
install -m 0600 "${TUNNEL_CREDENTIALS}" "${CONFIG_ROOT}/ingress/tunnel-credentials.json"

cat > "${CONFIG_ROOT}/deploy.conf" <<EOF
RUNNARR_DEPLOY_ROOT=${STATE_ROOT}
RUNNARR_DEPLOY_ASSETS=${ASSET_ROOT}
RUNNARR_DEPLOY_BASE_DOMAIN=${BASE_DOMAIN}
RUNNARR_NONPROD_INGRESS_NETWORK=runnarr-nonprod-ingress
RUNNARR_NONPROD_INGRESS_SUBNET=${NONPROD_INGRESS_SUBNET}
RUNNARR_MAX_PREVIEWS=10
RUNNARR_MIN_AVAILABLE_MEMORY_BYTES=12884901888
RUNNARR_MIN_FREE_DISK_BYTES=10737418240
RUNNARR_BACKUP_KEEP=3
EOF
chmod 0600 "${CONFIG_ROOT}/deploy.conf"
chgrp "${DEPLOY_USER}" "${CONFIG_ROOT}/deploy.conf"
chmod 0640 "${CONFIG_ROOT}/deploy.conf"

if ! docker network inspect runnarr-nonprod-ingress >/dev/null 2>&1; then
  docker network create \
    --subnet "${NONPROD_INGRESS_SUBNET}" \
    runnarr-nonprod-ingress >/dev/null
fi

cat > "${CONFIG_ROOT}/ingress/.env" <<EOF
RUNNARR_INGRESS_NGINX_CONFIG=${CONFIG_ROOT}/ingress/default.conf
RUNNARR_CLOUDFLARED_CONFIG=${CONFIG_ROOT}/ingress/cloudflared.yml
RUNNARR_TUNNEL_CREDENTIALS=${CONFIG_ROOT}/ingress/tunnel-credentials.json
RUNNARR_NONPROD_INGRESS_NETWORK=runnarr-nonprod-ingress
EOF
chmod 0600 "${CONFIG_ROOT}/ingress/.env"
chown -R "${DEPLOY_USER}:${DEPLOY_USER}" "${STATE_ROOT}"
install -d -o "${DEPLOY_USER}" -g "${DEPLOY_USER}" -m 0700 "${STATE_ROOT}/.ssh"

cat <<EOF
Host assets installed.

Still required before starting the ingress or enabling Actions:
  1. Create ${STATE_ROOT}/config/preview.env with RUNNARR_PREVIEW_ADMIN_PASSWORD_HASH.
  2. Create root-owned staging/base.env and production/base.env files.
  3. Create production/image.env with the currently deployed local RUNNARR_IMAGE.
  4. Configure age, RUNNARR_BACKUP_AGE_RECIPIENT, and RUNNARR_PRODUCTION_URL in deploy.conf.
  5. Install separate forced SSH keys for nonprod and production in ${STATE_ROOT}/.ssh/authorized_keys.
  6. Configure the deploy account's read-only GHCR login.
  7. Configure Cloudflare Access and exact DNS records, including
     runnarr-deploy.${BASE_DOMAIN} for passwordless deployment SSH.
  8. Optionally run deploy/configure-preview-routing.sh to enable one shared
     Valhalla graph for isolated pull-request previews.

Start ingress only after those steps:
  docker compose --env-file ${CONFIG_ROOT}/ingress/.env \\
    -f ${ASSET_ROOT}/docker-compose.ingress.yml up -d
EOF
