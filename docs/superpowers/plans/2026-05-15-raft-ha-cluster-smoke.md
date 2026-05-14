# Raft HA Cluster Smoke Test Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Docker Compose로 3-node Raft reverse proxy cluster와 3-backend server 환경을 구성하고, smoke script로 bootstrap, join, replication, follower rejection, failover, rejoin, persistence를 검증한다.

**Architecture:** 새 compose 시나리오 `composes/raft-ha-cluster`를 추가한다. `proxy-1`은 bootstrap node, `proxy-2`/`proxy-3`은 leader dashboard join endpoint로 합류하는 node로 구성한다. 검증은 `scripts/raft-ha-cluster-smoke.sh`가 Docker Compose와 dashboard/proxy HTTP API를 호출해 end-to-end로 수행한다.

**Tech Stack:** Docker Compose, existing `Dockerfile`, existing `composes/test-server`, Bash, `curl`, Go reverse proxy app, HashiCorp Raft runtime.

---

## File Structure

- Create: `composes/raft-ha-cluster/compose.yaml`
  - 3 backend service와 3 proxy service를 정의한다.
  - 각 proxy는 repo root `Dockerfile`로 build하고 node별 app config를 mount한다.
  - 각 proxy는 별도 named volume으로 `/app/data/raft`를 가진다.
- Create: `composes/raft-ha-cluster/configs/node-1/app.json`
  - bootstrap leader 후보 설정.
- Create: `composes/raft-ha-cluster/configs/node-2/app.json`
  - `proxy-1` dashboard로 join하는 node 설정.
- Create: `composes/raft-ha-cluster/configs/node-3/app.json`
  - `proxy-1` dashboard로 join하는 node 설정.
- Create: `composes/raft-ha-cluster/configs/seed/default.json`
  - 최초 bootstrap 시 import할 default namespace proxy config.
- Create: `composes/raft-ha-cluster/README.md`
  - 환경 구조, 수동 실행법, smoke script 설명, known limitations를 적는다.
- Create: `scripts/raft-ha-cluster-smoke.sh`
  - end-to-end 검증 자동화 script.
- Modify: `composes/README.md`
  - 새 `raft-ha-cluster` 시나리오를 목록에 추가한다.

Existing user changes:

- `AGENTS.md` 삭제 상태는 이 작업 범위 밖이다. stage/commit하지 않는다.
- `docs/architecture/raft-ha-implementation-summary.ko.md`가 untracked라면 별도 문서 작업 결과다. 이 계획 실행 시 함께 수정하지 않는다.

---

## Task 1: Add Compose Cluster Skeleton

**Files:**
- Create: `composes/raft-ha-cluster/compose.yaml`
- Create directory: `composes/raft-ha-cluster/configs/node-1`
- Create directory: `composes/raft-ha-cluster/configs/node-2`
- Create directory: `composes/raft-ha-cluster/configs/node-3`
- Create directory: `composes/raft-ha-cluster/configs/seed`

- [ ] **Step 1: Create the compose file**

Create `composes/raft-ha-cluster/compose.yaml` with this exact content:

