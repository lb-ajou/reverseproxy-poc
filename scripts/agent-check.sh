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

require_task_metadata() {
  local task_file="$1"
  grep -Eiq 'Requirements-Clarity:[[:space:]]*(clear|unclear)' "$task_file" ||
    fail "task file must declare Requirements-Clarity"
  grep -Eiq 'Clarification-Status:[[:space:]]*(resolved|pending)' "$task_file" ||
    fail "task file must declare Clarification-Status"
  grep -Eiq 'Assumptions-Used:[[:space:]]*(no|yes)' "$task_file" ||
    fail "task file must declare Assumptions-Used"
  grep -Eiq 'Assumption-Approval:[[:space:]]*(not-required|approved|pending)' "$task_file" ||
    fail "task file must declare Assumption-Approval"
  grep -Eiq 'Function-Length-Exception:[[:space:]]*(no|yes)' "$task_file" ||
    fail "task file must declare Function-Length-Exception"
  grep -Eiq 'Function-Length-Approval:[[:space:]]*(not-required|approved|pending)' "$task_file" ||
    fail "task file must declare Function-Length-Approval"
  grep -Eiq 'Implementation-Plan-Status:[[:space:]]*(drafted|pending|approved)' "$task_file" ||
    fail "task file must declare Implementation-Plan-Status"
  grep -Eiq 'Implementation-Step-Status:[[:space:]]*(planning-only|awaiting-user-approval|approved-for-implementation)' "$task_file" ||
    fail "task file must declare Implementation-Step-Status"
  grep -Eiq 'Implementation-Approval:[[:space:]]*(approved|pending)' "$task_file" ||
    fail "task file must declare Implementation-Approval"
}

check_requirement_governance() {
  local task_file
  local clarity_status
  local assumption_status

  task_file="$(resolve_task_file)" || fail "missing task file under plan/tasks/; create one from docs/harness/task-template.md"
  require_task_metadata "$task_file"

  if grep -Eiq 'Requirements-Clarity:[[:space:]]*unclear' "$task_file"; then
    grep -Eiq 'Clarification-Status:[[:space:]]*resolved' "$task_file" ||
      fail "unclear requirements require Clarification-Status: resolved before implementation"
    grep -Eiq 'Clarification-Questions:[[:space:]]*(.+|none)' "$task_file" ||
      fail "unclear requirements must record Clarification-Questions"
    grep -Eiq 'Clarification-Answer:[[:space:]]*(.+|none)' "$task_file" ||
      fail "unclear requirements must record Clarification-Answer"
  fi

  if grep -Eiq 'Assumptions-Used:[[:space:]]*yes' "$task_file"; then
    grep -Eiq 'Assumption-Approval:[[:space:]]*approved' "$task_file" ||
      fail "assumptions require user approval before implementation"
    grep -Eiq 'Approval-Evidence:[[:space:]]*.+$' "$task_file" ||
      fail "approved assumptions must record Approval-Evidence"
  fi

  if grep -Eiq 'Function-Length-Exception:[[:space:]]*yes' "$task_file"; then
    grep -Eiq 'Function-Length-Approval:[[:space:]]*approved' "$task_file" ||
      fail "function length exceptions require user approval before implementation"
    grep -Eiq 'Function-Length-Evidence:[[:space:]]*.+$' "$task_file" ||
      fail "function length exceptions must record Function-Length-Evidence"
  fi

  if grep -Eiq 'Implementation-Plan-Status:[[:space:]]*(drafted|pending)' "$task_file"; then
    fail "implementation plan must be approved before writing code"
  fi

  if grep -Eiq 'Implementation-Step-Status:[[:space:]]*(planning-only|awaiting-user-approval)' "$task_file"; then
    fail "implementation step is still waiting for approval"
  fi

  grep -Eiq 'Implementation-Approval:[[:space:]]*approved' "$task_file" ||
    fail "implementation plan requires user approval before implementation"
  grep -Eiq 'Implementation-Approval-Evidence:[[:space:]]*.+$' "$task_file" ||
    fail "approved implementation plans must record Implementation-Approval-Evidence"

  clarity_status="$(grep -Eio 'Requirements-Clarity:[[:space:]]*(clear|unclear)' "$task_file" | tail -n 1)"
  assumption_status="$(grep -Eio 'Assumptions-Used:[[:space:]]*(no|yes)' "$task_file" | tail -n 1)"
  info "task governance: ok (${clarity_status:-unknown}, ${assumption_status:-unknown})"
}

function_length_exception_allowed() {
  local task_file
  task_file="$(resolve_task_file)" || return 1
  grep -Eiq 'Function-Length-Exception:[[:space:]]*yes' "$task_file" &&
    grep -Eiq 'Function-Length-Approval:[[:space:]]*approved' "$task_file"
}

approved_files_contains() {
  local task_file="$1"
  local candidate="$2"
  perl -e '
    my ($candidate, $task_file) = @ARGV;
    open my $fh, "<", $task_file or exit 1;
    while (my $line = <$fh>) {
      next unless $line =~ /Approved-Files:\s*(.+)$/;
      my @items = map { s/^\s+|\s+$//gr } split /,/, $1;
      exit 0 if grep { $_ eq $candidate } @items;
      exit 1;
    }
    exit 1;
  ' "$candidate" "$task_file"
}

check_approved_files() {
  local task_file
  local -a staged=()
  local path

  task_file="$(resolve_task_file)" || fail "missing task file under plan/tasks/"
  if ! has_staged_changes; then
    info "approval: no staged changes, skipping approved file gate"
    return
  fi

  mapfile -t staged < <(staged_paths)
  for path in "${staged[@]}"; do
    approved_files_contains "$task_file" "$path" ||
      fail "staged file is not listed in Approved-Files: $path"
  done

  info "approval: approved files match staged changes"
}

check_function_length() {
  local -a go_files=()
  local output=""

  if ! has_staged_changes; then
    info "length: no staged changes, skipping function length gate"
    return
  fi

  mapfile -t go_files < <(git diff --cached --name-only --diff-filter=ACMR -- "*.go")
  if ((${#go_files[@]} == 0)); then
    info "length: no staged Go files to check"
    return
  fi

  if function_length_exception_allowed; then
    info "length: approved exception found in $(resolve_task_file)"
    return
  fi

  output="$(
    perl -ne '
      our ($in_func, $sig, $start, $depth, $line);
      $line = $.;
      if (!$in_func && /^\s*func\b/) {
        $in_func = 1;
        $sig = $_;
        $start = $line;
        $depth = 0;
      }
      if ($in_func) {
        $depth += tr/{/{/;
        $depth -= tr/}/}/;
        if ($depth == 0 && /\}/) {
          my $len = $line - $start + 1;
          if ($len > 15) {
            chomp($sig);
            print "$ARGV:$start:$len:$sig\n";
          }
          $in_func = 0;
          $sig = q{};
        }
      }
    ' "${go_files[@]}"
  )"

  if [[ -n "$output" ]]; then
    printf 'FAIL: function or method length exceeded 15 lines:\n' >&2
    while IFS= read -r line; do
      printf '  %s\n' "$line" >&2
    done <<< "$output"
    fail "split large functions or record an approved exception in the task file"
  fi

  info "length: ok"
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

check_requirement_governance
check_approved_files
check_function_length
check_format
run_tests
check_doc_sync
check_dashboard_asset

info "agent check completed"
