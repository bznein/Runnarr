#!/usr/bin/env bash

die() {
  printf 'runnarr-deploy: %s\n' "$*" >&2
  exit 1
}

validate_commit() {
  [[ "${1:-}" =~ ^[0-9a-f]{40}$ ]]
}

validate_digest() {
  [[ "${1:-}" =~ ^ghcr\.io/bznein/runnarr@sha256:[0-9a-f]{64}$ ]]
}

validate_deployment_id() {
  [[ "${1:-}" =~ ^[0-9]{8}T[0-9]{6}Z-[0-9a-f]{12}$ ]]
}

validate_pr_number() {
  [[ "${1:-}" =~ ^[1-9][0-9]{0,8}$ ]]
}

validate_environment() {
  [[ "${1:-}" == "preview" || "${1:-}" == "staging" || "${1:-}" == "production" ]]
}

deployment_id() {
  local commit="$1"
  printf '%s-%s\n' "$(date -u +%Y%m%dT%H%M%SZ)" "${commit:0:12}"
}

write_deploy_config() {
  local target="$1"
  local deploy_root="$2"
  local asset_root="$3"
  local base_domain="$4"
  local nonprod_subnet="$5"
  local backup_recipient_line=""
  local staging_seed_username_line=""
  local production_url_line=""

  if [[ -e "${target}" ]]; then
    [[ -f "${target}" && ! -L "${target}" ]] ||
      die "refusing to replace non-regular deployment config: ${target}"
    local line
    while IFS= read -r line; do
      case "${line}" in
        RUNNARR_BACKUP_AGE_RECIPIENT=*) backup_recipient_line="${line}" ;;
        RUNNARR_STAGING_SEED_USERNAME=*) staging_seed_username_line="${line}" ;;
        RUNNARR_PRODUCTION_URL=*) production_url_line="${line}" ;;
      esac
    done < "${target}"
  fi

  local temporary
  temporary="$(mktemp "${target}.XXXXXX")"
  {
    printf 'RUNNARR_DEPLOY_ROOT=%s\n' "${deploy_root}"
    printf 'RUNNARR_DEPLOY_ASSETS=%s\n' "${asset_root}"
    printf 'RUNNARR_DEPLOY_BASE_DOMAIN=%s\n' "${base_domain}"
    printf 'RUNNARR_NONPROD_INGRESS_NETWORK=runnarr-nonprod-ingress\n'
    printf 'RUNNARR_NONPROD_INGRESS_SUBNET=%s\n' "${nonprod_subnet}"
    printf 'RUNNARR_MAX_PREVIEWS=10\n'
    printf 'RUNNARR_MIN_AVAILABLE_MEMORY_BYTES=12884901888\n'
    printf 'RUNNARR_MIN_FREE_DISK_BYTES=10737418240\n'
    printf 'RUNNARR_BACKUP_KEEP=3\n'
    [[ -z "${backup_recipient_line}" ]] || printf '%s\n' "${backup_recipient_line}"
    [[ -z "${staging_seed_username_line}" ]] || printf '%s\n' "${staging_seed_username_line}"
    [[ -z "${production_url_line}" ]] || printf '%s\n' "${production_url_line}"
  } > "${temporary}"
  chmod 0600 "${temporary}"
  mv "${temporary}" "${target}"
}

require_file() {
  [[ -f "$1" ]] || die "required file is missing: $1"
}

require_directory() {
  [[ -d "$1" ]] || die "required directory is missing: $1"
}

json_field() {
  local file="$1"
  local field="$2"
  jq -er "${field}" "${file}"
}

write_image_environment() {
  local target="$1"
  local image="$2"
  local environment="$3"
  local base_url="${4:-}"
  local ingress_alias="${5:-}"
  local preview_pr="${6:-}"
  local temporary

  validate_digest "${image}" || die "invalid immutable image: ${image}"
  validate_environment "${environment}" || die "invalid environment: ${environment}"
  temporary="$(mktemp "${target}.XXXXXX")"
  {
    printf 'RUNNARR_IMAGE=%s\n' "${image}"
    printf 'RUNNARR_DEPLOY_ENVIRONMENT=%s\n' "${environment}"
    if [[ -n "${base_url}" ]]; then
      printf 'RUNNARR_BASE_URL=%s\n' "${base_url}"
    fi
    if [[ -n "${ingress_alias}" ]]; then
      printf 'RUNNARR_INGRESS_ALIAS=%s\n' "${ingress_alias}"
    fi
    if [[ -n "${preview_pr}" ]]; then
      printf 'RUNNARR_PREVIEW_PR=%s\n' "${preview_pr}"
    fi
  } > "${temporary}"
  chmod 0600 "${temporary}"
  mv "${temporary}" "${target}"
}

atomic_json() {
  local target="$1"
  shift
  local temporary
  temporary="$(mktemp "${target}.XXXXXX")"
  jq -n "$@" > "${temporary}"
  chmod 0600 "${temporary}"
  mv "${temporary}" "${target}"
}