```yaml
services:
  backend-a:
    build:
      context: ../test-server
    environment:
      SERVER_NAME: backend-a
      SCENARIO_NAME: raft-ha-cluster
      PORT: "8080"
      SERVER_VERSION: v1
      HEALTH_STATUS: "200"
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8080/health"]
      interval: 5s
      timeout: 2s
      retries: 12
      start_period: 3s
    networks:
      - raft-ha-net

  backend-b:
    build:
      context: ../test-server
    environment:
      SERVER_NAME: backend-b
      SCENARIO_NAME: raft-ha-cluster
      PORT: "8080"
      SERVER_VERSION: v1
      HEALTH_STATUS: "200"
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8080/health"]
      interval: 5s
      timeout: 2s
      retries: 12
      start_period: 3s
    networks:
      - raft-ha-net

  backend-c:
    build:
      context: ../test-server
    environment:
      SERVER_NAME: backend-c
      SCENARIO_NAME: raft-ha-cluster
      PORT: "8080"
      SERVER_VERSION: v1
      HEALTH_STATUS: "200"
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8080/health"]
      interval: 5s
      timeout: 2s
      retries: 12
      start_period: 3s
    networks:
      - raft-ha-net

  proxy-1:
    build:
      context: ../..
    command: ["/app/configs/node-1/app.json"]
    depends_on:
      backend-a:
        condition: service_healthy
      backend-b:
        condition: service_healthy
      backend-c:
        condition: service_healthy
    ports:
      - "18080:8080"
      - "19090:9090"
      - "17001:7001"
    volumes:
      - ./configs:/app/configs:ro
      - proxy-1-raft:/app/data/raft
    networks:
      - raft-ha-net

  proxy-2:
    build:
      context: ../..
    command: ["/app/configs/node-2/app.json"]
    depends_on:
      backend-a:
        condition: service_healthy
      backend-b:
        condition: service_healthy
      backend-c:
        condition: service_healthy
    ports:
      - "18081:8080"
      - "19091:9090"
      - "17002:7002"
    volumes:
      - ./configs:/app/configs:ro
      - proxy-2-raft:/app/data/raft
    networks:
      - raft-ha-net

  proxy-3:
    build:
      context: ../..
    command: ["/app/configs/node-3/app.json"]
    depends_on:
      backend-a:
        condition: service_healthy
      backend-b:
        condition: service_healthy
      backend-c:
        condition: service_healthy
    ports:
      - "18082:8080"
      - "19092:9090"
      - "17003:7003"
    volumes:
      - ./configs:/app/configs:ro
      - proxy-3-raft:/app/data/raft
    networks:
      - raft-ha-net

networks:
  raft-ha-net:
    name: raft-ha-cluster-net

volumes:
  proxy-1-raft:
  proxy-2-raft:
  proxy-3-raft:
```

- [ ] **Step 2: Validate compose syntax**

Run:

```bash
docker compose -f composes/raft-ha-cluster/compose.yaml config
```

Expected:

- Exit code `0`.
- Output includes services `backend-a`, `backend-b`, `backend-c`, `proxy-1`, `proxy-2`, `proxy-3`.
- Output includes volumes `proxy-1-raft`, `proxy-2-raft`, `proxy-3-raft`.

- [ ] **Step 3: Commit**

```bash
git add composes/raft-ha-cluster/compose.yaml
git commit -m "test(raft): add compose cluster skeleton"
```

Do not stage `AGENTS.md` or unrelated untracked docs.

---

## Task 2: Add Node Configs and Seed Config

**Files:**
- Create: `composes/raft-ha-cluster/configs/node-1/app.json`
- Create: `composes/raft-ha-cluster/configs/node-2/app.json`
- Create: `composes/raft-ha-cluster/configs/node-3/app.json`
- Create: `composes/raft-ha-cluster/configs/seed/default.json`

- [ ] **Step 1: Create node-1 app config**

Create `composes/raft-ha-cluster/configs/node-1/app.json`:

```json
{
  "proxyListenAddr": ":8080",
  "dashboardListenAddr": ":9090",
  "proxyConfigDir": "/app/configs/seed",
  "configStore": "raft",
  "raftNodeId": "node-1",
  "raftBindAddr": "0.0.0.0:7001",
  "raftAdvertiseAddr": "proxy-1:7001",
  "raftDataDir": "/app/data/raft",
  "raftBootstrap": true,
  "raftJsonSeedDir": "/app/configs/seed"
}
```

- [ ] **Step 2: Create node-2 app config**

Create `composes/raft-ha-cluster/configs/node-2/app.json`:

```json
{
  "proxyListenAddr": ":8080",
  "dashboardListenAddr": ":9090",
  "proxyConfigDir": "/app/configs/seed",
  "configStore": "raft",
  "raftNodeId": "node-2",
  "raftBindAddr": "0.0.0.0:7002",
  "raftAdvertiseAddr": "proxy-2:7002",
  "raftDataDir": "/app/data/raft",
  "raftJoinAddr": "http://proxy-1:9090"
}
```

- [ ] **Step 3: Create node-3 app config**

Create `composes/raft-ha-cluster/configs/node-3/app.json`:

```json
{
  "proxyListenAddr": ":8080",
  "dashboardListenAddr": ":9090",
  "proxyConfigDir": "/app/configs/seed",
  "configStore": "raft",
  "raftNodeId": "node-3",
  "raftBindAddr": "0.0.0.0:7003",
  "raftAdvertiseAddr": "proxy-3:7003",
  "raftDataDir": "/app/data/raft",
  "raftJoinAddr": "http://proxy-1:9090"
}
```

