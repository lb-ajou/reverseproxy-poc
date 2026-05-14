# Raft 설정 상태 운영 규칙

이 문서는 HA 모드에서 어떤 상태가 Raft로 복제되고, 어떤 상태가 노드 로컬에 남는지 운영자가 빠르게 확인하기 위한 기준이다.

## 핵심 규칙

- Raft는 desired proxy config만 관리한다. 즉 route, upstream pool 같은 프록시 설정의 목표 상태만 합의 대상이다.
- `configs/app.json`은 노드 로컬 설정으로 남는다. listen 주소, dashboard 주소, config store 선택, Raft bind/join 정보처럼 프로세스별로 달라질 수 있는 값은 Raft에 넣지 않는다.
- `configs/proxy/*.json`은 HA 모드에서 bootstrap, import, export, 개발 편의용 artifact로만 취급한다. 정상 운영 중 설정 쓰기의 source of truth는 Raft 상태다.
- 기존 Raft data dir이 있으면 JSON seed보다 Raft data dir의 상태가 우선한다. 재시작한 노드는 남아 있는 Raft log/snapshot에서 desired config를 복원한다.
- Join 모드는 로컬 JSON seed를 무시한다. 새 노드는 클러스터에 합류한 뒤 leader가 가진 Raft 상태를 따라간다.
- Health 상태와 `least_connection` 카운터는 로컬 노드 상태다. Raft로 복제하지 않으며 dashboard runtime API도 응답한 노드의 로컬 관측값을 보여준다.
- Follower에 설정 쓰기 요청이 도착하면 leader forward를 하지 않고 `409 Conflict`를 반환한다. JSON body에는 `code: "not_raft_leader"`와 가능한 경우 `leader_address`가 포함된다.
