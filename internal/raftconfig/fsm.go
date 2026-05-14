package raftconfig

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/hashicorp/raft"

	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/configstore"
	"reverseproxy-poc/internal/proxyconfig"
)

type FSM struct {
	mu      sync.RWMutex
	state   configstore.DesiredState
	appCfg  config.AppConfig
	onApply func(configstore.DesiredState)
}

func NewFSM() *FSM {
	return NewFSMWithConfig(config.AppConfig{}, nil)
}

func NewFSMWithConfig(appCfg config.AppConfig, onApply func(configstore.DesiredState)) *FSM {
	return &FSM{
		state: configstore.DesiredState{
			Namespaces: map[string]proxyconfig.Config{},
			AppliedAt:  time.Now(),
		},
		appCfg:  appCfg,
		onApply: onApply,
	}
}

func (f *FSM) Apply(log *raft.Log) any {
	cmd, err := DecodeCommand(log.Data)
	if err != nil {
		return ApplyResponse{Error: err.Error()}
	}

	f.mu.RLock()
	next := cloneDesiredState(f.state)
	f.mu.RUnlock()

	if err := f.applyCommand(&next, cmd); err != nil {
		return ApplyResponse{Error: err.Error()}
	}
	if errs := validateDesiredState(next); len(errs) > 0 {
		return ApplyResponse{Error: proxyconfig.ValidationErrors(errs).Error()}
	}
	if _, err := configstore.ProjectSnapshot(f.appCfg, next); err != nil {
		return ApplyResponse{Error: err.Error()}
	}

	next.Version = log.Index
	next.AppliedAt = time.Now()

	f.mu.Lock()
	f.state = cloneDesiredState(next)
	f.mu.Unlock()

	if f.onApply != nil {
		f.onApply(cloneDesiredState(next))
	}

	return ApplyResponse{}
}

func (f *FSM) DesiredState() configstore.DesiredState {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return cloneDesiredState(f.state)
}

func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	return newFSMSnapshot(f.DesiredState()), nil
}

func (f *FSM) Restore(reader io.ReadCloser) error {
	defer func() {
		_ = reader.Close()
	}()

	state, err := decodeSnapshot(reader)
	if err != nil {
		return err
	}

	f.mu.Lock()
	f.state = cloneDesiredState(state)
	f.mu.Unlock()

	if f.onApply != nil {
		f.onApply(cloneDesiredState(state))
	}

	return nil
}

