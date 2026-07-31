#!/usr/bin/env bash

set -euo pipefail
umask 077

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run this configurator as root." >&2
  exit 1
fi
if [[ "$#" -ne 1 || ! -f "$1" ]]; then
  echo "usage: configure-staging.sh STAGING_HASH_FILE" >&2
  exit 1
fi

TARGET="/srv/runnarr/environments/staging/base.env"
[[ ! -e "${TARGET}" ]] || {
  echo "refusing to replace existing staging configuration: ${TARGET}" >&2
  exit 1
}
id runnarr-deploy >/dev/null 2>&1 || {
  echo "runnarr-deploy account does not exist" >&2
  exit 1
}

hash_line="$(sed -n '/[^[:space:]]/{s/\r$//;p;q;}' "$1")"
prefix="RUNNARR_ADMIN_PASSWORD_HASH='"
if [[ "${hash_line}" == "${prefix}"*"'" ]]; then
  password_hash="${hash_line#"${prefix}"}"
  password_hash="${password_hash%"'"}"
else
  password_hash="${hash_line}"
fi
[[ "${password_hash}" =~ ^\$2[aby]\$12\$[./A-Za-z0-9]{53}$ ]] || {
  echo "staging password is not a bcrypt cost-12 hash" >&2
  exit 1
}

database_password="$(openssl rand -hex 24)"
secret_key="$(openssl rand -hex 32)"
temporary="$(mktemp /tmp/runnarr-staging-base.XXXXXX)"
trap 'rm -f -- "${temporary}"' EXIT

{
  printf 'POSTGRES_USER=runnarr\n'
  printf 'POSTGRES_PASSWORD=%s\n' "${database_password}"
  printf 'POSTGRES_DB=runnarr\n'
  printf 'DATABASE_URL=postgres://runnarr:%s@db:5432/runnarr?sslmode=disable\n' "${database_password}"
  printf 'RUNNARR_PUBLIC_MODE=true\n'
  printf 'RUNNARR_LOCAL_AUTH_ENABLED=true\n'
  printf 'RUNNARR_TRUST_PROXY=true\n'
  printf 'RUNNARR_ADMIN_USERNAME=staging-admin\n'
  printf "RUNNARR_ADMIN_PASSWORD_HASH='%s'\n" "${password_hash}"
  printf 'RUNNARR_SECRET_KEY=%s\n' "${secret_key}"
  printf 'RUNNARR_OIDC_GOOGLE_CLIENT_ID=\n'
  printf 'RUNNARR_OIDC_GOOGLE_CLIENT_SECRET=\n'
  printf 'RUNNARR_OIDC_ALLOWED_EMAILS=\n'
  printf 'RUNNARR_GOOGLE_CLIENT_ID=\n'
  printf 'RUNNARR_GOOGLE_CLIENT_SECRET=\n'
  printf 'RUNNARR_APP_CPU_LIMIT=1.0\n'
  printf 'RUNNARR_APP_MEMORY_LIMIT=1g\n'
  printf 'RUNNARR_DB_CPU_LIMIT=1.0\n'
  printf 'RUNNARR_DB_MEMORY_LIMIT=2g\n'
  printf 'RUNNARR_NETWORK_SUBNET=10.91.0.0/24\n'
  printf 'RUNNARR_NONPROD_INGRESS_NETWORK=runnarr-nonprod-ingress\n'
} > "${temporary}"

install -o root -g runnarr-deploy -m 0640 "${temporary}" "${TARGET}"
echo "Installed ${TARGET}; staging remains stopped."
