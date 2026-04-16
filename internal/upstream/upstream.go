package upstream

import (
	"sync"
	"time"
)

type Pool struct {
	GlobalID    string
	LocalID     string
	Source      string
	Targets     []Target
	HealthCheck *HealthCheck
	mu          sync.RWMutex
	targetState []TargetState
	next        uint64
}

type Target struct {
	Raw string
}

type TargetState struct {
	Healthy       bool
	LastCheckedAt time.Time
	LastError     string
}

type HealthCheck struct {
	Path         string
	Interval     string
	Timeout      string
	ExpectStatus int
}

func (p *Pool) SnapshotStates() []TargetState {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return append([]TargetState(nil), p.targetState...)
}

func (p *Pool) SetTargetHealthy(index int, checkedAt time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if index < 0 || index >= len(p.targetState) {
		return false
	}

	p.targetState[index] = TargetState{
		Healthy:       true,
		LastCheckedAt: checkedAt,
	}

	return true
}

func (p *Pool) SetTargetUnhealthy(index int, checkedAt time.Time, lastErr string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if index < 0 || index >= len(p.targetState) {
		return false
	}

	p.targetState[index] = TargetState{
		Healthy:       false,
		LastCheckedAt: checkedAt,
		LastError:     lastErr,
	}

	return true
}
