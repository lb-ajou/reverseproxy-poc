package upstream

import (
	"testing"
	"time"
)

func TestPoolNextTarget_RoundRobin(t *testing.T) {
	pool := &Pool{
		Targets: []Target{
			{Raw: "10.0.0.11:8080"},
			{Raw: "10.0.0.12:8080"},
		},
		targetState: []TargetState{
			{Healthy: true},
			{Healthy: true},
		},
	}

	first, ok := pool.NextTarget()
	if !ok {
		t.Fatal("NextTarget() returned no target on first call")
	}
	second, ok := pool.NextTarget()
	if !ok {
		t.Fatal("NextTarget() returned no target on second call")
	}
	third, ok := pool.NextTarget()
	if !ok {
		t.Fatal("NextTarget() returned no target on third call")
	}

	if got, want := first.Raw, "10.0.0.11:8080"; got != want {
		t.Fatalf("first target = %q, want %q", got, want)
	}
	if got, want := second.Raw, "10.0.0.12:8080"; got != want {
		t.Fatalf("second target = %q, want %q", got, want)
	}
	if got, want := third.Raw, "10.0.0.11:8080"; got != want {
		t.Fatalf("third target = %q, want %q", got, want)
	}
}

func TestPoolNextTarget_SkipsUnhealthyTargets(t *testing.T) {
	pool := &Pool{
		Targets: []Target{
			{Raw: "10.0.0.11:8080"},
			{Raw: "10.0.0.12:8080"},
			{Raw: "10.0.0.13:8080"},
		},
		targetState: []TargetState{
			{Healthy: true},
			{Healthy: true},
			{Healthy: true},
		},
	}

	ok := pool.SetTargetUnhealthy(1, time.Now(), "connection refused")
	if !ok {
		t.Fatal("SetTargetUnhealthy() returned false")
	}

	first, ok := pool.NextTarget()
	if !ok {
		t.Fatal("NextTarget() returned no target on first call")
	}
	second, ok := pool.NextTarget()
	if !ok {
		t.Fatal("NextTarget() returned no target on second call")
	}
	third, ok := pool.NextTarget()
	if !ok {
		t.Fatal("NextTarget() returned no target on third call")
	}

	if got, want := first.Raw, "10.0.0.11:8080"; got != want {
		t.Fatalf("first target = %q, want %q", got, want)
	}
	if got, want := second.Raw, "10.0.0.13:8080"; got != want {
		t.Fatalf("second target = %q, want %q", got, want)
	}
	if got, want := third.Raw, "10.0.0.11:8080"; got != want {
		t.Fatalf("third target = %q, want %q", got, want)
	}
}

func TestPoolNextTarget_ReturnsFalseWhenNoHealthyTargets(t *testing.T) {
	pool := &Pool{
		Targets: []Target{
			{Raw: "10.0.0.11:8080"},
			{Raw: "10.0.0.12:8080"},
		},
		targetState: []TargetState{
			{Healthy: true},
			{Healthy: true},
		},
	}

	pool.SetTargetUnhealthy(0, time.Now(), "timeout")
	pool.SetTargetUnhealthy(1, time.Now(), "timeout")

	if _, ok := pool.NextTarget(); ok {
		t.Fatal("NextTarget() returned healthy target when none should exist")
	}
}
