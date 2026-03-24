package runtime

import (
	"sync"
	"time"

	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/proxyconfig"
	"reverseproxy-poc/internal/route"
	"reverseproxy-poc/internal/upstream"
)

type State struct {
	mu       sync.RWMutex
	snapshot Snapshot
}

func NewState(snapshot Snapshot) *State {
	return &State{
		snapshot: snapshot,
	}
}

func NewSnapshot(
	appCfg config.AppConfig,
	proxyCfgs []proxyconfig.LoadedConfig,
	routes []route.Route,
	upstreams *upstream.Registry,
) Snapshot {
	proxyCfgsCopy := append([]proxyconfig.LoadedConfig(nil), proxyCfgs...)
	routesCopy := append([]route.Route(nil), routes...)

	return Snapshot{
		AppConfig:    appCfg,
		ProxyConfigs: proxyCfgsCopy,
		RouteTable:   routesCopy,
		Upstreams:    upstreams,
		AppliedAt:    time.Now(),
	}
}

func (s *State) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.snapshot
}

func (s *State) Swap(snapshot Snapshot) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.snapshot = snapshot

	return s.snapshot
}
