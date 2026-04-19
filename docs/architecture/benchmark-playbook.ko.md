# 벤치마크 플레이북

이 문서는 `composes/benchmark-check` 시나리오에서 리버스프록시와 비교군 프록시 성능을 어떻게 측정할지 정리한다.

## 목표

- `wrk`로 포화 지점과 최대 처리량 후보를 찾는다.
- `vegeta`로 고정 RPS에서 `p95/p99`, `error rate`를 비교한다.
- `k6`로 램프업, 유지, 스파이크 구간에서의 안정성을 본다.
- `docker stats`로 프록시 컨테이너의 `CPU/메모리`를 함께 기록한다.
- 동일한 백엔드 풀에 대해 `reverseproxy`, `Caddy`, `Nginx`, `HAProxy`를 round robin 기준으로 비교한다.

## 도구별 역할

### `wrk`

- 질문: 프록시가 어디서 포화되기 시작하는가
- 방식: thread와 connection을 높여 가며 짧은 구간 부하를 준다
- 핵심 지표: `requests/sec`, 평균 latency, tail latency, timeout 징후
- 해석: `RPS` 증가가 둔화되는데 `p99`가 급등하면 포화 구간 후보로 본다

권장 기본값:

- duration: `30s`
- threads: `4`
- connections: `50,100,200,400`

### `vegeta`

- 질문: 특정 RPS를 고정했을 때 얼마나 안정적인가
- 방식: 여러 단계의 rate를 고정하고 각 구간을 일정 시간 유지한다
- 핵심 지표: 실제 throughput, `p95`, `p99`, `error rate`, status code 분포
- 해석: 허용 가능한 `p99`와 `error rate`를 만족하는 최대 rate를 운영 가능한 최고 성능으로 본다

권장 기본값:

- rates: `300,600,900,1200`
- duration: `60s`

### `k6`

- 질문: 램프업과 스파이크가 있을 때 성능이 어떻게 흔들리는가
- 방식: steady rate를 만든 뒤 spike를 올리고 다시 내려온다
- 핵심 지표: 시나리오 단계별 latency 변화, 실패율, threshold 충족 여부
- 해석: steady 상태는 괜찮아도 spike에서 무너지면 운영 위험으로 본다

권장 기본값:

- ramp up: `30s`
- steady: `60s`
- spike: `30s`
- peak rate: `1200`

### `docker stats`

- 질문: 부하 동안 프록시가 CPU와 메모리를 얼마나 쓰는가
- 방식: 실험 시간 동안 샘플링한 CSV를 남긴다
- 핵심 지표: 평균 CPU, 피크 CPU, 평균 메모리, 피크 메모리, pids
- 해석: latency 상승과 동시에 CPU가 고정되면 CPU 포화 가능성이 높다. 메모리가 선형 증가하면 버퍼링이나 누수 가능성을 의심한다.

## 권장 실험 순서

1. `docker compose -f composes/benchmark-check/compose.yaml up -d --build`
2. `BENCHMARK_TARGET=proxy`, `caddy`, `nginx`, `haproxy` 순서로 대상 프록시를 바꿔 같은 절차를 반복
3. `tools/benchmark-wrk.sh`로 connection 단계별 한계점 탐색
4. `tools/benchmark-vegeta.sh`로 후보 구간 재측정
5. `tools/benchmark-k6.sh`로 운영형 시나리오 검증
6. 각 단계마다 `tools/benchmark-stats.sh`를 병행 실행

예시:

```bash
BENCHMARK_TARGET=nginx tools/benchmark-stats.sh 120 2 &
BENCHMARK_TARGET=nginx tools/benchmark-wrk.sh 30s 4 50,100,200,400
wait
```

```bash
BENCHMARK_TARGET=caddy tools/benchmark-stats.sh 240 2 &
BENCHMARK_TARGET=caddy tools/benchmark-vegeta.sh 400,800,1200,1600 90s
wait
```

```bash
BENCHMARK_TARGET=haproxy tools/benchmark-stats.sh 180 2 &
BENCHMARK_TARGET=haproxy tools/benchmark-k6.sh 30s 60s 30s 1600
wait
```

## 결과 정리 기준

결과 비교 표는 아래 열을 기본으로 둔다.

- tool
- scenario
- target rate or connections
- actual rps
- p95
- p99
- error rate
- avg cpu
- peak cpu
- avg memory
- peak memory
- notes

## 현재 시나리오 특성

- 프록시는 `round_robin`으로 `backend-a`, `backend-b`, `backend-c`에 분산한다.
- 각 백엔드는 `5ms` 응답 지연을 넣어 프록시 overhead와 tail latency 변화를 관찰하기 쉽게 한다.
- 기본 엔드포인트는 `GET /api/info`다.
- 기본 Host 헤더는 `benchmark.localtest.me`다.
- 기본 포트는 `reverseproxy=18080`, `caddy=18081`, `nginx=18082`, `haproxy=18083`다.

## 본측정 결과

아래 표는 `2026-04-19`에 `plan/benchmarks/final/` 아래 산출된 결과를 요약한 것이다.

### `wrk` 15s, 4 threads, `50/100/200` connections

