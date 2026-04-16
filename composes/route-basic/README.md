# route-basic

기본 라우팅 시나리오다. 서로 다른 두 백엔드 서버를 띄우고, 로컬 프록시가 특정 host/path 요청을 어느 서버로 보내는지 확인한다.

## 실행

```bash
docker compose -f composes/route-basic/compose.yaml up -d
```

## 직접 접근 확인

```bash
curl http://localhost:18081/api/info
curl http://localhost:18082/api/info
curl http://localhost:18081/health
curl http://localhost:18082/health
```

기대 결과:

- `18081`은 `server: "route-alpha"`
- `18082`는 `server: "route-beta"`
- `/health`는 모두 `200 OK`

## 로컬 프록시 설정 예시

`configs/app.json`

```json
{
  "proxyListenAddr": ":8080",
  "dashboardListenAddr": ":9090",
  "proxyConfigDir": "configs/proxy"
}
```

예시 파일: `configs/proxy/route-basic.json`

```json
{
  "name": "route-basic",
  "routes": [
    {
      "id": "r-alpha",
      "enabled": true,
      "match": {
        "hosts": ["alpha.localtest.me"],
        "path": { "type": "prefix", "value": "/" }
      },
      "upstream_pool": "pool-alpha"
    },
    {
      "id": "r-beta",
      "enabled": true,
      "match": {
        "hosts": ["beta.localtest.me"],
        "path": { "type": "prefix", "value": "/" }
      },
      "upstream_pool": "pool-beta"
    }
  ],
  "upstream_pools": {
    "pool-alpha": {
      "upstreams": ["127.0.0.1:18081"],
      "health_check": {
        "path": "/health",
        "interval": "10s",
        "timeout": "3s",
        "expect_status": 200
      }
    },
    "pool-beta": {
      "upstreams": ["127.0.0.1:18082"],
      "health_check": {
        "path": "/health",
        "interval": "10s",
        "timeout": "3s",
        "expect_status": 200
      }
    }
  }
}
```

## 프록시 확인 예시

로컬에서 프록시를 실행한 뒤:

```bash
curl -H 'Host: alpha.localtest.me' http://localhost:8080/api/info
curl -H 'Host: beta.localtest.me' http://localhost:8080/api/info
```

기대 결과:

- 첫 요청은 `route-alpha`
- 둘째 요청은 `route-beta`

## 대시보드 확인

```bash
curl http://localhost:9090/api/runtime/routes
curl http://localhost:9090/api/upstreams
```

라우트와 upstream pool이 각각 `route-basic` 설정 기준으로 보이면 정상이다.