func (f *FSM) applyCommand(state *configstore.DesiredState, cmd Command) error {
	switch cmd.Type {
	case CommandCreateNamespace:
		if _, exists := state.Namespaces[cmd.Namespace]; exists {
			return fmt.Errorf("namespace %q already exists", cmd.Namespace)
		}
		state.Namespaces[cmd.Namespace] = ensureNamespace(state.Namespaces, cmd.Namespace)
	case CommandDeleteNamespace:
		if _, exists := state.Namespaces[cmd.Namespace]; !exists {
			return fmt.Errorf("namespace %q was not found", cmd.Namespace)
		}
		delete(state.Namespaces, cmd.Namespace)
	case CommandCreateUpstreamPool:
		cfg := ensureNamespace(state.Namespaces, cmd.Namespace)
		if _, exists := cfg.UpstreamPools[cmd.PoolID]; exists {
			return fmt.Errorf("upstream pool %q already exists", cmd.PoolID)
		}
		cfg.UpstreamPools[cmd.PoolID] = cloneUpstreamPool(cmd.Pool)
		state.Namespaces[cmd.Namespace] = cfg
	case CommandUpdateUpstreamPool:
		cfg, exists := state.Namespaces[cmd.Namespace]
		if !exists {
			return fmt.Errorf("namespace %q was not found", cmd.Namespace)
		}
		cfg = cloneConfig(cfg)
		if _, exists := cfg.UpstreamPools[cmd.PoolID]; !exists {
			return fmt.Errorf("upstream pool %q was not found", cmd.PoolID)
		}
		cfg.UpstreamPools[cmd.PoolID] = cloneUpstreamPool(cmd.Pool)
		state.Namespaces[cmd.Namespace] = cfg
	case CommandDeleteUpstreamPool:
		cfg, exists := state.Namespaces[cmd.Namespace]
		if !exists {
			return fmt.Errorf("namespace %q was not found", cmd.Namespace)
		}
		cfg = cloneConfig(cfg)
		if _, exists := cfg.UpstreamPools[cmd.PoolID]; !exists {
			return fmt.Errorf("upstream pool %q was not found", cmd.PoolID)
		}
		for _, route := range cfg.Routes {
			if route.UpstreamPool == cmd.PoolID {
				return fmt.Errorf("upstream pool %q is still referenced by route %q", cmd.PoolID, route.ID)
			}
		}
		delete(cfg.UpstreamPools, cmd.PoolID)
		state.Namespaces[cmd.Namespace] = cfg
	case CommandCreateRoute:
		cfg := ensureNamespace(state.Namespaces, cmd.Namespace)
		for _, route := range cfg.Routes {
			if route.ID == cmd.Route.ID {
				return fmt.Errorf("route %q already exists", cmd.Route.ID)
			}
		}
		cfg.Routes = append(cfg.Routes, cloneRoute(cmd.Route))
		state.Namespaces[cmd.Namespace] = cfg
	case CommandUpdateRoute:
		if cmd.Route.ID != cmd.RouteID {
			return fmt.Errorf("route id in command must match route id in body")
		}
		cfg, exists := state.Namespaces[cmd.Namespace]
		if !exists {
			return fmt.Errorf("namespace %q was not found", cmd.Namespace)
		}
		cfg = cloneConfig(cfg)
		for index, route := range cfg.Routes {
			if route.ID == cmd.RouteID {
				cfg.Routes[index] = cloneRoute(cmd.Route)
				state.Namespaces[cmd.Namespace] = cfg
				return nil
			}
		}
		return fmt.Errorf("route %q was not found", cmd.RouteID)
	case CommandDeleteRoute:
		cfg, exists := state.Namespaces[cmd.Namespace]
		if !exists {
			return fmt.Errorf("namespace %q was not found", cmd.Namespace)
		}
		cfg = cloneConfig(cfg)
		for index, route := range cfg.Routes {
			if route.ID == cmd.RouteID {
				cfg.Routes = append(cfg.Routes[:index], cfg.Routes[index+1:]...)
				state.Namespaces[cmd.Namespace] = cfg
				return nil
			}
		}
		return fmt.Errorf("route %q was not found", cmd.RouteID)
	case CommandImportJSONConfig:
		if len(state.Namespaces) != 0 {
			return fmt.Errorf("import requires empty state")
		}
		state.Namespaces = make(map[string]proxyconfig.Config, len(cmd.Import))
		for namespace, cfg := range cmd.Import {
			state.Namespaces[namespace] = cloneConfig(cfg)
		}
	default:
		return fmt.Errorf("unknown command type %q", cmd.Type)
	}

	return nil
}

func ensureNamespace(namespaces map[string]proxyconfig.Config, namespace string) proxyconfig.Config {
	cfg, exists := namespaces[namespace]
	if !exists {
		cfg = proxyconfig.Config{}
	}
	cfg = cloneConfig(cfg)
	if cfg.Routes == nil {
		cfg.Routes = []proxyconfig.RouteConfig{}
	}
	if cfg.UpstreamPools == nil {
		cfg.UpstreamPools = map[string]proxyconfig.UpstreamPool{}
	}
	namespaces[namespace] = cfg
	return cfg
}

func validateDesiredState(state configstore.DesiredState) []proxyconfig.ValidationError {
	var errs []proxyconfig.ValidationError
	for namespace, cfg := range state.Namespaces {
		for _, err := range cfg.Validate() {
			err.Field = "namespaces." + namespace + "." + err.Field
			errs = append(errs, err)
		}
	}
	return errs
}
