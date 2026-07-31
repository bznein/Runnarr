#!/usr/bin/env bash

set -euo pipefail

config="/srv/runnarr/environments/staging/base.env"

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run this verifier as root." >&2
  exit 1
fi
if [[ ! -f "${config}" ]]; then
  echo "staging configuration not found: ${config}" >&2
  exit 1
fi

read_value() {
  local key="$1"
  local line
  local value=""

  while IFS= read -r line; do
    [[ "${line}" == "${key}="* ]] || continue
    value="${line#*=}"
  done < "${config}"

  value="${value%$'\r'}"
  if [[ "${value}" == \'*\' && "${value}" == *\' ]]; then
    value="${value:1:${#value}-2}"
  elif [[ "${value}" == \"*\" && "${value}" == *\" ]]; then
    value="${value:1:${#value}-2}"
  fi
  if [[ -z "${value}" ]]; then
    echo "${key} is empty" >&2
    return 1
  fi
  REPLY="${value}"
}

read_value RUNNARR_OIDC_GOOGLE_CLIENT_ID
client_id="${REPLY}"
read_value RUNNARR_OIDC_GOOGLE_CLIENT_SECRET
read_value RUNNARR_OIDC_ALLOWED_EMAILS
allowed_emails="${REPLY}"

if [[ "${client_id}" != *.apps.googleusercontent.com ]]; then
  echo "RUNNARR_OIDC_GOOGLE_CLIENT_ID has an unexpected format" >&2
  exit 1
fi
if [[ "${allowed_emails}" != *"=staging-admin"* ]]; then
  echo "RUNNARR_OIDC_ALLOWED_EMAILS must map a Google email to staging-admin" >&2
  exit 1
fi

echo "Staging OIDC configuration is populated."
