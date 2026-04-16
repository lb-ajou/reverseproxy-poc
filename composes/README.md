# Compose Test Environments

`composes/`는 리버스프록시를 로컬에서 실행할 때 붙여볼 수 있는 백엔드 테스트 환경 모음이다.

이 디렉토리의 compose들은 프록시 앱 자체를 띄우지 않는다. 프록시는 로컬에서 실행하고, compose는 외부 접근 가능한 백엔드 서버들과 health check 대상만 제공한다.

## 공통 규칙

- 모든 서버는 호스트 포트로 직접 접근 가능하다.
- 모든 서버는 `/health`와 `/api/info`를 제공한다.
- Docker Compose 자체 healthcheck도 `/health` 기준으로 설정한다.
- 시나리오별 프록시 설정 예시는 각 하위 디렉토리 `README.md`에 적어 둔다.

## 시나리오 목록

- `route-basic`
  - 서로 다른 두 서버를 직접 라우팅 대상으로 쓰는 기본 시나리오
- `lb-multi-upstream`
  - 같은 역할의 여러 서버를 하나의 upstream pool로 묶는 시나리오
- `failure-healthcheck`
  - 일부 서버가 `/health`에서 실패하는 상태를 재현하는 시나리오
- `round-robin-check`
  - round-robin 분산을 자동 스크립트로 검증하는 시나리오
- `sticky-cookie-check`
  - sticky cookie 기반 라우팅을 실제 cookie jar로 검증하는 시나리오

## 실행 예시

```bash
docker compose -f composes/route-basic/compose.yaml up -d
docker compose -f composes/route-basic/compose.yaml down
```

## 공용 테스트 서버 API

### `GET /health`

- 정상 서버: `200 OK`
- 실패 시나리오 서버: `HEALTH_STATUS` 환경변수에 지정한 상태 반환

### `GET /api/info`

아래 형태의 JSON을 반환한다.

```json
{
  "server": "route-alpha",
  "scenario": "route-basic",
  "hostname": "container-hostname",
  "port": "8080",
  "version": "v1",
  "health_status": 200
}
```

## 로컬 프록시와 함께 테스트하는 기본 순서

1. 원하는 compose 시나리오를 띄운다.
2. 해당 시나리오 `README`의 예시를 참고해 로컬 `configs/app.json`, `configs/proxy/*.json`을 맞춘다.
3. 로컬에서 리버스프록시 앱을 실행한다.
4. 프록시 주소와 백엔드 주소를 각각 호출해 동작을 비교한다.
5. 필요하면 대시보드 `GET /api/upstreams`, `GET /api/runtime/routes`로 현재 상태를 확인한다.