- [ ] **Step 4: Create seed proxy config**

Create `composes/raft-ha-cluster/configs/seed/default.json`:

```json
{
  "routes": [
    {
      "id": "r-raft",
      "enabled": true,
      "match": {
        "hosts": ["raft.localtest.me"],
        "path": { "type": "prefix", "value": "/" }
      },
      "upstream_pool": "pool-raft"
    }
  ],
  "upstream_pools": {
    "pool-raft": {
      "upstreams": [
        "backend-a:8080",
        "backend-b:8080",
        "backend-c:8080"
      ],
      "health_check": {
        "path": "/health",
        "interval": "5s",
        "timeout": "2s",
        "expect_status": 200
      }
    }
  }
}
```

- [ ] **Step 5: Validate JSON files**

Run:

```bash
go test ./internal/config ./internal/proxyconfig
```

Expected:

- Exit code `0`.

Also run:

```bash
docker compose -f composes/raft-ha-cluster/compose.yaml config
```

Expected:

- Exit code `0`.

- [ ] **Step 6: Commit**

```bash
git add composes/raft-ha-cluster/configs/node-1/app.json composes/raft-ha-cluster/configs/node-2/app.json composes/raft-ha-cluster/configs/node-3/app.json composes/raft-ha-cluster/configs/seed/default.json
git commit -m "test(raft): add cluster node configs"
```

---

## Task 3: Add Smoke Script Helpers and Bootstrap Checks

**Files:**
- Create: `scripts/raft-ha-cluster-smoke.sh`

- [ ] **Step 1: Create smoke script with helpers and bootstrap path**

Create `scripts/raft-ha-cluster-smoke.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

COMPOSE_FILE="composes/raft-ha-cluster/compose.yaml"
PROJECT_NAME="reverseproxy-raft-ha"

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

require_config_contains() {
  local node="$1"
  local expected="$2"
  local url
  url="$(dashboard_url "$node")/api/namespaces/default/config"

  local body
  body="$(curl -fsS "$url")"
  if ! json_contains "$body" "$expected"; then
    printf "%s\n" "$body" >&2
    fail "${node} config does not contain ${expected}"
  fi
}

require_proxy_route() {
  local node="$1"
  local host="$2"
  local url
  url="$(proxy_url "$node")/api/info"

  local body
  body="$(curl -fsS -H "Host: ${host}" "$url")"
  if ! printf "%s" "$body" | grep -Eq '"server":"backend-(a|b|c)"'; then
    printf "%s\n" "$body" >&2
    fail "${node} did not route ${host} to a backend"
  fi
}

main() {
  log "reset compose environment"
  compose down -v --remove-orphans

  log "start backends and bootstrap node"
  compose up -d --build backend-a backend-b backend-c proxy-1

  wait_http "http://localhost:19090/api/namespaces/default/config" "proxy-1 dashboard"

  log "verify bootstrap seed on proxy-1"
  require_config_contains node-1 '"r-raft"'
  require_config_contains node-1 '"pool-raft"'
  require_proxy_route node-1 "raft.localtest.me"

  log "bootstrap checks passed"
}

main "$@"
```

- [ ] **Step 2: Make script executable**

Run:

```bash
chmod +x scripts/raft-ha-cluster-smoke.sh
```

- [ ] **Step 3: Static-check script syntax**

Run:

```bash
bash -n scripts/raft-ha-cluster-smoke.sh
```

Expected:

- Exit code `0`.

- [ ] **Step 4: Run bootstrap smoke path**

Run:

```bash
scripts/raft-ha-cluster-smoke.sh
```

Expected:

- Exit code `0`.
- Output includes `bootstrap checks passed`.

If Docker build fails because `Dockerfile` uses Go 1.24 while the current module requires a newer local toolchain, stop and update `Dockerfile` in a separate small fix commit before continuing. Do not work around a build failure by skipping container verification.

- [ ] **Step 5: Commit**

```bash
git add scripts/raft-ha-cluster-smoke.sh
git commit -m "test(raft): add bootstrap smoke script"
```

---

## Task 4: Extend Smoke Script for Join and Replication

**Files:**
- Modify: `scripts/raft-ha-cluster-smoke.sh`

- [ ] **Step 1: Add API write helpers**

Insert these functions before `main()`:

