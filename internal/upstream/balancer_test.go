package upstream

import (
	"testing"
	"time"
)

func TestPoolNextTarget_RoundRobin(t *testing.T) {
	pool := newTestPool("10.0.0.11:8080", "10.0.0.12:8080")
	requireNextTargets(t, pool, "10.0.0.11:8080", "10.0.0.12:8080", "10.0.0.11:8080")
}

func TestPoolNextTarget_SkipsUnhealthyTargets(t *testing.T) {
	pool := newTestPool("10.0.0.11:8080", "10.0.0.12:8080", "10.0.0.13:8080")
	requireSetTargetUnhealthy(t, pool, 1, "connection refused")
	requireNextTargets(t, pool, "10.0.0.11:8080", "10.0.0.13:8080", "10.0.0.11:8080")
}

func TestPoolNextTarget_ReturnsFalseWhenNoHealthyTargets(t *testing.T) {
	pool := newTestPool("10.0.0.11:8080", "10.0.0.12:8080")
	requireSetTargetUnhealthy(t, pool, 0, "timeout")
	requireSetTargetUnhealthy(t, pool, 1, "timeout")
	if _, ok := pool.NextTarget(); ok {
		t.Fatal("NextTarget() returned healthy target when none should exist")
	}
}

func TestPoolHashTarget_ReusesStableTarget(t *testing.T) {
	pool := newTestPool("10.0.0.11:8080", "10.0.0.12:8080")
	first := requireHashTarget(t, pool, "client-a")
	second := requireHashTarget(t, pool, "client-a")
	if first.Raw != second.Raw {
		t.Fatalf("HashTarget() mismatch: %q != %q", first.Raw, second.Raw)
	}
}

func TestPoolHashTarget_SkipsUnhealthyTargets(t *testing.T) {
	pool := newTestPool("10.0.0.11:8080", "10.0.0.12:8080")
	requireSetTargetUnhealthy(t, pool, 0, "down")
	target := requireHashTarget(t, pool, "client-a")
	if got, want := target.Raw, "10.0.0.12:8080"; got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
}

func newTestPool(raws ...string) *Pool {
	targets := make([]Target, 0, len(raws))
	states := make([]TargetState, 0, len(raws))
	for _, raw := range raws {
		targets = append(targets, Target{Raw: raw})
		states = append(states, TargetState{Healthy: true})
	}
	return &Pool{Targets: targets, targetState: states}
}

func requireNextTargets(t *testing.T, pool *Pool, wants ...string) {
	t.Helper()
	for i, want := range wants {
		target, ok := pool.NextTarget()
		if !ok {
			t.Fatalf("NextTarget() returned no target on call %d", i+1)
		}
		if target.Raw != want {
			t.Fatalf("target on call %d = %q, want %q", i+1, target.Raw, want)
		}
	}
}

func requireSetTargetUnhealthy(t *testing.T, pool *Pool, index int, reason string) {
	t.Helper()
	if ok := pool.SetTargetUnhealthy(index, time.Now(), reason); !ok {
		t.Fatal("SetTargetUnhealthy() returned false")
	}
}

func requireHashTarget(t *testing.T, pool *Pool, key string) Target {
	t.Helper()
	target, ok := pool.HashTarget(key)
	if !ok {
		t.Fatal("HashTarget() returned no target")
	}
	return target
}
