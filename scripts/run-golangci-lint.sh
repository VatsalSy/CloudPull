#!/usr/bin/env bash

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION:-v1.64.2}"
LOCAL_BIN_DIR="${PROJECT_ROOT}/.cache/bin"

mkdir -p "${PROJECT_ROOT}/.cache"
mkdir -p "${PROJECT_ROOT}/.cache/gocache"
mkdir -p "${PROJECT_ROOT}/.cache/gomod"
mkdir -p "${PROJECT_ROOT}/.cache/gopath"
mkdir -p "${PROJECT_ROOT}/.cache/golangci-lint"
mkdir -p "${LOCAL_BIN_DIR}"

export GOCACHE="${PROJECT_ROOT}/.cache/gocache"
export GOMODCACHE="${PROJECT_ROOT}/.cache/gomod"
export GOPATH="${PROJECT_ROOT}/.cache/gopath"
export GOLANGCI_LINT_CACHE="${PROJECT_ROOT}/.cache/golangci-lint"

get_golangci_lint_version() {
  local bin="$1"
  "$bin" version 2>/dev/null | sed -nE 's/.*version (v[0-9]+\.[0-9]+\.[0-9]+).*/\1/p' | head -n 1
}

version_ge() {
  local a="${1#v}"
  local b="${2#v}"
  local a_major a_minor a_patch
  local b_major b_minor b_patch

  IFS='.' read -r a_major a_minor a_patch <<<"$a"
  IFS='.' read -r b_major b_minor b_patch <<<"$b"

  if ((a_major != b_major)); then
    ((a_major > b_major))
    return
  fi

  if ((a_minor != b_minor)); then
    ((a_minor > b_minor))
    return
  fi

  ((a_patch >= b_patch))
}

select_golangci_lint() {
  local local_bin="${LOCAL_BIN_DIR}/golangci-lint"

  if [[ -x "${local_bin}" ]]; then
    local local_version
    local_version="$(get_golangci_lint_version "${local_bin}")"
    if [[ -n "${local_version}" ]] && version_ge "${local_version}" "${GOLANGCI_LINT_VERSION}"; then
      echo "${local_bin}"
      return
    fi
  fi

  if command -v golangci-lint >/dev/null 2>&1; then
    local global_bin
    global_bin="$(command -v golangci-lint)"
    local global_version
    global_version="$(get_golangci_lint_version "${global_bin}")"
    if [[ -n "${global_version}" ]] && version_ge "${global_version}" "${GOLANGCI_LINT_VERSION}"; then
      echo "${global_bin}"
      return
    fi
  fi

  echo ""
}

cd "${PROJECT_ROOT}"

GOLANGCI_LINT_BIN="$(select_golangci_lint)"
if [[ -z "${GOLANGCI_LINT_BIN}" ]]; then
  echo "Installing golangci-lint ${GOLANGCI_LINT_VERSION} into ${LOCAL_BIN_DIR}..." >&2
  GOBIN="${LOCAL_BIN_DIR}" go install "github.com/golangci/golangci-lint/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}"
  GOLANGCI_LINT_BIN="${LOCAL_BIN_DIR}/golangci-lint"
fi

exec "${GOLANGCI_LINT_BIN}" run --config="${PROJECT_ROOT}/.golangci.yml" --timeout=5m "$@"
