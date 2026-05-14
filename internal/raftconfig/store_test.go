package raftconfig

import (
	"context"
	"errors"
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
