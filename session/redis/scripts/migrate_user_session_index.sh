#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  migrate_user_session_index.sh [options] -- <redis-cli options>

Options:
  --key-prefix PREFIX   Redis key prefix configured by redis.WithKeyPrefix.
  --all-prefixes        Scan all prefixed and unprefixed hashidx session meta keys.
  --scan-count N        SCAN count hint. Default: 1000.
  --scan-sleep SECONDS  Sleep interval after each SCAN call. Default: 0.01.
  --apply               Write missing hashidx user session index entries.
  --check-zset          Detect legacy zset session keys: [prefix:]sess:{app}:user.
  --fail-on-zset        Exit non-zero if legacy zset sessions are found.
  --lua PATH            Lua script path. Default: sibling migrate_user_session_index.lua.
  -h, --help            Show this help.

Examples:
  # Dry-run standalone Redis.
  ./migrate_user_session_index.sh --check-zset -- -u 'redis://localhost:6379/0'

  # Apply migration with key prefix.
  ./migrate_user_session_index.sh --key-prefix myapp --apply -- -h 127.0.0.1 -p 6379

  # Redis Cluster: pass -c and run this script against each master endpoint.
  ./migrate_user_session_index.sh --apply --check-zset -- -c -h 10.0.0.1 -p 6379
USAGE
}

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
lua_script="${script_dir}/migrate_user_session_index.lua"
key_prefix=""
all_prefixes=0
scan_count=1000
scan_sleep=0.01
apply=0
check_zset=0
fail_on_zset=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --key-prefix)
      key_prefix="${2:-}"
      shift 2
      ;;
    --all-prefixes)
      all_prefixes=1
      shift
      ;;
    --scan-count)
      scan_count="${2:-}"
      shift 2
      ;;
    --scan-sleep)
      scan_sleep="${2:-}"
      shift 2
      ;;
    --apply)
      apply=1
      shift
      ;;
    --check-zset)
      check_zset=1
      shift
      ;;
    --fail-on-zset)
      check_zset=1
      fail_on_zset=1
      shift
      ;;
    --lua)
      lua_script="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      break
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ $# -eq 0 ]]; then
  echo "redis-cli options are required after --" >&2
  usage >&2
  exit 2
fi

if [[ ! -f "${lua_script}" ]]; then
  echo "lua script not found: ${lua_script}" >&2
  exit 2
fi

prefix="${key_prefix%:}"
if [[ -n "${prefix}" ]]; then
  prefix="${prefix}:"
fi

if [[ "${all_prefixes}" -eq 1 && -n "${prefix}" ]]; then
  echo "--all-prefixes and --key-prefix cannot be used together" >&2
  exit 2
fi

if ! [[ "${scan_count}" =~ ^[1-9][0-9]*$ ]]; then
  echo "--scan-count must be a positive integer" >&2
  exit 2
