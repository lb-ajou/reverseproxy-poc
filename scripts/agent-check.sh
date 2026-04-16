#!/usr/bin/env bash

set -euo pipefail

mode="${1:-fast}"

case "$mode" in
  fast|full)
    ;;
  *)
    echo "usage: $0 [fast|full]" >&2
    exit 2
    ;;
esac

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-/tmp/go-build-cache}"
mkdir -p "$GOCACHE"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

info() {
  echo "==> $*"
}

check_format() {
  local -a go_files=()

  if has_staged_changes; then
    mapfile -t go_files < <(git diff --cached --name-only --diff-filter=ACMR -- "*.go")
  else
    info "format: no staged Go files, skipping staged format gate"
    return
  fi

  if ((${#go_files[@]} == 0)); then
    info "format: no staged Go files to check"
    return
  fi

  mapfile -t dirty < <(gofmt -l "${go_files[@]}")
  if ((${#dirty[@]} > 0)); then
    printf 'FAIL: gofmt mismatch:\n' >&2
    printf '  %s\n' "${dirty[@]}" >&2
    fail "run gofmt before committing"
  fi

  info "format: ok"
}

package_list() {
  local pkg
  while IFS= read -r pkg; do
    case "$mode" in
      fast)
        case "$pkg" in
          reverseproxy-poc/internal/proxy|reverseproxy-poc/internal/upstream)
            continue
            ;;
        esac
        ;;
    esac
    printf '%s\n' "$pkg"
  done < <(go list ./...)
}

run_tests() {
  mapfile -t packages < <(package_list)
  if ((${#packages[@]} == 0)); then
    fail "no packages selected for test run"
  fi

  info "test: running $mode suite"
  go test "${packages[@]}"
}

has_staged_changes() {
  ! git diff --cached --quiet --ignore-submodules --
}

staged_paths() {
  git diff --cached --name-only --diff-filter=ACMR
}

resolve_task_file() {
  local candidate=""
  local latest=""
  local latest_mtime=0

  if [[ -n "${TASK_FILE:-}" ]]; then
    [[ -f "$TASK_FILE" ]] || fail "TASK_FILE does not exist: $TASK_FILE"
    printf '%s\n' "$TASK_FILE"
    return
  fi

  shopt -s nullglob
  for candidate in plan/tasks/*.md; do
    local mtime
    mtime="$(stat -c '%Y' "$candidate")"
    if ((mtime > latest_mtime)); then
      latest="$candidate"
      latest_mtime="$mtime"
    fi
  done
  shopt -u nullglob

  [[ -n "$latest" ]] || return 1
  printf '%s\n' "$latest"
}

doc_acknowledged() {
  local task_file
  task_file="$(resolve_task_file)" || return 1
  grep -Eiq 'Docs-Impact:[[:space:]]*none' "$task_file" &&
    grep -Eiq 'Docs-Reason:[[:space:]]*.+$' "$task_file"
}

check_doc_sync() {
  local -a staged=()
  local need_dashboard=0
  local need_architecture=0
  local need_type_ref=0
  local path

  if ! has_staged_changes; then
    info "docs: no staged changes, skipping staged doc sync gate"
    return
  fi

  mapfile -t staged < <(staged_paths)
  for path in "${staged[@]}"; do
    case "$path" in
      docs/api/dashboard-api.ko.md|docs/architecture/architecture.ko.md|docs/conventions/directory-convention.ko.md|docs/conventions/type-reference.ko.md)
        ;;
      internal/admin/*|internal/dashboard/*)
        need_dashboard=1
        ;;
      internal/app/*|internal/config/*|internal/proxy/*|internal/proxyconfig/*|internal/route/*|internal/runtime/*|internal/upstream/*|main.go|configs/app.json)
        need_architecture=1
        ;;
    esac

    case "$path" in
      internal/proxyconfig/config.go|internal/proxyconfig/errors.go|internal/route/route.go|internal/upstream/upstream.go)
        need_type_ref=1
        ;;
    esac
  done

  if ((need_dashboard == 0 && need_architecture == 0 && need_type_ref == 0)); then
    info "docs: no synced docs required for staged paths"
    return
  fi

  if doc_acknowledged; then
    info "docs: task waiver found in $(resolve_task_file)"
    return
  fi

  if ((need_dashboard == 1)) && ! printf '%s\n' "${staged[@]}" | grep -qx 'docs/api/dashboard-api.ko.md'; then
    fail "staged dashboard/admin changes require docs/api/dashboard-api.ko.md or a Docs-Impact waiver in plan/tasks/*.md"
  fi

  if ((need_architecture == 1)) && ! printf '%s\n' "${staged[@]}" | grep -Eq '^(docs/architecture/architecture\.ko\.md|docs/conventions/directory-convention\.ko\.md)$'; then
    fail "staged structural changes require docs/architecture/architecture.ko.md or docs/conventions/directory-convention.ko.md"
  fi

  if ((need_type_ref == 1)) && ! printf '%s\n' "${staged[@]}" | grep -qx 'docs/conventions/type-reference.ko.md'; then
    fail "staged type-contract changes require docs/conventions/type-reference.ko.md"
  fi

  info "docs: ok"
}

check_dashboard_asset() {
  local asset="internal/dashboard/static/index.html"
  [[ -f "$asset" ]] || fail "missing dashboard asset: $asset"
  info "asset: ok"
}

check_format
run_tests
check_doc_sync
check_dashboard_asset

info "agent check completed"
