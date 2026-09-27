#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENDOR_DIR="$(cd "${SCRIPT_DIR}/../src/renderer/lib/ghostty/vendor" && pwd)"
GHOSTTY_REVISION="$(tr -d '[:space:]' < "${VENDOR_DIR}/VERSION")"
CACHE_DIR="${HOME}/.cache/babysitter"
GHOSTTY_SOURCE_DIR="${GHOSTTY_SOURCE_DIR:-${CACHE_DIR}/ghostty-${GHOSTTY_REVISION:0:8}}"
GHOSTTY_ZIG_VERSION="${GHOSTTY_ZIG_VERSION:-0.15.2}"
GHOSTTY_ZIG="${GHOSTTY_ZIG:-}"

log() {
  printf '[libghostty-vt-wasm] %s\n' "$*"
}

die() {
  printf '[libghostty-vt-wasm] error: %s\n' "$*" >&2
  exit 1
}

ensure_zig() {
  if [[ -n "${GHOSTTY_ZIG}" ]]; then
    [[ -x "${GHOSTTY_ZIG}" ]] || die "GHOSTTY_ZIG is not executable: ${GHOSTTY_ZIG}"
    return
  fi
  if command -v zig >/dev/null 2>&1 && [[ "$(zig version)" == "${GHOSTTY_ZIG_VERSION}" ]]; then
    GHOSTTY_ZIG="$(command -v zig)"
    return
  fi

  local host_os host_arch zig_dir
  host_os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  host_arch="$(uname -m)"
  case "${host_os}" in
    darwin) host_os="macos" ;;
    linux) ;;
    *) die "no Zig download for ${host_os}" ;;
  esac
  case "${host_arch}" in
    arm64) host_arch="aarch64" ;;
    aarch64 | x86_64) ;;
    *) die "no Zig download for ${host_arch}" ;;
  esac

  zig_dir="${CACHE_DIR}/zig-${GHOSTTY_ZIG_VERSION}"
  GHOSTTY_ZIG="${zig_dir}/zig"
  [[ -x "${GHOSTTY_ZIG}" ]] && return
  mkdir -p "${zig_dir}"
  log "downloading Zig ${GHOSTTY_ZIG_VERSION}"
  curl -fsSL \
    "https://ziglang.org/download/${GHOSTTY_ZIG_VERSION}/zig-${host_arch}-${host_os}-${GHOSTTY_ZIG_VERSION}.tar.xz" \
    | tar -xJ --strip-components=1 -C "${zig_dir}"
}

ensure_ghostty_source() {
  if [[ ! -d "${GHOSTTY_SOURCE_DIR}/.git" ]]; then
    mkdir -p "$(dirname "${GHOSTTY_SOURCE_DIR}")"
    log "cloning Ghostty"
    git clone --filter=blob:none --no-checkout https://github.com/ghostty-org/ghostty.git "${GHOSTTY_SOURCE_DIR}"
  fi
  if [[ "$(git -C "${GHOSTTY_SOURCE_DIR}" rev-parse HEAD 2>/dev/null || echo none)" != "${GHOSTTY_REVISION}" ]]; then
    log "checking out Ghostty ${GHOSTTY_REVISION}"
    git -C "${GHOSTTY_SOURCE_DIR}" fetch --depth=1 origin "${GHOSTTY_REVISION}"
    git -C "${GHOSTTY_SOURCE_DIR}" checkout --detach "${GHOSTTY_REVISION}"
  fi
  [[ "$(git -C "${GHOSTTY_SOURCE_DIR}" rev-parse HEAD)" == "${GHOSTTY_REVISION}" ]] \
    || die "the checkout is not at ${GHOSTTY_REVISION}"
}

ensure_zig
ensure_ghostty_source

build_root="$(mktemp -d)"
trap 'rm -rf "${build_root}"' EXIT

log "building ${GHOSTTY_REVISION} for wasm32-freestanding"
(
  cd "${GHOSTTY_SOURCE_DIR}"
  "${GHOSTTY_ZIG}" build \
    -Demit-lib-vt \
    -Dtarget=wasm32-freestanding \
    -Doptimize=ReleaseSmall \
    -Dstrip=true \
    -Dlib-version-string="0.1.0-dev+${GHOSTTY_REVISION}" \
    -p "${build_root}"
)

cp "${build_root}/bin/ghostty-vt.wasm" "${VENDOR_DIR}/ghostty-vt.wasm"
log "wrote ${VENDOR_DIR}/ghostty-vt.wasm"
