#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "$REPO_ROOT"

COMPOSE_FILE="composes/raft-ha-cluster/compose.yaml"
PROJECT_NAME="${RAFT_HA_PROJECT_NAME:-reverseproxy-raft-ha-$$}"
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

cleanup() {
  local status=$?
  trap - EXIT INT TERM

  if [ "${KEEP_RAFT_HA_SMOKE:-0}" = "1" ]; then
    log "leaving compose environment running for inspection: project=${PROJECT_NAME}"
  else
    compose down -v --remove-orphans >/dev/null 2>&1 || true
  fi

  exit "$status"
}

require_command() {
  local command_name="$1"
  if ! command -v "$command_name" >/dev/null 2>&1; then
    fail "required command not found: ${command_name}"
  fi
}

check_dependencies() {
  require_command curl
  require_command docker
  require_command go
  require_command jq
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

config_has_route() {
  local body="$1"
  local route_id="$2"
  printf "%s" "$body" | jq -e --arg route_id "$route_id" '.routes[]? | select(.id == $route_id)' >/dev/null
}

config_has_pool() {
  local body="$1"
  local pool_id="$2"
  printf "%s" "$body" | jq -e --arg pool_id "$pool_id" '.upstream_pools[$pool_id] != null' >/dev/null
}

try_config_has_route() {
  local node="$1"
  local route_id="$2"
  local url
  url="$(dashboard_url "$node")/api/namespaces/default/config"

  local body
  body="$(curl -fsS "$url" 2>/dev/null)" || return 1
  config_has_route "$body" "$route_id"
}

try_config_has_pool() {
  local node="$1"
  local pool_id="$2"
  local url
  url="$(dashboard_url "$node")/api/namespaces/default/config"

  local body
  body="$(curl -fsS "$url" 2>/dev/null)" || return 1
  config_has_pool "$body" "$pool_id"
}

require_config_has_route() {
  local node="$1"
  local route_id="$2"
  local url
  url="$(dashboard_url "$node")/api/namespaces/default/config"

  local body
  if ! body="$(curl -fsS "$url")"; then
    fail "${node} config request failed"
  fi
  if ! config_has_route "$body" "$route_id"; then
    printf "%s\n" "$body" >&2
    fail "${node} config does not contain route ${route_id}"
  fi
}

require_config_has_pool() {
  local node="$1"
  local pool_id="$2"
  local url
  url="$(dashboard_url "$node")/api/namespaces/default/config"

  local body
  if ! body="$(curl -fsS "$url")"; then
    fail "${node} config request failed"
  fi
  if ! config_has_pool "$body" "$pool_id"; then
    printf "%s\n" "$body" >&2
    fail "${node} config does not contain pool ${pool_id}"
  fi
}

try_proxy_route() {
  local node="$1"
  local host="$2"
  local url
  url="$(proxy_url "$node")/api/info"

  local body
  body="$(curl -fsS -H "Host: ${host}" "$url" 2>/dev/null)" || return 1
  printf "%s" "$body" | jq -e '.server | test("^backend-[abc]$")' >/dev/null
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
  if ! printf "%s" "$body" | jq -e '.server | test("^backend-[abc]$")' >/dev/null; then
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

require_follower_write_rejected() {
  local follower="$1"
  local base
  base="$(dashboard_url "$follower")"

  local response_file status body
  response_file="$(mktemp)"
  status="$(curl -sS -o "$response_file" -w "%{http_code}" \
    -X POST "${base}/api/namespaces/default/routes" \
    -H "Content-Type: application/json" \
    -d '{
      "id": "r-follower-rejected",
      "enabled": true,
      "match": {
        "hosts": ["rejected.localtest.me"],
        "path": { "type": "prefix", "value": "/" }
      },
      "upstream_pool": "pool-raft"
    }')"
  body="$(cat "$response_file")"
  rm -f "$response_file"

  if [ "$status" != "409" ]; then
    printf "%s\n" "$body" >&2
    fail "expected follower write on ${follower} to return 409, got ${status}"
  fi
  if ! printf "%s" "$body" | jq -e '.code == "not_raft_leader"' >/dev/null; then
    printf "%s\n" "$body" >&2
    fail "expected follower write on ${follower} to return not_raft_leader"
  fi
  if ! printf "%s" "$body" | jq -e '.leader_address | type == "string" and length > 0' >/dev/null; then
    printf "%s\n" "$body" >&2
    fail "expected follower write on ${follower} to include leader_address"
  fi
}

require_join_validation_rejected() {
  local request_body="$1"
  local expected_code="$2"

  local response_file status response
  response_file="$(mktemp)"
  status="$(curl -sS -o "$response_file" -w "%{http_code}" \
    -X POST "http://localhost:19090/api/raft/join" \
    -H "Content-Type: application/json" \
    -d "$request_body")"
  response="$(cat "$response_file")"
  rm -f "$response_file"

  if [ "$status" != "400" ]; then
    printf "%s\n" "$response" >&2
    fail "expected join validation to return 400, got ${status}"
  fi
  if ! printf "%s" "$response" | jq -e --arg code "$expected_code" '.code == $code' >/dev/null; then
    printf "%s\n" "$response" >&2
    fail "expected join validation code ${expected_code}"
  fi
}

try_create_failover_pool() {
  local node="$1"
  local base
  base="$(dashboard_url "$node")"

  curl -fsS -X POST "${base}/api/namespaces/default/upstream-pools" \
    -H "Content-Type: application/json" \
    -d '{
      "id": "pool-failover",
      "upstreams": ["backend-a:8080", "backend-b:8080", "backend-c:8080"],
      "health_check": {
        "path": "/health",
        "interval": "5s",
        "timeout": "2s",
        "expect_status": 200
      }
    }' >/dev/null
}

