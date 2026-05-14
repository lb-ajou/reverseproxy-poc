#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "$REPO_ROOT"

COMPOSE_FILE="composes/raft-ha-cluster/compose.yaml"
PROJECT_NAME="reverseproxy-raft-ha"
OUT_DIR="composes/raft-ha-cluster/.out"

dashboard_url() {
  case "$1" in
    node-1) printf "http://localhost:19090" ;;
    node-2) printf "http://localhost:19091" ;;
    node-3) printf "http://localhost:19092" ;;
    *) printf "unknown node: %s\n" "$1" >&2; return 1 ;;
  esac
}

proxy_url() {
  case "$1" in
    node-1) printf "http://localhost:18080" ;;
    node-2) printf "http://localhost:18081" ;;
    node-3) printf "http://localhost:18082" ;;
    *) printf "unknown node: %s\n" "$1" >&2; return 1 ;;
  esac
}

log() {
  printf "\n[raft-ha-smoke] %s\n" "$*"
}

fail() {
  printf "\n[raft-ha-smoke] FAIL: %s\n" "$*" >&2
  exit 1
}

compose() {
  docker compose -p "$PROJECT_NAME" -f "$COMPOSE_FILE" "$@"
}

build_binaries() {
  mkdir -p "$OUT_DIR"
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "${OUT_DIR}/reverseproxy" ./main.go
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "${OUT_DIR}/test-server" ./composes/test-server
}

wait_http() {
  local url="$1"
  local name="$2"
  local attempts="${3:-60}"
  local delay="${4:-1}"

  for _ in $(seq 1 "$attempts"); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep "$delay"
  done

  fail "timed out waiting for ${name} at ${url}"
}

json_contains() {
  local json="$1"
  local expected="$2"
  printf "%s" "$json" | grep -Fq "$expected"
}

try_config_contains() {
  local node="$1"
  local expected="$2"
  local url
  url="$(dashboard_url "$node")/api/namespaces/default/config"

  local body
  body="$(curl -fsS "$url" 2>/dev/null)" || return 1
  json_contains "$body" "$expected"
}

require_config_contains() {
  local node="$1"
  local expected="$2"
  local url
  url="$(dashboard_url "$node")/api/namespaces/default/config"

  local body
  if ! body="$(curl -fsS "$url")"; then
    fail "${node} config request failed"
  fi
  if ! json_contains "$body" "$expected"; then
    printf "%s\n" "$body" >&2
    fail "${node} config does not contain ${expected}"
  fi
}

try_proxy_route() {
  local node="$1"
  local host="$2"
  local url
  url="$(proxy_url "$node")/api/info"

  local body
  body="$(curl -fsS -H "Host: ${host}" "$url" 2>/dev/null)" || return 1
  printf "%s" "$body" | grep -Eq '"server":"backend-(a|b|c)"'
}

require_proxy_route() {
  local node="$1"
  local host="$2"
  local url
  url="$(proxy_url "$node")/api/info"

  local body
  if ! body="$(curl -fsS -H "Host: ${host}" "$url")"; then
    fail "${node} request for ${host} failed"
  fi
  if ! printf "%s" "$body" | grep -Eq '"server":"backend-(a|b|c)"'; then
    printf "%s\n" "$body" >&2
    fail "${node} did not route ${host} to a backend"
  fi
}

create_added_pool() {
  local leader="$1"
  local base
  base="$(dashboard_url "$leader")"

  curl -fsS -X POST "${base}/api/namespaces/default/upstream-pools" \
    -H "Content-Type: application/json" \
    -d '{
      "id": "pool-added",
      "upstreams": ["backend-a:8080", "backend-b:8080", "backend-c:8080"],
      "health_check": {
        "path": "/health",
        "interval": "5s",
        "timeout": "2s",
        "expect_status": 200
      }
    }' >/dev/null
}

create_added_route() {
  local leader="$1"
  local base
  base="$(dashboard_url "$leader")"

  curl -fsS -X POST "${base}/api/namespaces/default/routes" \
    -H "Content-Type: application/json" \
    -d '{
      "id": "r-added",
      "enabled": true,
      "match": {
        "hosts": ["raft-added.localtest.me"],
        "path": { "type": "prefix", "value": "/" }
      },
      "upstream_pool": "pool-added"
    }' >/dev/null
}

wait_config_contains() {
  local node="$1"
  local expected="$2"
  local attempts="${3:-60}"

  for _ in $(seq 1 "$attempts"); do
    if try_config_contains "$node" "$expected"; then
      return 0
    fi
    sleep 1
  done

  fail "${node} config did not converge on ${expected}"
}

wait_proxy_route() {
  local node="$1"
  local host="$2"
  local attempts="${3:-60}"

  for _ in $(seq 1 "$attempts"); do
    if try_proxy_route "$node" "$host"; then
      return 0
    fi
    sleep 1
  done

  require_proxy_route "$node" "$host"
}

main() {
  log "reset compose environment"
  compose down -v --remove-orphans

  log "build linux binaries"
  build_binaries

  log "start backends and bootstrap node"
  compose up -d --build backend-a backend-b backend-c proxy-1

  wait_http "http://localhost:19090/api/namespaces/default/config" "proxy-1 dashboard"

  log "verify bootstrap seed on proxy-1"
  require_config_contains node-1 '"r-raft"'
  require_config_contains node-1 '"pool-raft"'
  wait_proxy_route node-1 "raft.localtest.me"

  log "bootstrap checks passed"

  log "start joining nodes"
  compose up -d proxy-2 proxy-3

  wait_http "http://localhost:19091/api/namespaces/default/config" "proxy-2 dashboard"
  wait_http "http://localhost:19092/api/namespaces/default/config" "proxy-3 dashboard"

  log "verify joined nodes caught up with seed"
  wait_config_contains node-2 '"r-raft"'
  wait_config_contains node-3 '"r-raft"'
  wait_proxy_route node-2 "raft.localtest.me"
  wait_proxy_route node-3 "raft.localtest.me"

  log "write new route through proxy-1 leader"
  create_added_pool node-1
  create_added_route node-1

  log "verify replication to all nodes"
  wait_config_contains node-1 '"r-added"'
  wait_config_contains node-2 '"r-added"'
  wait_config_contains node-3 '"r-added"'
  wait_proxy_route node-1 "raft-added.localtest.me"
  wait_proxy_route node-2 "raft-added.localtest.me"
  wait_proxy_route node-3 "raft-added.localtest.me"

  log "join and replication checks passed"
}

main "$@"
