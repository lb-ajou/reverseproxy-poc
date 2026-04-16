# 개발 하네스 가이드

이 문서는 이 저장소의 기본 개발 하네스를 설명한다.

## 목표

- 사람과 에이전트가 같은 검증 절차를 사용한다.
- 로컬에서 빠르게 돌릴 수 있는 기본 게이트를 제공한다.
- 문서 동기화와 커밋 메시지 규칙을 기본 흐름에 포함한다.

## 시작 순서

1. `docs/harness/task-template.md`를 기준으로 `plan/tasks/<date>-<slug>.md`를 작성한다.
2. 구현을 진행한다.
3. `scripts/agent-check.sh fast`로 기본 검증을 돌린다.
4. 필요하면 `scripts/agent-check.sh full`로 소켓 의존 테스트까지 확인한다.
5. `scripts/install-hooks.sh`로 훅을 설치한다.
6. 커밋은 `scripts/agent-commit.sh "type: 한글 요약"`으로 진행한다.

## 검증 단계

### fast

- `GOCACHE`를 `/tmp/go-build-cache`로 강제한다.
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