```bash
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
    if require_config_contains "$node" "$expected" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  fail "${node} config did not converge on ${expected}"
}
```

- [ ] **Step 2: Extend `main()` after bootstrap checks**

After `log "bootstrap checks passed"`, add:

```bash
  log "start joining nodes"
  compose up -d proxy-2 proxy-3

  wait_http "http://localhost:19091/api/namespaces/default/config" "proxy-2 dashboard"
  wait_http "http://localhost:19092/api/namespaces/default/config" "proxy-3 dashboard"

  log "verify joined nodes caught up with seed"
  wait_config_contains node-2 '"r-raft"'
  wait_config_contains node-3 '"r-raft"'
  require_proxy_route node-2 "raft.localtest.me"
  require_proxy_route node-3 "raft.localtest.me"

  log "write new route through proxy-1 leader"
  create_added_pool node-1
  create_added_route node-1

  log "verify replication to all nodes"
  wait_config_contains node-1 '"r-added"'
  wait_config_contains node-2 '"r-added"'
  wait_config_contains node-3 '"r-added"'
  require_proxy_route node-1 "raft-added.localtest.me"
  require_proxy_route node-2 "raft-added.localtest.me"
  require_proxy_route node-3 "raft-added.localtest.me"

  log "join and replication checks passed"
```

- [ ] **Step 3: Static-check script syntax**

Run:

```bash
bash -n scripts/raft-ha-cluster-smoke.sh
```

Expected:

- Exit code `0`.

- [ ] **Step 4: Run smoke script**

Run:

```bash
scripts/raft-ha-cluster-smoke.sh
```

Expected:

- Exit code `0`.
- Output includes `join and replication checks passed`.

- [ ] **Step 5: Commit**

```bash
git add scripts/raft-ha-cluster-smoke.sh
git commit -m "test(raft): verify join and replication"
```

---

## Task 5: Add Follower Rejection and Join Validation Checks

**Files:**
- Modify: `scripts/raft-ha-cluster-smoke.sh`

- [ ] **Step 1: Add negative assertion helpers**

Insert these functions before `main()`:

```bash
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
  if ! printf "%s" "$body" | grep -Fq '"code":"not_raft_leader"'; then
    printf "%s\n" "$body" >&2
    fail "expected follower write on ${follower} to return not_raft_leader"
  fi
  if ! printf "%s" "$body" | grep -Fq '"leader_address"'; then
    printf "%s\n" "$body" >&2
    fail "expected follower write on ${follower} to include leader_address"
  fi
}

require_join_validation_rejected() {
  local body="$1"
  local expected_code="$2"
  local response_file status response
  response_file="$(mktemp)"
  status="$(curl -sS -o "$response_file" -w "%{http_code}" \
    -X POST "http://localhost:19090/api/raft/join" \
    -H "Content-Type: application/json" \
    -d "$body")"
  response="$(cat "$response_file")"
  rm -f "$response_file"

  if [ "$status" != "400" ]; then
    printf "%s\n" "$response" >&2
    fail "expected join validation to return 400, got ${status}"
  fi
  if ! printf "%s" "$response" | grep -Fq "\"code\":\"${expected_code}\""; then
    printf "%s\n" "$response" >&2
    fail "expected join validation code ${expected_code}"
  fi
}
```

- [ ] **Step 2: Extend `main()` after replication checks**

After `log "join and replication checks passed"`, add:

```bash
  log "verify follower write rejection"
  require_follower_write_rejected node-2

  log "verify raft join validation"
  require_join_validation_rejected '{"node_id":"bad:node","raft_address":"proxy-bad:7009"}' "invalid_node_id"
  require_join_validation_rejected '{"node_id":"node-bad","raft_address":"not-a-host-port"}' "invalid_raft_address"

  log "negative checks passed"
```

- [ ] **Step 3: Static-check script syntax**

Run:

```bash
bash -n scripts/raft-ha-cluster-smoke.sh
```

Expected:

- Exit code `0`.

- [ ] **Step 4: Run smoke script**

Run:

```bash
scripts/raft-ha-cluster-smoke.sh
```

Expected:

- Exit code `0`.
- Output includes `negative checks passed`.

- [ ] **Step 5: Commit**

```bash
git add scripts/raft-ha-cluster-smoke.sh
git commit -m "test(raft): verify join validation errors"
```

---

## Task 6: Add Failover, Rejoin, and Persistence Checks

