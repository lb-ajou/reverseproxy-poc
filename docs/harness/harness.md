# 개발 하네스 가이드

이 문서는 이 저장소의 기본 개발 하네스를 설명한다.

## 목표

- 사람과 에이전트가 같은 검증 절차를 사용한다.
- 로컬에서 빠르게 돌릴 수 있는 기본 게이트를 제공한다.
- 문서 동기화와 커밋 메시지 규칙을 기본 흐름에 포함한다.
- 하네스 자체도 비대해지면 책임별로 분리한다.

## 공식 명령

### `/HARNESS-STRICT`

사용자가 작업 요청 앞에 `/HARNESS-STRICT`를 붙이면 하네스 강제 모드로 해석한다.

의미:

- 작업 파일 먼저 작성
- 질문/가정/구현 계획 승인 전 구현 금지
- 구현 후 `scripts/agent-check.sh fast`
- 커밋은 하네스 기본 경로 사용

사용 예시:

```text
/HARNESS-STRICT
대시보드 설정 저장 API를 수정해줘
```

## 시작 순서

1. `docs/harness/task-template.md`를 기준으로 `plan/tasks/<date>-<slug>.md`를 작성한다.
2. 요구사항이 모호하면 작업 파일의 `요구사항 명확성` 섹션에 질문과 답변 상태를 기록하고 사용자 확인을 먼저 받는다.
3. 임의 판단이 들어가면 `가정 및 승인` 섹션에 가정과 승인 근거를 기록한다.
4. `구현 계획 및 승인` 섹션에 어떤 파일과 함수/메서드를 작성할지 적고 사용자 승인을 받는다.
5. 구현을 진행한다.
6. `scripts/agent-check.sh fast`로 기본 검증을 돌린다.
7. 필요하면 `scripts/agent-check.sh full`로 소켓 의존 테스트까지 확인한다.
8. `scripts/install-hooks.sh`로 훅을 설치한다.
9. 커밋은 `scripts/agent-commit.sh "type: 한글 요약"`으로 진행한다.

`/HARNESS-STRICT` 요청은 위 순서를 예외 없이 따른다.

## 검증 단계

### fast

- `GOCACHE`를 `/tmp/go-build-cache`로 강제한다.
- 작업 파일의 요구사항 명확성/가정 승인 상태를 검사한다.
- 작업 파일의 구현 계획 작성 여부와 사용자 승인 상태를 검사한다.
- 스테이징된 Go 파일의 함수/메서드 길이 제한을 검사한다.
- `gofmt -l` 기반 포맷 검사를 수행한다.
- 소켓 의존 패키지를 제외한 기본 `go test`를 수행한다.
- 스테이징된 변경을 기준으로 문서 동기화가 필요한지 검사한다.
- `internal/dashboard/static/index.html` 존재 여부를 확인한다.

### full

- fast 단계의 모든 검사를 포함한다.
- `go test ./...`를 전체 실행한다.
- 현재 환경이 소켓 리스닝을 막으면 실패할 수 있다.

## 훅

- `pre-commit`: `scripts/agent-check.sh fast`
- `commit-msg`: `scripts/validate-commit-msg.sh`

`scripts/install-hooks.sh`는 `core.hooksPath`를 `.githooks`로 맞춘다.

커밋 메시지 규칙:

- Conventional Commit 형식 사용
- 요약은 한글 작성
- 예: `feat(harness): 로컬 개발 하네스 추가`

## 문서 동기화

다음 변경은 기본적으로 관련 문서 갱신이 필요하다.

- `internal/admin`, `internal/dashboard` 변경
- 패키지 책임이나 디렉토리 구조 변경
- 타입 의미나 API 계약 변경

문서 수정이 불필요하면 현재 작업 파일의 `문서 영향` 섹션에 그 이유를 남긴다.

기본적으로 하네스는 `plan/tasks/` 아래에서 가장 최근 수정된 작업 파일을 읽는다. 다른 작업 파일을 강제로 선택하려면 `TASK_FILE` 환경변수를 사용한다.

요구사항 명확성 규칙:

- 모호한 요구사항은 질문 없이 구현하지 않는다.
- 사용자 응답 전에는 `Clarification-Status: pending` 상태로 두고 구현을 멈춘다.
- 가정이 개입되면 `Assumption-Approval: approved`가 없으면 검증을 통과할 수 없다.

구현 계획 승인 규칙:

- 코드를 쓰기 전에 어떤 파일과 함수/메서드를 수정할지 사용자에게 설명한다.
- 작업 파일의 `구현 계획 및 승인` 섹션에 그 계획을 기록한다.
- `Implementation-Approval: approved`가 없으면 구현과 커밋을 진행하지 않는다.

코드 크기 규칙:

- 스테이징된 Go 파일의 함수/메서드는 가능한 한 15줄 이내로 유지한다.
- 15줄 초과가 필요하면 작업 파일에 예외 사유와 승인 근거를 남긴다.

하네스 구조 규칙:

- 하네스 규칙이 늘어나면 문서와 스크립트를 책임별로 분리한다.
- `scripts/agent-check.sh`가 너무 많은 검사를 떠안기 시작하면 보조 스크립트로 나눈 뒤 상위 진입점만 유지한다.
- `docs/harness/` 아래 문서도 요구사항, 구현 승인, 스타일, 운영 가이드처럼 주제별로 나눌 수 있어야 한다.
