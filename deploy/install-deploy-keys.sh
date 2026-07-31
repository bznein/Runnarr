#!/usr/bin/env bash

set -euo pipefail
umask 077

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run this key installer as root." >&2
  exit 1
fi
if [[ "$#" -ne 2 || ! -f "$1" || ! -f "$2" ]]; then
  echo "usage: install-deploy-keys.sh NONPROD_PUBLIC_KEY PRODUCTION_PUBLIC_KEY" >&2
  exit 1
fi

TARGET="/srv/runnarr/.ssh/authorized_keys"
[[ ! -e "${TARGET}" ]] || {
  echo "refusing to replace existing ${TARGET}" >&2
  exit 1
}
id runnarr-deploy >/dev/null 2>&1 || {
  echo "runnarr-deploy account does not exist" >&2
  exit 1
}

read_public_key() {
  local path="$1"
  local key_type key_data
  read -r key_type key_data _ < "${path}"
  [[ "${key_type}" == "ssh-ed25519" && "${key_data}" =~ ^[A-Za-z0-9+/]+={0,2}$ ]] || {
    echo "invalid Ed25519 public key: ${path}" >&2
    exit 1
  }
  ssh-keygen -lf "${path}" >/dev/null
  printf '%s %s\n' "${key_type}" "${key_data}"
}

nonprod_key="$(read_public_key "$1")"
production_key="$(read_public_key "$2")"
[[ "${nonprod_key}" != "${production_key}" ]] || {
  echo "non-production and production must use different SSH keys" >&2
  exit 1
}

temporary="$(mktemp /tmp/runnarr-authorized-keys.XXXXXX)"
trap 'rm -f -- "${temporary}"' EXIT
printf '%s %s\n' \
  'restrict,command="env RUNNARR_DEPLOY_SCOPE=nonprod RUNNARR_DEPLOY_CONFIG=/etc/runnarr/deploy.conf /opt/runnarr-deploy/deploy/runnarr-deploy"' \
  "${nonprod_key}" > "${temporary}"
printf '%s %s\n' \
  'restrict,command="env RUNNARR_DEPLOY_SCOPE=production RUNNARR_DEPLOY_CONFIG=/etc/runnarr/deploy.conf /opt/runnarr-deploy/deploy/runnarr-deploy"' \
  "${production_key}" >> "${temporary}"

install -o runnarr-deploy -g runnarr-deploy -m 0600 "${temporary}" "${TARGET}"
echo "Installed two restricted deployment keys in ${TARGET}."