**Files:**
- Modify: `scripts/raft-ha-cluster-smoke.sh`

- [ ] **Step 1: Add leader probing helpers**

Insert these functions before `main()`:

```bash
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
```

- [ ] **Step 2: Extend `main()` after negative checks**

After `log "negative checks passed"`, add:

```bash
  log "stop proxy-1 and wait for failover"
  compose stop proxy-1
  sleep 3

  local new_leader
  new_leader="$(find_leader_after_failover)"
  log "new leader after failover: ${new_leader}"
  create_failover_route "$new_leader"

  log "verify failover write on surviving nodes"
  wait_config_contains node-2 '"r-failover"'
  wait_config_contains node-3 '"r-failover"'
  require_proxy_route node-2 "raft-failover.localtest.me"
  require_proxy_route node-3 "raft-failover.localtest.me"

  log "restart old leader and verify catch-up"
  compose up -d proxy-1
  wait_http "http://localhost:19090/api/namespaces/default/config" "proxy-1 dashboard after rejoin"
  wait_config_contains node-1 '"r-failover"'
  require_proxy_route node-1 "raft-failover.localtest.me"

  log "verify persistence across stop/start"
  compose stop proxy-1 proxy-2 proxy-3
  compose up -d proxy-1 proxy-2 proxy-3
  wait_http "http://localhost:19090/api/namespaces/default/config" "proxy-1 dashboard after persistence restart"
  wait_http "http://localhost:19091/api/namespaces/default/config" "proxy-2 dashboard after persistence restart"
  wait_http "http://localhost:19092/api/namespaces/default/config" "proxy-3 dashboard after persistence restart"
  wait_config_contains node-1 '"r-failover"'
  wait_config_contains node-2 '"r-failover"'
  wait_config_contains node-3 '"r-failover"'

  log "failover, rejoin, and persistence checks passed"
```

- [ ] **Step 3: Static-check script syntax**

Run:

```bash
bash -n scripts/raft-ha-cluster-smoke.sh
```

Expected:

- Exit code `0`.

- [ ] **Step 4: Run full smoke script**

Run:

```bash
scripts/raft-ha-cluster-smoke.sh
```

Expected:

- Exit code `0`.
- Output includes `failover, rejoin, and persistence checks passed`.

If failover leader probing is flaky because leader election takes longer than 60 seconds, increase only the retry count in `find_leader_after_failover`; do not weaken the assertions.

- [ ] **Step 5: Commit**

```bash
git add scripts/raft-ha-cluster-smoke.sh
git commit -m "test(raft): verify failover and persistence"
```

---

## Task 7: Add Scenario Documentation

**Files:**
- Create: `composes/raft-ha-cluster/README.md`
- Modify: `composes/README.md`

- [ ] **Step 1: Create scenario README**

Create `composes/raft-ha-cluster/README.md`:

```markdown
# raft-ha-cluster

3개 reverse proxy 컨테이너가 HashiCorp Raft로 설정 상태를 공유하고, 3개 backend server로 요청을 전달하는 HA 검증 시나리오다.

## 구성

- `backend-a`, `backend-b`, `backend-c`: `/health`, `/api/info`를 제공하는 테스트 서버
- `proxy-1`: bootstrap node
  - proxy: `http://localhost:18080`
  - dashboard: `http://localhost:19090`
  - raft host port: `17001`
- `proxy-2`: join node
  - proxy: `http://localhost:18081`
  - dashboard: `http://localhost:19091`
  - raft host port: `17002`
- `proxy-3`: join node
  - proxy: `http://localhost:18082`
  - dashboard: `http://localhost:19092`
  - raft host port: `17003`

## 실행

```bash
docker compose -f composes/raft-ha-cluster/compose.yaml up -d --build
```

clean bootstrap부터 다시 시작하려면 volume까지 삭제한다.

```bash
docker compose -f composes/raft-ha-cluster/compose.yaml down -v
```

## Smoke test

```bash
scripts/raft-ha-cluster-smoke.sh
```

검증 범위:

- bootstrap node가 JSON seed를 Raft state로 import하는지
- join node가 leader를 통해 cluster에 합류하고 seed 상태를 따라오는지
- leader write가 모든 node에 복제되는지
- follower write가 `409 not_raft_leader`로 거절되는지
- join API가 잘못된 `node_id`, `raft_address`를 거절하는지
- leader 중단 후 남은 node에서 write가 가능한지
- 중단됐던 node가 재기동 후 최신 state로 catch-up 하는지
- Raft volume 유지 재시작 후 설정 state가 유지되는지

