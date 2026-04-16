# Dashboard API

`internal/dashboard`와 `internal/admin` 구현 기준으로, 대시보드가 사용하는 namespace 기반 설정 API를 정리한 문서다.

## 개요

- 대시보드는 SPA HTML 한 장을 `go:embed`로 포함해 서빙한다.
- `/api/...` 요청은 JSON API로 처리한다.
- `/api/`가 아닌 `GET`/`HEAD` 요청은 모두 같은 `index.html`을 반환한다.
- 설정 편집은 `internal/dashboard/config_api.go`가 받고, 실제 파일 저장과 reload는 `internal/admin/service.go`가 처리한다.
- 런타임 조회는 `internal/dashboard/runtime_api.go`가 담당한다.

관련 구현:

- `internal/dashboard/handler.go`
- `internal/dashboard/config_api.go`
- `internal/dashboard/runtime_api.go`
- `internal/dashboard/view.go`
- `internal/admin/service.go`

## Namespace 모델

namespace는 `configs/proxy/*.json` 파일명과 1:1로 대응한다.

- `configs/proxy/default.json` -> `default`
- `configs/proxy/admin.json` -> `admin`

즉 namespace는 UI상의 임의 그룹이 아니라, 실제 편집 대상 proxy config 파일이다.

규칙:

- route와 upstream pool은 namespace 내부 리소스다.
- route의 `upstream_pool`은 같은 namespace 안의 pool ID를 참조해야 한다.
- 저장은 해당 namespace 파일에만 반영된다.
- 저장 성공 뒤에는 항상 `ReloadFromFile()`로 전체 런타임 스냅샷을 다시 만든다.

namespace 이름은 `^[A-Za-z0-9._-]+$`만 허용한다.

## 기본 동작

### `default` namespace

`GET /api/namespaces`는 `default.json` 파일이 없어도 항상 `default`를 목록에 넣는다.

이 경우 응답 항목은 다음처럼 내려간다.

- `namespace: "default"`
- `exists: false`

### 존재하지 않는 namespace 조회

`GET /api/namespaces/{namespace}/config`는 파일이 없어도 `404`를 주지 않는다. 대신 빈 설정 뷰를 반환한다.

예:

```json
{
  "namespace": "default",
  "exists": false,
  "routes": [],
  "upstream_pools": {},
  "applied_at": "2026-04-13T00:00:00Z"
}
```

여기서 `applied_at`은 파일 시간이 아니라 현재 런타임 스냅샷의 적용 시각이다.

### 존재하지 않는 namespace에 대한 첫 저장

route 생성이나 upstream pool 생성은 namespace 파일이 없어도 동작할 수 있다. 서비스 계층이 빈 설정에서 시작해 파일을 새로 저장한다.

즉 `POST /api/namespaces/{namespace}/routes`와 `POST /api/namespaces/{namespace}/upstream-pools`는 첫 쓰기 시점에 파일을 만들 수 있다.

### 명시적 namespace 생성

`POST /api/namespaces`는 빈 namespace 파일을 미리 만들고 싶을 때 쓰는 API다. 필수는 아니지만, 사이드바의 "새 namespace 만들기" 같은 UI에는 직접 연결하기 좋다.

## Config API

모든 편집 API는 namespace 기준으로 동작한다.

### Namespace 목록/생성/삭제

- `GET /api/namespaces`
- `POST /api/namespaces`
- `DELETE /api/namespaces/{namespace}`

`GET /api/namespaces` 응답:

```json
{
  "items": [
    {
      "namespace": "default",
      "path": "configs/proxy/default.json",
      "exists": true,
      "route_count": 1,
      "upstream_pool_count": 1
    }
  ],
  "default_namespace": "default"
}
```

`POST /api/namespaces` 요청:

```json
{
  "namespace": "admin"
}
```

동작:

- 빈 config를 저장한다.
- 저장 후 `ReloadFromFile()`을 호출한다.
- reload가 실패하면 생성한 파일을 제거한다.

`DELETE /api/namespaces/{namespace}` 동작:

- 대상 파일을 삭제한다.
- 삭제 후 `ReloadFromFile()`을 호출한다.
- reload가 실패하면 원래 파일 내용을 복구한다.

