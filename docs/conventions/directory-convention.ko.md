# 디렉토리 컨벤션

## 목적

이 문서는 현재 저장소의 디렉토리 구조, 패키지 책임, 의존성 경계를 설명하기 위한 문서다.

대상 독자는 두 부류다.

- 코드를 읽고 유지보수하는 사람
- 이후 이 저장소를 다시 읽으며 구조 의도를 빠르게 파악해야 하는 코딩 에이전트

디렉토리의 책임이나 소유 범위가 의미 있게 바뀌면 이 문서도 함께 갱신하는 것을 원칙으로 한다.

## 프로젝트 목표

이 저장소는 리버스 프록시 POC다.

현재 구현 방향은 아래와 같다.

- `configs/app.json`에서 앱 레벨 부트 설정 로드
- `configs/proxy/*.json`에서 리버스 프록시 설정 파일 전체 로드
- 서버 시작 시 설정 검증 수행
- 하나의 전역 route table 생성
- 하나의 전역 upstream registry 생성
- 활성 상태는 메모리 snapshot으로 유지
- 요청은 현재 runtime snapshot을 기준으로 프록시

현재 단계에서 범위 밖인 것:

- 자동 reload
- 파일 watch
- 설정 저장 / 백업
- runtime health check 실행
- dashboard 쓰기 API

## 최상위 구조

### `go.mod`

단일 Go 모듈 정의 파일이다.

규칙:

- POC 단계에서는 하나의 모듈만 유지한다
- 특별한 이유가 없으면 멀티 모듈로 쪼개지 않는다
- 실제 구현은 `internal/` 아래에 둔다

### `main.go`

실행 진입점이다.

역할:

- 설정 파일 경로 결정
- 로거 초기화
- 앱 설정 로드
- app 생성
- 서버 실행

규칙:

- `main.go`는 얇게 유지한다
- 라우팅 정책, runtime 조립, 세부 비즈니스 로직은 넣지 않는다

### `configs/`

애플리케이션이 읽는 설정 파일 디렉토리다.

현재 구조:

- `configs/app.json`
- `configs/proxy/*.json`

의도:

- `app.json`은 서버 부트 설정
- `proxy/*.json`은 리버스 프록시 desired state 문서

### `docs/`

사람이 읽을 수 있는 구조, 설계, 하네스 문서의 공식 루트다.

의도:

- 프로젝트 구조 의도 보존
- 이후 참여자가 패키지 경계를 빠르게 이해할 수 있게 함
- 코딩 에이전트가 구조 컨텍스트를 복구할 수 있게 함

현재 예:

- `docs/architecture/architecture.ko.md`
- `docs/api/dashboard-api.ko.md`
- `docs/conventions/directory-convention.ko.md`
- `docs/conventions/type-reference.ko.md`
- `docs/harness/agent-contract.md`
- `docs/harness/harness.md`

참고:

- 기존 `convention/` 디렉토리는 이전 문서 경로와의 호환을 위해 남아 있을 수 있다

### `plan/`

작업 계획과 임시 메모 디렉토리다.

상태:

- gitignore 대상
- 영구 컨벤션 문서가 아니라 작업 중 참고용 문서 보관 용도

권장 디렉토리:

- `plan/tasks/`

의도:

- `docs/harness/task-template.md`를 기준으로 작업별 스펙 파일 보관
- 현재 작업의 목표, 테스트 계획, 문서 영향 기록

### `scripts/`

로컬 개발 하네스와 저장소 보조 명령을 둔다.

현재 예:

- `scripts/agent-check.sh`
- `scripts/agent-commit.sh`
- `scripts/install-hooks.sh`
- `scripts/validate-commit-msg.sh`

규칙:

- 하네스 검증 로직이 커지면 책임별 보조 스크립트로 분리한다
- 상위 진입점은 유지하되, 세부 검사 로직을 한 파일에 과도하게 누적하지 않는다

### `.githooks/`

저장소 전용 Git hook 스크립트를 둔다.

현재 예:

- `.githooks/pre-commit`
- `.githooks/commit-msg`

## `internal/` 패키지 의도

실제 구현은 모두 `internal/` 아래에 둔다.

공통 규칙:

- 핵심 로직은 `internal/` 아래에 둔다
- 패키지 이름은 짧고 책임이 드러나야 한다
- `utils`, `helpers`, `common` 같은 모호한 이름은 피한다

## 패키지별 책임

### `internal/app`

애플리케이션 wiring과 시작 orchestration 계층이다.

역할:

- config 로드와 runtime 구성 연결
- runtime snapshot 생성
- proxy handler와 dashboard handler 생성
- HTTP 서버 생성
- 앱 실행 흐름 관리

여기에 둘 것:

- app 생성자
- startup wiring
- shutdown 흐름
- reload orchestration

여기에 두지 않을 것:

- 세부 라우팅 매칭 로직
- upstream balancing 로직
- 원본 설정 스키마 정의

### `internal/config`

앱 레벨 bootstrap 설정 전용 패키지다.

현재 역할:

- `AppConfig` 정의
- `configs/app.json` 로드
- 기본값 적용
- 앱 레벨 설정 검증

