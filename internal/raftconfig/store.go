package raftconfig

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/hashicorp/raft"

	"reverseproxy-poc/internal/configstore"
	"reverseproxy-poc/internal/proxyconfig"
)

type raftApplier interface {
	State() raft.RaftState
	Leader() raft.ServerAddress
	Apply(cmd []byte, timeout time.Duration) raft.ApplyFuture
}

type Store struct {
	raft    raftApplier
	fsm     *FSM
	timeout time.Duration
}

func NewStore(node raftApplier, fsm *FSM) *Store {
	return &Store{raft: node, fsm: fsm, timeout: 5 * time.Second}
}

func (s *Store) DesiredState(_ context.Context) (configstore.DesiredState, error) {
	return s.fsm.DesiredState(), nil
}

func (s *Store) ListNamespaces(_ context.Context) ([]configstore.NamespaceSummary, error) {
	state := s.fsm.DesiredState()
	namespaces := make([]string, 0, len(state.Namespaces)+1)
	hasDefault := false
	for namespace := range state.Namespaces {
		if namespace == configstore.DefaultNamespace {
			hasDefault = true
		}
		namespaces = append(namespaces, namespace)
	}
	if !hasDefault {
		namespaces = append(namespaces, configstore.DefaultNamespace)
	}
	sort.Strings(namespaces)

	items := make([]configstore.NamespaceSummary, 0, len(namespaces))
	for _, namespace := range namespaces {
		cfg, exists := state.Namespaces[namespace]
		cfg = cloneConfig(cfg)
		items = append(items, configstore.NamespaceSummary{
			Namespace:         namespace,
			Exists:            exists,
			RouteCount:        len(cfg.Routes),
			UpstreamPoolCount: len(cfg.UpstreamPools),
		})
	}
	return items, nil
}

func (s *Store) GetNamespaceConfig(_ context.Context, namespace string) (configstore.NamespaceConfig, error) {
	state := s.fsm.DesiredState()
	cfg, exists := state.Namespaces[namespace]
	cfg = cloneConfig(cfg)
	return configstore.NamespaceConfig{
		Namespace:     namespace,
		Exists:        exists,
		Routes:        cfg.Routes,
		UpstreamPools: cfg.UpstreamPools,
		AppliedAt:     state.AppliedAt,
	}, nil
}

func (s *Store) CreateNamespace(ctx context.Context, namespace string) (configstore.NamespaceSummary, error) {
	if err := s.apply(ctx, Command{Type: CommandCreateNamespace, Namespace: namespace}); err != nil {
		return configstore.NamespaceSummary{}, err
	}
	return configstore.NamespaceSummary{
		Namespace: namespace,
		Exists:    true,
	}, nil
}

func (s *Store) DeleteNamespace(ctx context.Context, namespace string) error {
	return s.apply(ctx, Command{Type: CommandDeleteNamespace, Namespace: namespace})
}

func (s *Store) CreateRoute(ctx context.Context, namespace string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error) {
	if err := s.apply(ctx, Command{Type: CommandCreateRoute, Namespace: namespace, Route: route}); err != nil {
		return proxyconfig.RouteConfig{}, err
	}
	return cloneRoute(route), nil
}

func (s *Store) UpdateRoute(ctx context.Context, namespace, id string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error) {
	if err := s.apply(ctx, Command{Type: CommandUpdateRoute, Namespace: namespace, RouteID: id, Route: route}); err != nil {
		return proxyconfig.RouteConfig{}, err
	}
	return cloneRoute(route), nil
}

func (s *Store) DeleteRoute(ctx context.Context, namespace, id string) error {
	return s.apply(ctx, Command{Type: CommandDeleteRoute, Namespace: namespace, RouteID: id})
}

func (s *Store) CreateUpstreamPool(ctx context.Context, namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error) {
	if err := s.apply(ctx, Command{Type: CommandCreateUpstreamPool, Namespace: namespace, PoolID: id, Pool: pool}); err != nil {
		return proxyconfig.UpstreamPool{}, err
	}
	return cloneUpstreamPool(pool), nil
}

func (s *Store) UpdateUpstreamPool(ctx context.Context, namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error) {
	if err := s.apply(ctx, Command{Type: CommandUpdateUpstreamPool, Namespace: namespace, PoolID: id, Pool: pool}); err != nil {
		return proxyconfig.UpstreamPool{}, err
	}
	return cloneUpstreamPool(pool), nil
}

func (s *Store) DeleteUpstreamPool(ctx context.Context, namespace, id string) error {
	return s.apply(ctx, Command{Type: CommandDeleteUpstreamPool, Namespace: namespace, PoolID: id})
}

func (s *Store) ImportJSONConfig(ctx context.Context, namespaces map[string]proxyconfig.Config) error {
	return s.apply(ctx, Command{Type: CommandImportJSONConfig, Import: namespaces})
}

func (s *Store) apply(ctx context.Context, cmd Command) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.raft.State() != raft.Leader {
		return configstore.NewNotLeaderError(string(s.raft.Leader()))
	}

	data, err := EncodeCommand(cmd)
	if err != nil {
		return err
	}
	future := s.raft.Apply(data, s.timeout)
	if err := future.Error(); err != nil {
		if isRaftLeadershipError(err) {
			return configstore.NewNotLeaderError(string(s.raft.Leader()))
		}
		return err
	}
	if resp, ok := future.Response().(ApplyResponse); ok && resp.Error != "" {
		statusCode := resp.StatusCode
		if statusCode == 0 {
			statusCode = http.StatusUnprocessableEntity
		}
		code := resp.Code
		if code == "" {
			code = "raft_apply_rejected"
		}
		return &configstore.StoreError{
			StatusCode: statusCode,
			Code:       code,
			Message:    resp.Error,
		}
	}
	return nil
}

func isRaftLeadershipError(err error) bool {
	return errors.Is(err, raft.ErrNotLeader) ||
		errors.Is(err, raft.ErrLeadershipLost) ||
		errors.Is(err, raft.ErrLeadershipTransferInProgress)
}
