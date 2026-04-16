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

- `Docs-Impact`는 반드시 채운다.
- `Docs-Impact: none`이면 `Docs-Reason`을 반드시 적는다.
- 구조 변경, API 변경, 패키지 책임 변경은 기본적으로 `Docs-Impact: update`다.
- 여러 작업 파일이 있으면 하네스는 가장 최근 수정 파일을 기본으로 본다.
- 다른 파일을 기준으로 검증하려면 `TASK_FILE=plan/tasks/<file>.md scripts/agent-check.sh fast`처럼 명시한다.
