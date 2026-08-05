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
  "${ROOT}/deploy/configure-preview-routing.sh" \
  "${ROOT}/deploy/configure-staging.sh" \
  "${ROOT}/deploy/configure-tunnel-ssh.sh" \
  "${ROOT}/deploy/install-deploy-keys.sh" \
  "${ROOT}/deploy/install-host.sh" \
  "${ROOT}/deploy/verify-staging-oidc.sh"

DIGEST="ghcr.io/bznein/runnarr@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

# Mirror the documented installed-asset check. The non-production override
# requires an ingress alias even though `docker compose config` starts nothing.
RUNNARR_INGRESS_ALIAS=runnarr-config-check \
  docker compose \
  --file "${ROOT}/docker-compose.yml" \
  --file "${ROOT}/docker-compose.nonprod.yml" \
  config --format json > "${TEMPORARY}/nonprod-config-check.json"
jq -e \
  '.services.db.image == "postgis/postgis:16-3.5-alpine"
   and (.services.app.networks.runnarr.aliases | index("runnarr-config-check")) != null' \
  "${TEMPORARY}/nonprod-config-check.json" >/dev/null
grep -Fq 'RUNNARR_INGRESS_ALIAS=runnarr-config-check' \
  "${ROOT}/docs/postgis-upgrade.md" ||
  fail "the documented non-production config check omits its synthetic ingress alias"

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
RUNNARR_ROUTING_ENABLED=true
RUNNARR_ROUTING_URL=http://valhalla:8002
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
  --arg database_image "postgis/postgis:16-3.5-alpine" \
  '.services.app.image == $digest
   and .services.db.image == $database_image
   and (.services.app.ports == null)
   and (.services.db.ports == null)
   and (.services.app.mem_limit | tonumber) == 536870912
   and (.services.db.mem_limit | tonumber) == 536870912
   and (.services.app.networks.runnarr.aliases | index("runnarr-pr-179")) != null
   and .services.app.environment.RUNNARR_ROUTING_ENABLED == "true"
   and .services.app.environment.RUNNARR_ROUTING_URL == "http://valhalla:8002"
   and ((.services | has("valhalla")) | not)
   and ((.services.app.networks | has("nonprod-ingress")) | not)
   and ((.services.db.networks | has("nonprod-ingress")) | not)
   and .networks.runnarr.ipam.config[0].subnet == "10.100.179.0/24"' \
  "${TEMPORARY}/preview.json" >/dev/null

grep -Fq 'docker network connect "${network}" "${NONPROD_GATEWAY}"' \
  "${ROOT}/deploy/runnarr-deploy" ||
  fail "the ingress gateway connection unexpectedly owns an alias"
grep -Fq 'docker network connect --alias valhalla "${network}" "${PREVIEW_ROUTING_CONTAINER}"' \
  "${ROOT}/deploy/runnarr-deploy" ||
  fail "the shared Valhalla alias is not attached to preview networks"
grep -Fq 'disconnect_preview_routing "previews/${pr}"' \
  "${ROOT}/deploy/runnarr-deploy" ||
  fail "preview teardown does not detach shared Valhalla"
grep -Fq -- '--env-file "${staging_directory}/image.env"' \
  "${ROOT}/deploy/runnarr-deploy" ||
  fail "the rollback compatibility check does not load staging image settings"
grep -Fq 'docker logs --tail=200 "${name}"' "${ROOT}/deploy/runnarr-deploy" ||
  fail "rollback compatibility failures do not preserve diagnostics"

cat > "${TEMPORARY}/routing.env" <<EOF
RUNNARR_PREVIEW_ROUTING_ENABLED=true
RUNNARR_PREVIEW_ROUTING_CONTAINER=runnarr-nonprod-valhalla
RUNNARR_PREVIEW_ROUTING_SMOKE_FROM_LAT=53.3438
RUNNARR_PREVIEW_ROUTING_SMOKE_FROM_LON=-6.2546
RUNNARR_PREVIEW_ROUTING_SMOKE_TO_LAT=53.3382
RUNNARR_PREVIEW_ROUTING_SMOKE_TO_LON=-6.2591
RUNNARR_VALHALLA_IMAGE=ghcr.io/valhalla/valhalla-scripted:3.6.3@sha256:e688a89f7a86880aabcc8b2eec9bcefdcb639d603ee90a928a6d3c7d92d58486
VALHALLA_TILE_URL='https://download.geofabrik.de/europe/ireland-and-northern-ireland-latest.osm.pbf'
VALHALLA_BUILD_ELEVATION=False
VALHALLA_BUILD_ADMINS=True
VALHALLA_BUILD_TIME_ZONES=True
VALHALLA_SERVER_THREADS=2
RUNNARR_VALHALLA_CPU_LIMIT=2.0
RUNNARR_VALHALLA_MEMORY_LIMIT=6g
RUNNARR_VALHALLA_PIDS_LIMIT=512
EOF
docker compose \
  --project-name runnarr-preview-routing \
  --env-file "${TEMPORARY}/routing.env" \
  --file "${ROOT}/deploy/docker-compose.routing.yml" \
  config --format json > "${TEMPORARY}/routing.json"
jq -e \
  '.services.valhalla.image == "ghcr.io/valhalla/valhalla-scripted:3.6.3@sha256:e688a89f7a86880aabcc8b2eec9bcefdcb639d603ee90a928a6d3c7d92d58486"
   and .services.valhalla.container_name == "runnarr-nonprod-valhalla"
   and (.services.valhalla.ports == null)
   and (.services.valhalla.mem_limit | tonumber) == 6442450944
   and .services.valhalla.labels["com.runnarr.environment"] == "preview-routing"
   and .volumes["valhalla-data"].name == "runnarr-nonprod-valhalla-data"' \
  "${TEMPORARY}/routing.json" >/dev/null

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
