package upstream

import (
	"sync"
	"sync/atomic"
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
	active      []uint64
	next        uint64
}

func (p *Pool) ActiveConnections(index int) uint64 {
	if index < 0 || index >= len(p.active) {
		return 0
	}
	return atomic.LoadUint64(&p.active[index])
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
	return p.setTargetState(index, checkedAt, lastErr, false)
}

func (p *Pool) setTargetState(index int, checkedAt time.Time, lastErr string, healthy bool) bool {
	if index < 0 || index >= len(p.targetState) {
		return false
	}
	p.targetState[index] = TargetState{
		Healthy:       healthy,
		LastCheckedAt: checkedAt,
		LastError:     lastErr,
	}
	return true
}