fi
if ! [[ "${scan_sleep}" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
  echo "--scan-sleep must be a non-negative number" >&2
  exit 2
fi

redis_cli=(redis-cli "$@")
if [[ "${all_prefixes}" -eq 1 ]]; then
  meta_patterns=("*:hashidx:meta:*" "hashidx:meta:*")
  zset_patterns=("*:sess:{*}:*" "sess:{*}:*")
else
  meta_patterns=("${prefix}hashidx:meta:*")
  zset_patterns=("${prefix}sess:{*}:*")
fi

meta_scanned=0
missing_index=0
added_index=0
existing_index=0
invalid_meta=0
zset_keys=0
zset_sessions=0

index_key_from_meta_key() {
  local meta_key="$1"
  local marker="hashidx:meta:"
  if [[ "${meta_key}" != *"${marker}"* ]]; then
    return 1
  fi

  local key_prefix="${meta_key%%${marker}*}"
  local rest="${meta_key#*${marker}}"
  if [[ ! "${rest}" =~ ^(.+):\{([^}]*)\}:(.+)$ ]]; then
    return 1
  fi

  local app_name="${BASH_REMATCH[1]}"
  local user_id="${BASH_REMATCH[2]}"
  if [[ -z "${app_name}" || -z "${user_id}" ]]; then
    return 1
  fi

  printf '%s' "${key_prefix}hashidx:sessidx:${app_name}:{${user_id}}"
}

scan_pattern() {
  local pattern="$1"
  local callback="$2"
  local cursor=0
  local response
  local next_cursor
  local key

  while true; do
    response="$("${redis_cli[@]}" --raw SCAN "${cursor}" MATCH "${pattern}" COUNT "${scan_count}")"
    next_cursor="$(printf '%s\n' "${response}" | sed -n '1p')"
    if ! [[ "${next_cursor}" =~ ^[0-9]+$ ]]; then
      echo "invalid SCAN response for pattern ${pattern}: ${response}" >&2
      exit 1
    fi

    while IFS= read -r key; do
      [[ -z "${key}" ]] && continue
      "${callback}" "${key}"
    done < <(printf '%s\n' "${response}" | sed '1d')

    cursor="${next_cursor}"
    [[ "${cursor}" == "0" ]] && break
    if [[ "${scan_sleep}" != "0" && "${scan_sleep}" != "0.0" ]]; then
      sleep "${scan_sleep}"
    fi
  done
}

process_meta_key() {
  local meta_key="$1"
  local index_key
  local result

  meta_scanned=$((meta_scanned + 1))
  if ! index_key="$(index_key_from_meta_key "${meta_key}")"; then
    invalid_meta=$((invalid_meta + 1))
    return
  fi

  if [[ "${apply}" -eq 1 ]]; then
    result="$("${redis_cli[@]}" --eval "${lua_script}" "${meta_key}" "${index_key}" , apply)"
  else
    result="$("${redis_cli[@]}" --eval "${lua_script}" "${meta_key}" "${index_key}" , dry-run)"
  fi

  case "${result}" in
    1)
      if [[ "${apply}" -eq 1 ]]; then
        added_index=$((added_index + 1))
      else
        missing_index=$((missing_index + 1))
      fi
      ;;
    0)
      existing_index=$((existing_index + 1))
      ;;
    *)
      invalid_meta=$((invalid_meta + 1))
      ;;
  esac
}

process_zset_key() {
  local zset_key="$1"
  local count

  zset_keys=$((zset_keys + 1))
  count="$("${redis_cli[@]}" --raw HLEN "${zset_key}")"
  if ! [[ "${count}" =~ ^[0-9]+$ ]]; then
    echo "invalid HLEN response for ${zset_key}: ${count}" >&2
    exit 1
  fi

  zset_sessions=$((zset_sessions + count))
  if [[ "${zset_keys}" -le 20 ]]; then
    echo "legacy zset key: ${zset_key}, sessions=${count}"
  fi
}

for meta_pattern in "${meta_patterns[@]}"; do
  scan_pattern "${meta_pattern}" process_meta_key
done

if [[ "${check_zset}" -eq 1 ]]; then
  for zset_pattern in "${zset_patterns[@]}"; do
    scan_pattern "${zset_pattern}" process_zset_key
  done
fi

mode="dry-run"
if [[ "${apply}" -eq 1 ]]; then
  mode="apply"
fi

echo "mode: ${mode}"
echo "hashidx meta scanned: ${meta_scanned}"
if [[ "${apply}" -eq 1 ]]; then
  echo "hashidx index entries added: ${added_index}"
else
  echo "hashidx index entries missing: ${missing_index}"
fi
echo "hashidx index entries already existed: ${existing_index}"
echo "hashidx invalid meta skipped: ${invalid_meta}"
if [[ "${check_zset}" -eq 1 ]]; then
  echo "legacy zset keys found: ${zset_keys}"
  echo "legacy zset sessions found: ${zset_sessions}"
fi

if [[ "${fail_on_zset}" -eq 1 && "${zset_sessions}" -gt 0 ]]; then
  exit 1
fi
