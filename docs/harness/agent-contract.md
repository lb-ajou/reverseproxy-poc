# 개발 하네스 작업 계약

이 문서는 이 저장소에서 사람과 코딩 에이전트가 공통으로 따라야 하는 기본 작업 계약을 정의한다.

목표는 다음 네 가지다.

- 요구사항과 성공 기준을 먼저 고정한다.
- 구현 변경이 저장소 구조와 충돌하지 않게 한다.
- 테스트와 문서 동기화를 기본 경로에 포함한다.
- 커밋 직전 게이트를 일관되게 적용한다.

## 기본 원칙

- 작업 시작 전에는 `docs/harness/task-template.md` 형식으로 작업 의도를 정리한다.
- 로컬 작업 스펙 파일의 기본 위치는 `plan/tasks/*.md`다.
- 하네스는 기본적으로 `plan/tasks/` 아래에서 가장 최근에 수정된 Markdown 파일을 현재 작업 스펙으로 사용한다.
- 특정 작업 파일을 강제로 지정하려면 `TASK_FILE=plan/tasks/<file>.md`로 실행한다.
- 커밋 메시지는 Conventional Commit 형식을 따르되, 요약은 한글로 작성한다.
- 구조, API, 책임 경계가 바뀌면 관련 `docs/` 문서도 함께 갱신한다.
- 커밋 전에는 반드시 `scripts/agent-check.sh fast`를 통과한다.
- 커밋은 가능하면 `scripts/agent-commit.sh`를 사용한다.

## 변경 범위 규칙

- 구현 코드는 기존 패키지 책임을 따른다.
- 새로운 패키지나 디렉토리를 추가할 때는 먼저 기존 책임 분리를 재사용할 수 있는지 검토한다.
- `main.go`에는 wiring 이외의 상세 비즈니스 로직을 넣지 않는다.
- `internal/` 아래의 패키지 책임이 의미 있게 바뀌면 `docs/conventions/directory-convention.ko.md` 또는 `docs/architecture/architecture.ko.md`를 갱신한다.

## 테스트 규칙

- 동작 변경이 있으면 관련 테스트를 추가하거나 기존 테스트를 보강한다.
- 기본 검증 명령은 `scripts/agent-check.sh fast`다.
- 소켓 의존 테스트까지 확인해야 할 경우 `scripts/agent-check.sh full`을 사용한다.
- 포맷 검사는 자동 수정이 아니라 실패 보고를 우선한다.

## 커밋 메시지 규칙

- 형식은 Conventional Commit을 사용한다.
- 허용 타입은 `feat`, `fix`, `docs`, `refactor`, `test`, `chore`다.
- scope는 선택 사항이며 영문 식별자를 써도 된다.
- 콜론 뒤 요약은 한글로 작성한다.

예:

```text
feat(harness): 로컬 개발 하네스 추가
fix(dashboard): 정적 파일 경로 검증 오류 수정
docs(api): 대시보드 네임스페이스 API 설명 보강
```

## 문서 동기화 규칙

- `internal/admin`, `internal/dashboard` 변경은 `docs/api/dashboard-api.ko.md` 점검 대상이다.
- 구조나 책임 변화는 `docs/conventions/directory-convention.ko.md` 또는 `docs/architecture/architecture.ko.md` 점검 대상이다.
- 타입 의미나 용어 계약이 바뀌면 `docs/conventions/type-reference.ko.md` 점검 대상이다.
- 문서 수정이 불필요하다고 판단하면 현재 작업 스펙 파일에 이유를 남긴다.

권장 형식:

```md
## 문서 영향

- Docs-Impact: none
- Docs-Reason: 내부 리팩터링만 포함하며 공개 계약과 패키지 책임은 바뀌지 않음
```

## 금지 행동

- 검증 실패를 무시한 채 기본 경로로 커밋하지 않는다.
- 사용자 변경을 되돌리기 위해 무단으로 `git reset --hard` 같은 파괴적 명령을 쓰지 않는다.
- 문서 갱신이 필요한 변경을 코드만 수정한 채 끝내지 않는다.
- `internal/dashboard/static/index.html` 같은 필수 런타임 자산의 존재를 가정만 하고 넘어가지 않는다.
