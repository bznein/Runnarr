#!/usr/bin/env bash

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEMPORARY="$(mktemp -d "${TMPDIR:-/tmp}/runnarr-deploy-test.XXXXXX")"
trap 'rm -rf -- "${TEMPORARY}"' EXIT

# shellcheck source=deploy/lib.sh
source "${ROOT}/deploy/lib.sh"

fail() {
  echo "deployment test: $*" >&2
  exit 1
}

validate_commit "0123456789abcdef0123456789abcdef01234567" || fail "valid commit rejected"
! validate_commit "main" || fail "invalid commit accepted"
validate_digest "ghcr.io/bznein/runnarr@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" ||
  fail "valid digest rejected"
! validate_digest "ghcr.io/example/runnarr:latest" || fail "mutable image accepted"
validate_pr_number "179" || fail "valid PR rejected"
! validate_pr_number "0" || fail "invalid PR accepted"
validate_deployment_id "20260731T120000Z-0123456789ab" || fail "valid deployment id rejected"

bash -n \
  "${ROOT}/deploy/lib.sh" \
  "${ROOT}/deploy/runnarr-deploy" \
  "${ROOT}/deploy/configure-ghcr-login.sh" \
  "${ROOT}/deploy/configure-staging.sh" \
  "${ROOT}/deploy/configure-tunnel-ssh.sh" \
  "${ROOT}/deploy/install-deploy-keys.sh" \
  "${ROOT}/deploy/install-host.sh" \
  "${ROOT}/deploy/verify-staging-oidc.sh"

DIGEST="ghcr.io/bznein/runnarr@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
cat > "${TEMPORARY}/base.env" <<'EOF'
POSTGRES_USER=runnarr
POSTGRES_PASSWORD=test-password
POSTGRES_DB=runnarr
DATABASE_URL=postgres://runnarr:test-password@db:5432/runnarr?sslmode=disable
RUNNARR_ADMIN_USERNAME=admin
RUNNARR_ADMIN_PASSWORD=test-password
RUNNARR_SECRET_KEY=0123456789abcdef0123456789abcdef
RUNNARR_PUBLIC_MODE=false
RUNNARR_LOCAL_AUTH_ENABLED=true
RUNNARR_NONPROD_INGRESS_NETWORK=runnarr-nonprod-ingress
RUNNARR_APP_CPU_LIMIT=0.5
RUNNARR_APP_MEMORY_LIMIT=512m
RUNNARR_DB_CPU_LIMIT=0.5
RUNNARR_DB_MEMORY_LIMIT=512m
RUNNARR_NETWORK_SUBNET=10.100.179.0/24
EOF
cat > "${TEMPORARY}/preview-image.env" <<EOF
RUNNARR_IMAGE=${DIGEST}
RUNNARR_DEPLOY_ENVIRONMENT=preview
RUNNARR_PREVIEW_PR=179
RUNNARR_BASE_URL=https://runnarr-pr-179.example.com
RUNNARR_INGRESS_ALIAS=runnarr-pr-179
EOF

docker compose \
  --project-name runnarr-pr-179 \
  --env-file "${TEMPORARY}/base.env" \
  --env-file "${TEMPORARY}/preview-image.env" \
  --file "${ROOT}/docker-compose.yml" \
  --file "${ROOT}/docker-compose.deploy.yml" \
  --file "${ROOT}/docker-compose.nonprod.yml" \
  config --format json > "${TEMPORARY}/preview.json"

jq -e \
  --arg digest "${DIGEST}" \
  '.services.app.image == $digest
   and (.services.app.ports == null)
   and (.services.db.ports == null)
   and (.services.app.mem_limit | tonumber) == 536870912
   and (.services.db.mem_limit | tonumber) == 536870912
   and (.services.app.networks.runnarr.aliases | index("runnarr-pr-179")) != null
   and ((.services.app.networks | has("nonprod-ingress")) | not)
   and ((.services.db.networks | has("nonprod-ingress")) | not)
   and .networks.runnarr.ipam.config[0].subnet == "10.100.179.0/24"' \
  "${TEMPORARY}/preview.json" >/dev/null

! grep -Fq 'docker network connect --alias' "${ROOT}/deploy/runnarr-deploy" ||
  fail "the ingress alias must belong to the app, not the gateway"