### Namespace 설정 조회

- `GET /api/namespaces/{namespace}/config`

이 응답은 편집용 원본 config 뷰다. 즉 디스크에 저장되는 구조를 그대로 반영한다.

```json
{
  "namespace": "default",
  "exists": true,
  "routes": [],
  "upstream_pools": {},
  "applied_at": "2026-04-13T00:00:00Z"
}
```

### Route API

- `GET /api/namespaces/{namespace}/routes`
- `POST /api/namespaces/{namespace}/routes`
- `PUT /api/namespaces/{namespace}/routes/{id}`
- `DELETE /api/namespaces/{namespace}/routes/{id}`

생성/수정 body는 `proxyconfig.RouteConfig` 형식이다.

```json
{
  "id": "r-api",
  "enabled": true,
  "algorithm": "sticky_cookie",
  "match": {
    "hosts": ["api.example.com"],
    "path": {
      "type": "prefix",
      "value": "/api/"
    }
  },
  "upstream_pool": "pool-api"
}
```

주의:

- `PUT`에서는 URL의 `{id}`와 body의 `id`가 반드시 같아야 한다.
- route ID 중복 생성은 `409 Conflict`다.
- route 삭제/수정 대상이 없으면 `404 Not Found`다.
- `algorithm`은 `round_robin`, `sticky_cookie` 중 하나다.
- 생략하면 기본값은 `round_robin`이다.

검증 규칙 중 프론트에서 바로 알아야 할 부분:

- `match.hosts`는 최소 1개 이상이어야 한다.
- host는 빈 문자열일 수 없다.
- wildcard host는 현재 스키마에서 지원하지 않는다.
- `path.type`은 `exact`, `prefix`, `regex`만 허용한다.
- `exact`와 `prefix`는 `/`로 시작해야 한다.
- `prefix`는 `/` 또는 `/.../` 형태여야 한다.
- `algorithm`은 `round_robin`, `sticky_cookie`만 허용한다.
- `upstream_pool`은 같은 namespace 안에 이미 정의된 pool이어야 한다.

### Upstream Pool API

- `GET /api/namespaces/{namespace}/upstream-pools`
- `POST /api/namespaces/{namespace}/upstream-pools`
- `PUT /api/namespaces/{namespace}/upstream-pools/{id}`
- `DELETE /api/namespaces/{namespace}/upstream-pools/{id}`

생성 요청은 `id`와 pool 내용을 함께 보낸다.

```json
{
  "id": "pool-api",
  "upstreams": ["10.0.0.11:8080"],
  "health_check": {
    "path": "/health",
    "interval": "30s",
    "timeout": "3s",
    "expect_status": 200
  }
}
```

생성 응답은 아래 형태다.

```json
{
  "id": "pool-api",
  "pool": {
    "upstreams": ["10.0.0.11:8080"],
    "health_check": {
      "path": "/health",
      "interval": "30s",
      "timeout": "3s",
      "expect_status": 200
    }
  }
}
```

수정 요청 body는 `id` 없이 `proxyconfig.UpstreamPool`만 보낸다.

```json
{
  "upstreams": ["10.0.0.11:8080"],
  "health_check": {
    "path": "/health",
    "interval": "30s",
    "timeout": "3s",
    "expect_status": 200
  }
}
```

검증 규칙:

- `upstreams`는 최소 1개 이상이어야 한다.
- 각 upstream은 `host:port` 또는 `[ipv6]:port` 형식이어야 한다.
- `health_check.path`는 `/`로 시작해야 한다.
- `health_check.interval`, `health_check.timeout`은 Go duration 문자열이어야 한다. 예: `30s`, `1m`
- `health_check.expect_status`는 `100..599` 범위여야 한다.

삭제 제약:

- 해당 pool을 참조하는 route가 있으면 `409 Conflict`다.

## Runtime API

runtime API는 편집용 API가 아니라, 현재 프로세스에 적용된 활성 스냅샷 조회용 API다.

- `GET /api/runtime/config`
- `GET /api/runtime/routes`
- `GET /api/app-config`
- `GET /api/proxy-configs`
- `GET /api/upstreams`

route 조회 응답에는 `algorithm` 필드가 포함된다.

예:

