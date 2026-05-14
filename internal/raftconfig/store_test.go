package raftconfig

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/hashicorp/raft"

	"reverseproxy-poc/internal/configstore"
	"reverseproxy-poc/internal/proxyconfig"
)

func TestStoreReturnsNotLeaderWhenNodeIsFollower(t *testing.T) {
	store := NewStore(&fakeRaft{leader: "127.0.0.1:7001", state: raft.Follower}, NewFSM())

	_, err := store.CreateNamespace(context.Background(), "admin")
	if err == nil {
		t.Fatal("CreateNamespace() error = nil, want not leader")
	}
	if !configstore.IsNotLeader(err) {
		t.Fatalf("CreateNamespace() error = %v, want not leader", err)
	}
}

func TestStoreAppliesCommandOnLeader(t *testing.T) {
	fsm := NewFSM()
	store := NewStore(&fakeRaft{state: raft.Leader, apply: fsm.Apply}, fsm)

	_, err := store.CreateUpstreamPool(context.Background(), "default", "pool-api", proxyconfig.UpstreamPool{
		Upstreams: []string{"10.0.0.11:8080"},
	})
	if err != nil {
		t.Fatalf("CreateUpstreamPool() error = %v", err)
	}
	state := fsm.DesiredState()
	if _, ok := state.Namespaces["default"].UpstreamPools["pool-api"]; !ok {
		t.Fatal("pool-api missing from FSM state")
	}
}

func TestStoreMapsApplyLeadershipErrorToNotLeader(t *testing.T) {
	store := NewStore(&fakeRaft{leader: "127.0.0.1:7001", state: raft.Leader, applyErr: raft.ErrNotLeader}, NewFSM())

	_, err := store.CreateNamespace(context.Background(), "admin")
	if err == nil {
		t.Fatal("CreateNamespace() error = nil, want not leader")
	}
	if !configstore.IsNotLeader(err) {
		t.Fatalf("CreateNamespace() error = %v, want not leader", err)
	}
}

func TestStoreReturnsContextErrorBeforeApply(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	node := &fakeRaft{state: raft.Leader}
	store := NewStore(node, NewFSM())

	_, err := store.CreateNamespace(ctx, "admin")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateNamespace() error = %v, want context canceled", err)
	}
	if node.applyCount != 0 {
		t.Fatalf("Apply() calls = %d, want 0", node.applyCount)
	}
}

func TestStoreRejectsInvalidNamespaceWithBadRequest(t *testing.T) {
	fsm := NewFSM()
	store := NewStore(&fakeRaft{state: raft.Leader, apply: fsm.Apply}, fsm)

	_, err := store.CreateNamespace(context.Background(), "bad/name")
	requireStoreError(t, err, http.StatusBadRequest, "invalid_namespace")
}

func TestStoreMapsApplyRejectionsToFileModeSemantics(t *testing.T) {
	t.Run("duplicate namespace", func(t *testing.T) {
		fsm := NewFSM()
		store := NewStore(&fakeRaft{state: raft.Leader, apply: fsm.Apply}, fsm)

		if _, err := store.CreateNamespace(context.Background(), "admin"); err != nil {
			t.Fatalf("CreateNamespace() setup error = %v", err)
		}

		_, err := store.CreateNamespace(context.Background(), "admin")
		requireStoreError(t, err, http.StatusConflict, "resource_conflict")
	})

	t.Run("missing route delete", func(t *testing.T) {
		fsm := NewFSM()
		store := NewStore(&fakeRaft{state: raft.Leader, apply: fsm.Apply}, fsm)

		if _, err := store.CreateNamespace(context.Background(), "admin"); err != nil {
			t.Fatalf("CreateNamespace() setup error = %v", err)
		}

		err := store.DeleteRoute(context.Background(), "admin", "missing")
		requireStoreError(t, err, http.StatusNotFound, "resource_not_found")
	})

	t.Run("route id mismatch", func(t *testing.T) {
		fsm := NewFSM()
		store := NewStore(&fakeRaft{state: raft.Leader, apply: fsm.Apply}, fsm)

		_, err := store.UpdateRoute(context.Background(), "admin", "r-api", proxyconfig.RouteConfig{ID: "other"})
		requireStoreError(t, err, http.StatusBadRequest, "invalid_request")
	})

	t.Run("validation failure", func(t *testing.T) {
		fsm := NewFSM()
		store := NewStore(&fakeRaft{state: raft.Leader, apply: fsm.Apply}, fsm)

		_, err := store.CreateRoute(context.Background(), "admin", proxyconfig.RouteConfig{
			ID:           "r-api",
			Enabled:      true,
			Match:        proxyconfig.RouteMatchConfig{Hosts: []string{"api.example.com"}},
			UpstreamPool: "missing",
		})
		requireStoreError(t, err, http.StatusUnprocessableEntity, "validation_failed")
	})
}

func TestStoreImportJSONConfigAppliesOnlyToEmptyState(t *testing.T) {
	fsm := NewFSM()
	store := NewStore(&fakeRaft{state: raft.Leader, apply: fsm.Apply}, fsm)
	seed := map[string]proxyconfig.Config{
		"admin": {
			UpstreamPools: map[string]proxyconfig.UpstreamPool{
				"pool-api": {Upstreams: []string{"10.0.0.11:8080"}},
			},
		},
	}

	if err := store.ImportJSONConfig(context.Background(), seed); err != nil {
		t.Fatalf("ImportJSONConfig() error = %v", err)
	}
	if _, ok := fsm.DesiredState().Namespaces["admin"]; !ok {
		t.Fatal("imported namespace admin missing")
	}

	err := store.ImportJSONConfig(context.Background(), seed)
	requireStoreError(t, err, http.StatusConflict, "resource_conflict")
}

func requireStoreError(t *testing.T, err error, statusCode int, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want status %d code %q", statusCode, code)
	}
	var storeErr *configstore.StoreError
	if !errors.As(err, &storeErr) {
		t.Fatalf("error = %T %v, want *configstore.StoreError", err, err)
	}
	if storeErr.StatusCode != statusCode || storeErr.Code != code {
		t.Fatalf("StoreError = status %d code %q, want status %d code %q", storeErr.StatusCode, storeErr.Code, statusCode, code)
	}
}

type fakeRaft struct {
	leader     string
	state      raft.RaftState
	apply      func(*raft.Log) interface{}
	applyErr   error
	applyCount int
}

func (r *fakeRaft) State() raft.RaftState      { return r.state }
func (r *fakeRaft) Leader() raft.ServerAddress { return raft.ServerAddress(r.leader) }
func (r *fakeRaft) Apply(data []byte, timeout time.Duration) raft.ApplyFuture {
	r.applyCount++
	if r.applyErr != nil {
		return &fakeApplyFuture{err: r.applyErr}
	}
	if r.apply == nil {
		return &fakeApplyFuture{}
	}
	return &fakeApplyFuture{response: r.apply(&raft.Log{Index: 1, Data: data})}
}

type fakeApplyFuture struct {
	err      error
	response interface{}
}

func (f *fakeApplyFuture) Error() error          { return f.err }
func (f *fakeApplyFuture) Response() interface{} { return f.response }
func (f *fakeApplyFuture) Index() uint64         { return 1 }
func (f *fakeApplyFuture) Start() time.Time      { return time.Now() }
