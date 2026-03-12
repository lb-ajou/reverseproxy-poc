package runtime

import (
	"sync"
	"time"

	"reverseproxy-poc/internal/config"
)

type State struct {
	mu       sync.RWMutex
	snapshot Snapshot
}

func NewState(cfg config.Config) *State {
	return &State{
		snapshot: Snapshot{
			Config:    cfg,
			AppliedAt: time.Now(),
		},
	}
}

func (s *State) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.snapshot
}

func (s *State) Swap(cfg config.Config) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.snapshot = Snapshot{
		Config:    cfg,
		AppliedAt: time.Now(),
	}

	return s.snapshot
}