```json
[
  {
    "global_id": "default:r-api",
    "local_id": "r-api",
    "source": "default",
    "enabled": true,
    "hosts": ["api.example.com"],
    "path": {
      "kind": "prefix",
      "value": "/"
    },
    "algorithm": "sticky_cookie",
    "upstream_pool": "default:pool-api"
  }
]
```

`sticky_cookie` 동작:

- 첫 요청은 pool의 현재 round-robin 선택 결과를 사용한다.
- 응답 시 route 단위 cookie를 내려준다.
- 이후 같은 cookie를 가진 요청은 같은 upstream target을 재사용한다.
- cookie가 가리키는 target이 unhealthy면 다른 healthy target을 선택하고 cookie를 갱신한다.

차이:

- `GET /api/namespaces/{namespace}/config`
  현재 namespace 파일의 편집용 원본 구조를 반환한다.
- `GET /api/runtime/routes`
  reload 후 메모리에 올라간 전체 route table을 반환한다.

즉 config API는 "파일 편집", runtime API는 "현재 적용 상태 확인" 용도다.

주요 응답 형태:

- `/api/runtime/config`: `app_config`, `proxy_configs`, `route_table`, `upstreams`, `applied_at`
- `/api/runtime/routes`: `global_id`, `local_id`, `source`, `enabled`, `hosts`, `path`, `upstream_pool`
- `/api/upstreams`: `global_id`, `local_id`, `source`, `targets`, `health_check`

현재 `/api/upstreams`는 pool의 target 주소와 health check 설정은 내려주지만, 각 target의 실시간 health state는 응답에 포함하지 않는다.

## 저장과 reload

설정 변경 흐름은 `internal/admin/service.go`에 모여 있다.

1. namespace 파일을 읽는다.
2. 메모리상의 `proxyconfig.Config`를 수정한다.
3. `Config.Validate()`로 전체 config를 검증한다.
4. 파일을 atomic write로 저장한다.
5. `ReloadFromFile()`로 앱 상태를 다시 로드한다.
6. reload가 실패하면 이전 파일 내용으로 롤백한다.

이 구조 때문에 dashboard handler는 HTTP 입출력만 담당하고, 파일 저장과 런타임 일관성은 서비스 계층이 보장한다.

## 요청/에러 규칙

JSON body는 다음 규칙으로 파싱한다.

- unknown field를 허용하지 않는다.
- body는 JSON 객체 하나만 포함해야 한다.

에러 응답은 `admin.APIError` 형식이다.

```json
{
  "message": "validation failed",
  "errors": [
    {
      "field": "routes[0].id",
      "message": "duplicate route id"
    }
  ]
}
```

주요 상태 코드:

- `400 Bad Request`
  잘못된 JSON, 잘못된 namespace 형식, route path/body ID 불일치
- `404 Not Found`
  존재하지 않는 namespace/route/upstream pool 삭제 또는 수정, 존재하지 않는 API 경로
- `405 Method Not Allowed`
  지원하지 않는 HTTP 메서드
- `409 Conflict`
  namespace/route/pool 중복 생성, 참조 중인 upstream pool 삭제 시도
- `422 Unprocessable Entity`
  config validation 실패
- `500 Internal Server Error`
  파일 읽기/쓰기 실패, decode 실패, reload 실패

## 프론트엔드에서 바로 쓰는 흐름

대시보드 UI는 아래 순서로 붙이면 된다.

1. `GET /api/namespaces`로 목록을 가져온다.
2. 현재 선택 namespace를 전역 상태로 관리한다.
3. 편집 조회는 `GET /api/namespaces/{namespace}/config`를 사용한다.
4. route/pool 생성, 수정, 삭제는 같은 namespace 경로 아래로 보낸다.
5. 새 파일을 미리 만들고 싶으면 `POST /api/namespaces`를 사용한다.
6. namespace 제거는 `DELETE /api/namespaces/{namespace}`를 사용한다.

## 검증 범위

현재 동작은 최소한 아래 테스트로 확인된다.

- `internal/dashboard/api_test.go`
  handler 레벨의 라우팅, 상태 코드, JSON 응답 형식
- `internal/admin/service_test.go`
  파일 저장, reload, rollback, namespace 기본 규칙
