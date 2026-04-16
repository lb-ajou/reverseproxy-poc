# 작업 템플릿

작업을 시작할 때 아래 형식으로 목표와 완료 조건을 정리한다.

기본 저장 위치는 `plan/tasks/<date>-<slug>.md`다. `plan/`은 gitignore 대상이므로 로컬 작업 메모와 면제 근거를 안전하게 둘 수 있다.

예:

- `plan/tasks/2026-04-16-dashboard-route-fix.md`
- `plan/tasks/2026-04-16-harness-bootstrap.md`

```md
# 작업 제목

## 목표

- 이번 작업으로 달성할 사용자/시스템 목표

## 배경

- 왜 이 변경이 필요한지
- 현재 동작이나 제약

## 성공 기준

- 완료로 간주할 구체 조건
- 눈으로 확인 가능한 기대 결과

## 비범위

- 이번 작업에서 하지 않을 것

## 제약

- 성능, 호환성, 배포, 보안, 운영상 제약

## 요구사항 명확성

- Requirements-Clarity: clear | unclear
- Clarification-Status: resolved | pending
- Clarification-Questions: 사용자에게 추가로 물어본 내용 또는 none
- Clarification-Answer: 사용자 답변 요약 또는 none

## 가정 및 승인

- Assumptions-Used: no | yes
- Assumption-Notes: 사용한 가정 또는 none
- Assumption-Approval: not-required | approved | pending
- Approval-Evidence: 사용자 승인 근거 또는 none

## 코드 컨벤션 영향

- Function-Length-Exception: no | yes
- Function-Length-Notes: 15줄 초과 함수가 필요한 이유 또는 none
- Function-Length-Approval: not-required | approved | pending
- Function-Length-Evidence: 사용자 승인 근거 또는 none

## 구현 계획 및 승인

- Implementation-Plan-Status: drafted | pending | approved
- Implementation-Step-Status: planning-only | awaiting-user-approval | approved-for-implementation
- Planned-Files: 추가/수정할 파일 목록 또는 none
- Approved-Files: 사용자 승인을 받은 파일 목록 또는 none
- Planned-Symbols: 추가/수정할 함수, 메서드, 타입 목록 또는 none
- Approved-Symbols: 사용자 승인을 받은 함수, 메서드, 타입 목록 또는 none
- Planned-Behavior: 구현할 동작 요약 또는 none
- Planned-Tests: 추가/수정할 테스트 또는 none
- Implementation-Approval: approved | pending
- Implementation-Approval-Evidence: 사용자 승인 근거 또는 none

## 수정 예상 영역

- 예상 변경 패키지/문서/설정

## 테스트 계획

- fast 게이트에서 확인할 내용
- full 게이트나 수동 검증이 필요한 내용

## 문서 영향

- Docs-Impact: update | none
- Docs-Targets: docs/architecture/architecture.ko.md, docs/api/dashboard-api.ko.md
- Docs-Reason: 문서 수정 또는 면제 근거
```

## 최소 규칙

- `Requirements-Clarity`와 `Clarification-Status`는 반드시 채운다.
- `Requirements-Clarity: unclear`이면 `Clarification-Status: resolved`와 질문/답변 기록이 있어야 한다.
- `Assumptions-Used`는 반드시 채운다.
- `Assumptions-Used: yes`이면 `Assumption-Approval: approved`와 승인 근거가 있어야 한다.
- `Function-Length-Exception`은 반드시 채운다.
- `Function-Length-Exception: yes`이면 `Function-Length-Approval: approved`와 승인 근거가 있어야 한다.
- `Implementation-Plan-Status`, `Implementation-Step-Status`, `Implementation-Approval`은 반드시 채운다.
- `Implementation-Plan-Status: drafted` 또는 `pending`이면 `Implementation-Approval: pending`이어야 하며 코드를 작성하면 안 된다.
- `Implementation-Step-Status: planning-only` 또는 `awaiting-user-approval`이면 코드를 작성하면 안 된다.
- 실제 구현을 진행하려면 `Implementation-Plan-Status: approved`, `Implementation-Step-Status: approved-for-implementation`, `Implementation-Approval: approved`가 모두 필요하다.
- `Approved-Files`는 실제 수정 전에 사용자 승인을 받은 파일 목록이어야 한다.
- `Approved-Symbols`는 실제 수정 전에 사용자 승인을 받은 함수/메서드/타입 목록이어야 한다.
- `Implementation-Approval: approved`이면 승인 근거를 반드시 남긴다.
- `Docs-Impact`는 반드시 채운다.
- `Docs-Impact: none`이면 `Docs-Reason`을 반드시 적는다.
- 구조 변경, API 변경, 패키지 책임 변경은 기본적으로 `Docs-Impact: update`다.
- 여러 작업 파일이 있으면 하네스는 가장 최근 수정 파일을 기본으로 본다.
- 다른 파일을 기준으로 검증하려면 `TASK_FILE=plan/tasks/<file>.md scripts/agent-check.sh fast`처럼 명시한다.
