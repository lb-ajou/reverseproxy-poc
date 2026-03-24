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

func NewState(cfg config.AppConfig) *State {
	return &State{
		snapshot: Snapshot{
			AppConfig: cfg,
			AppliedAt: time.Now(),
		},
	}
}

func (s *State) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.snapshot
}

func (s *State) Swap(cfg config.AppConfig) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.snapshot = Snapshot{
		AppConfig: cfg,
		AppliedAt: time.Now(),
	}

	return s.snapshot
}