create_failover_route() {
  local leader="$1"
  local base
  base="$(dashboard_url "$leader")"

  curl -fsS -X POST "${base}/api/namespaces/default/routes" \
    -H "Content-Type: application/json" \
    -d '{
      "id": "r-failover",
      "enabled": true,
      "match": {
        "hosts": ["raft-failover.localtest.me"],
        "path": { "type": "prefix", "value": "/" }
      },
      "upstream_pool": "pool-failover"
    }' >/dev/null
}

find_leader_after_failover() {
  local attempts="${1:-60}"

  for _ in $(seq 1 "$attempts"); do
    for node in node-2 node-3; do
      if try_create_failover_pool "$node" >/dev/null 2>&1; then
        printf "%s" "$node"
        return 0
      fi
    done
    sleep 1
  done

  fail "could not find leader after stopping proxy-1"
}

wait_config_has_route() {
  local node="$1"
  local route_id="$2"
  local attempts="${3:-60}"

  for _ in $(seq 1 "$attempts"); do
    if try_config_has_route "$node" "$route_id"; then
      return 0
    fi
    sleep 1
  done

  fail "${node} config did not converge on route ${route_id}"
}

wait_config_has_pool() {
  local node="$1"
  local pool_id="$2"
  local attempts="${3:-60}"

  for _ in $(seq 1 "$attempts"); do
    if try_config_has_pool "$node" "$pool_id"; then
      return 0
    fi
    sleep 1
  done

  fail "${node} config did not converge on pool ${pool_id}"
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
  trap cleanup EXIT INT TERM

  check_dependencies

  log "reset compose environment"
  compose down -v --remove-orphans

  log "build linux binaries"
  build_binaries

  log "start backends and bootstrap node"
  compose up -d --build backend-a backend-b backend-c proxy-1

  wait_http "http://localhost:19090/api/namespaces/default/config" "proxy-1 dashboard"

  log "verify bootstrap seed on proxy-1"
  require_config_has_route node-1 "r-raft"
  require_config_has_pool node-1 "pool-raft"
  wait_proxy_route node-1 "raft.localtest.me"

  log "bootstrap checks passed"

  log "start joining nodes"
  compose up -d --build proxy-2 proxy-3

  wait_http "http://localhost:19091/api/namespaces/default/config" "proxy-2 dashboard"
  wait_http "http://localhost:19092/api/namespaces/default/config" "proxy-3 dashboard"

  log "verify joined nodes caught up with seed"
  wait_config_has_route node-2 "r-raft"
  wait_config_has_route node-3 "r-raft"
  wait_proxy_route node-2 "raft.localtest.me"
  wait_proxy_route node-3 "raft.localtest.me"

  log "write new route through proxy-1 leader"
  create_added_pool node-1
  create_added_route node-1

  log "verify replication to all nodes"
  wait_config_has_route node-1 "r-added"
  wait_config_has_route node-2 "r-added"
  wait_config_has_route node-3 "r-added"
  wait_config_has_pool node-1 "pool-added"
  wait_config_has_pool node-2 "pool-added"
  wait_config_has_pool node-3 "pool-added"
  wait_proxy_route node-1 "raft-added.localtest.me"
  wait_proxy_route node-2 "raft-added.localtest.me"
  wait_proxy_route node-3 "raft-added.localtest.me"

  log "join and replication checks passed"

  log "verify follower write rejection"
  require_follower_write_rejected node-2

  log "verify raft join validation"
  require_join_validation_rejected '{"node_id":"bad:node","raft_address":"proxy-bad:7009"}' "invalid_node_id"
  require_join_validation_rejected '{"node_id":"node-bad","raft_address":"not-a-host-port"}' "invalid_raft_address"

  log "negative checks passed"

  log "stop proxy-1 and wait for failover"
  compose stop proxy-1
  sleep 3

  local new_leader
  new_leader="$(find_leader_after_failover)"
  log "new leader after failover: ${new_leader}"
  create_failover_route "$new_leader"

  log "verify failover write on surviving nodes"
  wait_config_has_route node-2 "r-failover"
  wait_config_has_route node-3 "r-failover"
  wait_config_has_pool node-2 "pool-failover"
  wait_config_has_pool node-3 "pool-failover"
  wait_proxy_route node-2 "raft-failover.localtest.me"
  wait_proxy_route node-3 "raft-failover.localtest.me"

  log "restart old leader and verify catch-up"
  compose up -d proxy-1
  wait_http "http://localhost:19090/api/namespaces/default/config" "proxy-1 dashboard after rejoin"
  wait_config_has_route node-1 "r-failover"
  wait_config_has_pool node-1 "pool-failover"
  wait_proxy_route node-1 "raft-failover.localtest.me"

  log "verify persistence across stop/start"
  compose stop proxy-1 proxy-2 proxy-3
  compose up -d proxy-1 proxy-2 proxy-3
  wait_http "http://localhost:19090/api/namespaces/default/config" "proxy-1 dashboard after persistence restart"
  wait_http "http://localhost:19091/api/namespaces/default/config" "proxy-2 dashboard after persistence restart"
  wait_http "http://localhost:19092/api/namespaces/default/config" "proxy-3 dashboard after persistence restart"
  wait_config_has_route node-1 "r-failover"
  wait_config_has_route node-2 "r-failover"
  wait_config_has_route node-3 "r-failover"
  wait_config_has_pool node-1 "pool-failover"
  wait_config_has_pool node-2 "pool-failover"
  wait_config_has_pool node-3 "pool-failover"

  log "failover, rejoin, and persistence checks passed"
}

main "$@"