여기에 들어가는 예:

- proxy listen 주소
- dashboard listen 주소
- proxy config 디렉토리 경로

여기에 들어가면 안 되는 예:

- route 정의
- upstream pool 정의
- runtime health 상태

이유:

앱 부트 설정은 프록시 desired state보다 변경 주기가 훨씬 느리고, lifecycle 의미도 다르기 때문이다.

### `internal/proxyconfig`

리버스 프록시 설정 파일의 원본 스키마와 로딩을 담당한다.

현재 역할:

- `configs/proxy/*.json` 스키마 정의
- 파일 하나 또는 디렉토리 전체 로드
- 파일 단위 검증
- source 이름과 파일 경로 같은 메타데이터 유지

중요한 구분:

- 이 패키지는 설정 파일 표현을 담당한다
- runtime 라우팅 동작은 담당하지 않는다

### `internal/route`

runtime 라우팅 정책 계층이다.

현재 역할:

- `proxyconfig`의 route를 runtime route로 컴파일
- 전역 route ID 부여
- 전역 upstream pool 참조 부여
- regex 사전 컴파일
- 전역 route table 생성
- 우선순위 정렬
- 요청 host/path에 대한 route resolve

중요한 규칙:

- 모든 proxy config 파일의 route는 하나의 전역 route table로 합친다
- route 적용 순서는 JSON 배열 순서가 아니라 고정 우선순위 규칙을 따른다

현재 우선순위:

1. exact
2. prefix
3. regex
4. any

prefix 의미:

- 단순 문자열 prefix가 아니라 segment 기반 prefix다

### `internal/upstream`

runtime upstream registry와 balancing을 담당한다.

현재 역할:

- 모든 proxy config 파일의 upstream pool을 runtime pool로 컴파일
- 전역 pool ID 부여
- 전역 registry 생성
- pool 안에서 target 선택

현재 balancing:

- 단순 round-robin

중요한 구분:

- upstream pool의 설정 스키마는 `internal/proxyconfig`
- runtime registry와 target 선택은 `internal/upstream`

### `internal/runtime`

활성 메모리 상태를 관리하는 패키지다.

현재 역할:

- 현재 app config 보관
- 로드된 proxy config 메타데이터 보관
- 전역 route table 보관
- 전역 upstream registry 보관
- snapshot 읽기 제공
- snapshot 원자적 교체 지원

중요한 의도:

- runtime은 source of truth 대체물이 아니다
- runtime은 desired config를 컴파일한 활성 상태 뷰다

### `internal/proxy`

실제 리버스 프록시 요청 전달을 담당한다.

현재 역할:

- 현재 runtime snapshot 조회
- route table 기준 route resolve
- upstream target 선택
- 선택된 upstream으로 요청 전달

중요한 경계:

- `internal/proxy`는 라우팅 정책을 정의하지 않는다
- route/upstream이 내린 결정을 소비해서 전달만 담당한다

### `internal/dashboard`

현재 단계에서는 읽기 중심의 관리용 HTTP API를 담당한다.

현재 역할:

- 활성 설정과 runtime 상태 조회
- app config, proxy config, route, upstream 상태를 구조화된 응답으로 반환

현재 범위:

- 조회 API만 제공
- 설정 변경 API는 아직 없음

### `internal/middleware`

공통 HTTP 미들웨어를 담당한다.

현재 역할:

- 요청 로깅 같은 공통 미들웨어

규칙:

- 여러 핸들러에 공통인 HTTP 횡단 관심사만 둔다

## 의존성 방향

의도한 의존성 방향:

- `main` -> `app`
- `app` -> `config`, `proxyconfig`, `route`, `upstream`, `runtime`, `proxy`, `dashboard`
- `proxy` -> `runtime`, `route`
- `route` -> `proxyconfig`
- `upstream` -> `proxyconfig`
- `dashboard` -> `runtime`

서로 결합되면 안 되는 것:

- `route`는 `dashboard`를 알면 안 된다
- `upstream`은 `dashboard`를 알면 안 된다
- `config`는 HTTP/UI 패키지를 알면 안 된다

## 네임스페이스 규칙

각 proxy config 파일은 확장자를 제외한 파일명을 source 이름으로 가진다.

예:

- `configs/proxy/default.json` -> source `default`

전역 ID는 이 source를 붙여서 만든다.

- route ID: `<source>:<route.id>`
- upstream pool ID: `<source>:<pool.id>`

이유:

- 서로 다른 파일에서 같은 로컬 ID를 허용하기 위해서
- runtime에서는 전역 유일성을 보장하기 위해서

## 설계 의도 요약

이 코드베이스는 의도적으로 세 계층을 분리한다.

1. 파일 스키마 계층
2. runtime 정책 계층
3. 애플리케이션 wiring 계층

대응 관계:

- 파일 스키마 계층 -> `internal/config`, `internal/proxyconfig`
- runtime 정책 계층 -> `internal/route`, `internal/upstream`, `internal/runtime`
- 애플리케이션 wiring 계층 -> `internal/app`, `internal/proxy`, `internal/dashboard`

특별한 이유가 없다면 이 분리는 유지하는 것이 원칙이다.