| Target | 50 conn | 100 conn | 200 conn | 메모 |
| --- | ---: | ---: | ---: | --- |
| `reverseproxy` | `1622.79 RPS / 31.79ms` | `2671.52 / 41.02ms` | `3235.09 / 69.31ms` | 처리량이 가장 낮고 connection 증가에 따라 평균 latency 상승폭이 큼 |
| `caddy` | `2322.84 / 22.52ms` | `5871.89 / 20.69ms` | `6979.00 / 33.28ms` | 높은 처리량과 안정적인 평균 latency를 보임 |
| `nginx` | `3199.82 / 15.46ms` | `3195.77 / 32.18ms` | `3035.18 / 81.49ms` | `100` connection 이후 처리량이 거의 늘지 않았고 timeout `2`회 발생 |
| `haproxy` | `5246.96 / 10.02ms` | `6717.92 / 16.96ms` | `12081.42 / 18.96ms` | 최대 처리량과 평균 latency 모두 가장 강하게 나옴 |

### `vegeta` 30s, `300/600/900 RPS`

| Target | 300 RPS | 600 RPS | 900 RPS | 메모 |
| --- | --- | --- | --- | --- |
| `reverseproxy` | `p95 6.99ms / p99 13.13ms` | `p95 86.15ms / p99 236.32ms` | `p95 551.17ms / p99 760.85ms` | `600 RPS`부터 tail latency가 크게 증가 |
| `caddy` | `p95 7.48ms / p99 12.51ms` | `p95 15.43ms / p99 55.34ms` | `p95 29.45ms / p99 222.40ms` | 고정 RPS 안정성이 가장 좋음 |
| `nginx` | `p95 10.11ms / p99 18.86ms` | `p95 1.051s / p99 1.496s` | `p95 313.16ms / p99 483.77ms` | `600 RPS` 구간에서 크게 흔들림 |
| `haproxy` | `p95 14.54ms / p99 26.20ms` | `p95 22.67ms / p99 240.48ms` | `throughput 830.76 RPS`, `p95 363.33ms / p99 590.85ms` | `900 RPS`에서 목표 처리율을 유지하지 못함 |

### `k6` 15s ramp, 30s steady, 15s spike, peak `900`

| Target | Req Rate | Avg | p95 | Max | Dropped | 메모 |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| `reverseproxy` | `483.23/s` | `9.41ms` | `23.65ms` | `164.16ms` | `34` | spike 구간에서 dropped iterations가 발생 |
| `caddy` | `483.65/s` | `6.57ms` | `7.96ms` | `100.67ms` | `0` | 운영형 시나리오에서도 매우 안정적 |
| `nginx` | `483.64/s` | `6.37ms` | `7.65ms` | `46.77ms` | `0` | `vegeta`와 달리 이 시나리오에서는 매우 안정적 |
| `haproxy` | `483.70/s` | `6.04ms` | `6.64ms` | `33.03ms` | `0` | 운영형 시나리오 기준으로 가장 우수 |

## 해석 요약

- 최대 처리량은 `HAProxy`가 가장 강했다.
- 고정 RPS 안정성은 `Caddy`가 가장 좋았다.
- 운영형 시나리오는 `HAProxy`, `Nginx`, `Caddy`가 모두 안정적이었고, `reverseproxy`는 상대적으로 불리했다.
- 현재 `reverseproxy`는 비교군 대비 `vegeta 600+ RPS`와 `k6` spike 구간에서 최적화 여지가 있다.

## 측정 의미

- 이 문서의 `RPS`는 `클라이언트 -> 프록시 -> 업스트림 -> 프록시 -> 클라이언트` 전체 왕복이 끝난 뒤의 응답 기준이다.
- 즉, 업스트림 서버의 `GET /api/info`가 실제로 `200 OK`를 반환하고, 프록시가 그 응답을 받아 다시 클라이언트에 전달한 요청만 집계된다.
- `health check` 트래픽이나 프록시 내부에서만 끝나는 가상 응답은 포함되지 않는다.
- 현재 벤치마크 시나리오는 각 업스트림에 `SLEEP_MS=5`를 넣어 두었기 때문에, 결과에는 백엔드 처리시간 `5ms`와 프록시 오버헤드가 함께 반영된다.
- 따라서 이 문서의 수치는 순수 프록시 단독 성능이라기보다, `5ms` 응답 지연을 가진 업스트림을 둔 end-to-end 성능 비교 결과로 해석해야 한다.
- 다만 `RPS`는 일반적으로 `200 OK`만의 처리량을 뜻하지 않는다. 도구에 따라 `400`, `500` 같은 비정상 상태코드도 완료된 응답이면 처리량 집계에 포함될 수 있다.
- 그래서 성능 비교는 `RPS`만 단독으로 보지 않고, 반드시 `error rate`, `success ratio`, `status code` 분포를 함께 해석해야 한다.
- 이 문서의 비교 결과는 각 도구에서 `Status Codes`, `Success`, `http_req_failed`, `checks`를 함께 확인해 `200 응답 중심의 정상 처리`가 유지되는지 같이 검토한 값이다.

### 도구별 해석 기준

| Tool | 처리량 지표 | 같이 봐야 하는 지표 | 해석 포인트 |
| --- | --- | --- | --- |
| `wrk` | `Requests/sec` | `Latency`, `Socket errors`, timeout 여부 | `Requests/sec`만으로는 정상 응답 비율을 확정할 수 없으므로 latency 악화와 socket error를 함께 본다 |
| `vegeta` | `throughput` | `Success [ratio]`, `Status Codes`, `Error Set`, `p95/p99` | 고정 RPS를 준 상태에서 실제 처리율과 성공률이 유지되는지 본다 |
| `k6` | `http_reqs`, `iterations` | `http_req_failed`, `checks`, threshold, `p95/p99` | 시나리오형 부하에서 처리량뿐 아니라 SLA 기준을 만족하는지 본다 |
