# AGENTS.md

이 문서는 이 저장소에서 작업하는 에이전트를 위한 루트 진입 안내서다.

상세 규칙 원문은 아래 문서를 따른다.

- `docs/harness/agent-contract.md`
- `docs/harness/task-template.md`
- `docs/harness/harness.md`

## 시작 순서

1. `docs/harness/task-template.md`를 보고 `plan/tasks/<date>-<slug>.md`를 작성한다.
2. 요구사항이 모호하면 구현 전에 반드시 사용자에게 질문한다.
3. 요구사항이 부족해 가정이 필요하면 작업 파일에 기록하고 사용자 승인을 받는다.
4. 실제 코드를 쓰기 전에 어떤 파일, 함수, 메서드, 타입을 추가/수정할지 사용자에게 설명하고 승인받는다.
5. 구현 후 `scripts/agent-check.sh fast`를 실행한다.
6. 필요하면 `scripts/agent-check.sh full`을 실행한다.
7. 커밋은 `scripts/agent-commit.sh "type(scope): 한글 요약"`을 기본 경로로 사용한다.

## 필수 규칙

- 모호한 요구사항은 질문 없이 구현하지 않는다.
- 사용자 승인 없이 가정을 확정하지 않는다.
- 사용자 승인 없이 구현 계획을 실행하지 않는다.
- 새로 수정하는 Go 함수/메서드는 가능한 한 15줄을 넘기지 않는다.
- 하네스 규칙이나 검증 로직이 비대해지면 책임별로 분리한다.
- 구조, API, 타입 의미가 바뀌면 관련 `docs/` 문서를 갱신한다.

## 작업 파일 체크리스트

작업 파일에는 최소한 아래 상태가 있어야 한다.

- `Requirements-Clarity`
- `Clarification-Status`
- `Assumptions-Used`
- `Assumption-Approval`
- `Function-Length-Exception`
- `Function-Length-Approval`
- `Implementation-Plan-Status`
- `Implementation-Approval`
- `Docs-Impact`

## 검증 명령

```bash
scripts/agent-check.sh fast
scripts/agent-check.sh full
scripts/agent-commit.sh "feat(scope): 한글 요약"
```