## 수동 확인 예시

```bash
curl http://localhost:19090/api/namespaces/default/config
curl http://localhost:19091/api/namespaces/default/config
curl http://localhost:19092/api/namespaces/default/config

curl -H 'Host: raft.localtest.me' http://localhost:18080/api/info
curl -H 'Host: raft.localtest.me' http://localhost:18081/api/info
curl -H 'Host: raft.localtest.me' http://localhost:18082/api/info
```

## 운영 전제

`/api/raft/join`은 admin/control-plane endpoint다. 이 POC에는 내장 인증이 없으므로 이 compose는 로컬 검증용으로만 사용한다.

`not_raft_leader` 응답의 `leader_address`는 Raft advertise address다. dashboard HTTP URL이 아닐 수 있으므로 client가 직접 retry URL로 사용하면 안 된다.
```

- [ ] **Step 2: Update composes index**

In `composes/README.md`, add this bullet to the scenario list:

```markdown
- `raft-ha-cluster`
  - 3-node Raft reverse proxy cluster와 3-backend 환경에서 bootstrap, join, replication, failover, persistence를 검증하는 시나리오
```

- [ ] **Step 3: Verify docs and full tests**

Run:

```bash
bash -n scripts/raft-ha-cluster-smoke.sh
go test ./...
```

Expected:

- Both commands exit `0`.

- [ ] **Step 4: Run full smoke script**

Run:

```bash
scripts/raft-ha-cluster-smoke.sh
```

Expected:

- Exit code `0`.
- Output includes all final success messages.

- [ ] **Step 5: Commit**

```bash
git add composes/raft-ha-cluster/README.md composes/README.md
git commit -m "docs(raft): document compose smoke scenario"
```

---

## Task 8: Final Verification and Cleanup

**Files:**
- No source changes expected unless a verification failure reveals a real bug.

- [ ] **Step 1: Check worktree**

Run:

```bash
git status --short
```

Expected:

- No unexpected modified files from this plan.
- Pre-existing `D AGENTS.md` may still be present and must remain unstaged unless the user explicitly asked to include it.
- Pre-existing `?? docs/architecture/raft-ha-implementation-summary.ko.md` may still be present and must remain unstaged unless the user explicitly asked to include it.

- [ ] **Step 2: Run unit tests**

Run:

```bash
go test ./...
```

Expected:

- Exit code `0`.

- [ ] **Step 3: Run compose syntax check**

Run:

```bash
docker compose -f composes/raft-ha-cluster/compose.yaml config
```

Expected:

- Exit code `0`.

- [ ] **Step 4: Run full smoke test from clean state**

Run:

```bash
scripts/raft-ha-cluster-smoke.sh
```

Expected:

- Exit code `0`.
- Output includes:
  - `bootstrap checks passed`
  - `join and replication checks passed`
  - `negative checks passed`
  - `failover, rejoin, and persistence checks passed`

- [ ] **Step 5: Leave environment in deterministic state**

Run:

```bash
docker compose -p reverseproxy-raft-ha -f composes/raft-ha-cluster/compose.yaml down -v --remove-orphans
```

Expected:

- Exit code `0`.

- [ ] **Step 6: Final status**

Run:

```bash
git status --short
git log --oneline -8
```

Expected:

- Only pre-existing unrelated user changes remain unstaged.
- Recent commits include the compose scenario, configs, smoke script, docs.

---

## Self-Review

- Spec coverage: The plan covers 3 proxy nodes, 3 backend nodes, Docker Compose, node configs, seed config, smoke script, bootstrap, join, replication, follower rejection, join validation, failover, rejoin, persistence, and docs.
- Placeholder scan: No `TBD`, `TODO`, or vague implementation steps are intentionally left.
- Type consistency: Node IDs are `node-1`, `node-2`, `node-3`; service names are `proxy-1`, `proxy-2`, `proxy-3`; host ports are `18080`-`18082`, `19090`-`19092`, `17001`-`17003`.
- Scope check: This is one coherent test environment feature. It does not include production auth, membership removal, or `/api/raft/status`; those remain out of scope.
