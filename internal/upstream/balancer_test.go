package upstream

import "testing"

func TestPoolNextTarget_RoundRobin(t *testing.T) {
	pool := &Pool{
		Targets: []Target{
			{Raw: "10.0.0.11:8080"},
			{Raw: "10.0.0.12:8080"},
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
