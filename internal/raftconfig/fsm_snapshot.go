package raftconfig

import (
	"encoding/json"
	"io"

	"github.com/hashicorp/raft"

	"reverseproxy-poc/internal/configstore"
	"reverseproxy-poc/internal/proxyconfig"
)

type fsmSnapshot struct {
	state configstore.DesiredState
}

func newFSMSnapshot(state configstore.DesiredState) raft.FSMSnapshot {
	return &fsmSnapshot{state: cloneDesiredState(state)}
}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	if err := json.NewEncoder(sink).Encode(s.state); err != nil {
		_ = sink.Cancel()
		return err
	}
	return sink.Close()
}

func (s *fsmSnapshot) Release() {}

func decodeSnapshot(reader io.Reader) (configstore.DesiredState, error) {
	var state configstore.DesiredState
	if err := json.NewDecoder(reader).Decode(&state); err != nil {
		return configstore.DesiredState{}, err
	}
	if state.Namespaces == nil {
		state.Namespaces = map[string]proxyconfig.Config{}
	}
	return cloneDesiredState(state), nil
}

func cloneDesiredState(state configstore.DesiredState) configstore.DesiredState {
	cloned := state
	cloned.Namespaces = make(map[string]proxyconfig.Config, len(state.Namespaces))
	for namespace, cfg := range state.Namespaces {
		cloned.Namespaces[namespace] = cloneConfig(cfg)
	}
	return cloned
}

func cloneConfig(cfg proxyconfig.Config) proxyconfig.Config {
	cloned := cfg
	if cfg.Routes == nil {
		cloned.Routes = []proxyconfig.RouteConfig{}
	} else {
		cloned.Routes = make([]proxyconfig.RouteConfig, 0, len(cfg.Routes))
		for _, route := range cfg.Routes {
			cloned.Routes = append(cloned.Routes, cloneRoute(route))
		}
	}

	cloned.UpstreamPools = make(map[string]proxyconfig.UpstreamPool, len(cfg.UpstreamPools))
	for id, pool := range cfg.UpstreamPools {
		cloned.UpstreamPools[id] = cloneUpstreamPool(pool)
	}
	return cloned
}

func cloneRoute(route proxyconfig.RouteConfig) proxyconfig.RouteConfig {
	cloned := route
	cloned.Match.Hosts = append([]string(nil), route.Match.Hosts...)
	if route.Match.Path != nil {
		path := *route.Match.Path
		cloned.Match.Path = &path
	}
	return cloned
}

func cloneUpstreamPool(pool proxyconfig.UpstreamPool) proxyconfig.UpstreamPool {
	cloned := pool
	cloned.Upstreams = append([]string(nil), pool.Upstreams...)
	if pool.HealthCheck != nil {
		healthCheck := *pool.HealthCheck
		cloned.HealthCheck = &healthCheck
	}
	return cloned
}