cat > "${TEMPORARY}/production.env" <<'EOF'
POSTGRES_USER=runnarr
POSTGRES_PASSWORD=production-password
POSTGRES_DB=runnarr
DATABASE_URL=postgres://runnarr:production-password@db:5432/runnarr?sslmode=disable
RUNNARR_ADMIN_USERNAME=admin
RUNNARR_ADMIN_PASSWORD_HASH=deployment-test-hash
RUNNARR_SECRET_KEY=0123456789abcdef0123456789abcdef
RUNNARR_BASE_URL=https://runnarr.example.com
RUNNARR_PUBLIC_MODE=true
RUNNARR_LOCAL_AUTH_ENABLED=false
RUNNARR_OIDC_GOOGLE_CLIENT_ID=client
RUNNARR_OIDC_GOOGLE_CLIENT_SECRET=secret
RUNNARR_OIDC_ALLOWED_EMAILS=person@example.com=admin
RUNNARR_PROXY_NETWORK=proxy
RUNNARR_NETWORK_SUBNET=10.89.0.0/24
EOF
cat > "${TEMPORARY}/production-image.env" <<EOF
RUNNARR_IMAGE=${DIGEST}
RUNNARR_DEPLOY_ENVIRONMENT=production
EOF

docker compose \
  --project-name runnarr \
  --env-file "${TEMPORARY}/production.env" \
  --env-file "${TEMPORARY}/production-image.env" \
  --file "${ROOT}/docker-compose.yml" \
  --file "${ROOT}/docker-compose.deploy.yml" \
  --file "${ROOT}/docker-compose.public.yml" \
  config --format json > "${TEMPORARY}/production.json"

jq -e \
  --arg digest "${DIGEST}" \
  '.services.app.image == $digest
   and (.services.app.ports == null)
   and (.services.db.ports == null)
   and (.services.app.networks | has("proxy"))
   and ((.services.db.networks | has("proxy")) | not)
   and .networks.runnarr.ipam.config[0].subnet == "10.89.0.0/24"' \
  "${TEMPORARY}/production.json" >/dev/null

DOMAIN_REGEX='example\\.com'
sed \
  -e 's/__RUNNARR_BASE_DOMAIN__/example.com/g' \
  -e "s/__RUNNARR_BASE_DOMAIN_REGEX__/${DOMAIN_REGEX}/g" \
  "${ROOT}/deploy/ingress/default.conf.template" > "${TEMPORARY}/default.conf"
! grep -q '__RUNNARR_' "${TEMPORARY}/default.conf" || fail "ingress template was not fully rendered"
# The literal nginx variable must survive template rendering.
# shellcheck disable=SC2016
grep -Fq 'runnarr-pr-${preview_id}:8080' "${TEMPORARY}/default.conf" ||
  fail "preview ingress does not use the isolated project alias"
grep -Fq 'hostname: runnarr-deploy.__RUNNARR_BASE_DOMAIN__' \
  "${ROOT}/deploy/ingress/cloudflared.yml.template" ||
  fail "deployment SSH is not routed through the tunnel"
grep -Fq 'service: ssh://host.docker.internal:22' \
  "${ROOT}/deploy/ingress/cloudflared.yml.template" ||
  fail "deployment SSH tunnel does not target the host"
grep -Fq 'host.docker.internal:host-gateway' \
  "${ROOT}/deploy/docker-compose.ingress.yml" ||
  fail "cloudflared cannot resolve the host SSH endpoint"
grep -Fq 'user: "101:101"' "${ROOT}/deploy/docker-compose.ingress.yml" ||
  fail "the ingress gateway is not pinned to its unprivileged account"
grep -Fq 'user: "0:0"' "${ROOT}/deploy/docker-compose.ingress.yml" ||
  fail "cloudflared cannot read its root-owned credential mount"
grep -Fq '/var/cache/nginx:uid=101,gid=101,mode=0750' \
  "${ROOT}/deploy/docker-compose.ingress.yml" ||
  fail "the ingress gateway cache is not privately writable"
# The literal shell variable must remain in the installer assertion.
# shellcheck disable=SC2016
grep -Fq 'chmod 0644 "${CONFIG_ROOT}/ingress/default.conf"' \
  "${ROOT}/deploy/install-host.sh" ||
  fail "the unprivileged gateway cannot read its non-secret routing config"
[[ -x "${ROOT}/deploy/verify-staging-oidc.sh" ]] ||
  fail "the staging OIDC verifier is not executable"

echo "deployment configuration checks passed"
