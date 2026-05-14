package raftconfig

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/hashicorp/raft"

	"reverseproxy-poc/internal/configstore"
	"reverseproxy-poc/internal/proxyconfig"
)

func TestFSMApplyCreatePoolAndRoute(t *testing.T) {
	fsm := NewFSM()
	applyCommand(t, fsm, Command{
		Type:      CommandCreateUpstreamPool,
		Namespace: configstore.DefaultNamespace,
		PoolID:    "pool-api",
		Pool:      proxyconfig.UpstreamPool{Upstreams: []string{"10.0.0.11:8080"}},
	})
	applyCommand(t, fsm, Command{
		Type:      CommandCreateRoute,
		Namespace: configstore.DefaultNamespace,
		Route: proxyconfig.RouteConfig{
			ID:           "r-api",
			Enabled:      true,
			Match:        proxyconfig.RouteMatchConfig{Hosts: []string{"api.example.com"}},
			UpstreamPool: "pool-api",
		},
	})

	state := fsm.DesiredState()
	cfg := state.Namespaces[configstore.DefaultNamespace]
	if got, want := len(cfg.Routes), 1; got != want {
		t.Fatalf("len(cfg.Routes) = %d, want %d", got, want)
	}
	if _, ok := cfg.UpstreamPools["pool-api"]; !ok {
		t.Fatal("cfg.UpstreamPools[pool-api] missing")
	}
}

func TestFSMApplyInvalidCommandLeavesStateUnchanged(t *testing.T) {
	fsm := NewFSM()
	resp := applyCommand(t, fsm, Command{
		Type:      CommandCreateRoute,
		Namespace: configstore.DefaultNamespace,
		Route: proxyconfig.RouteConfig{
			ID:           "r-api",
			Enabled:      true,
			Match:        proxyconfig.RouteMatchConfig{Hosts: []string{"api.example.com"}},
			UpstreamPool: "missing",
		},
	})
	if resp.Error == "" {
		t.Fatal("response error is empty, want validation error")
	}
	if got := len(fsm.DesiredState().Namespaces); got != 0 {
		t.Fatalf("len(fsm.DesiredState().Namespaces) = %d, want 0", got)
	}
}

func TestFSMApplyRejectsInvalidNamespace(t *testing.T) {
	fsm := NewFSM()
	resp := applyCommand(t, fsm, Command{
		Type:      CommandCreateNamespace,
		Namespace: "bad/name",
	})

	requireApplyRejection(t, resp, http.StatusBadRequest, "invalid_namespace")
	if got := len(fsm.DesiredState().Namespaces); got != 0 {
		t.Fatalf("len(fsm.DesiredState().Namespaces) = %d, want 0", got)
	}
}

func TestFSMImportJSONConfigRejectsInvalidNamespace(t *testing.T) {
	fsm := NewFSM()
	resp := applyCommand(t, fsm, Command{
		Type: CommandImportJSONConfig,
		Import: map[string]proxyconfig.Config{
			"bad/name": {},
		},
	})

	requireApplyRejection(t, resp, http.StatusBadRequest, "invalid_namespace")
}

func TestFSMSnapshotRestoreRoundTrip(t *testing.T) {
	fsm := NewFSM()
	applyCommand(t, fsm, Command{
		Type:      CommandCreateNamespace,
		Namespace: "admin",
	})
	snapshot, err := fsm.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	var buf bytes.Buffer
	if err := snapshot.Persist(&memorySink{Buffer: &buf}); err != nil {
		t.Fatalf("Persist() error = %v", err)
	}

	restored := NewFSM()
	if err := restored.Restore(io.NopCloser(bytes.NewReader(buf.Bytes()))); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if _, ok := restored.DesiredState().Namespaces["admin"]; !ok {
		t.Fatal("restored namespace admin missing")
	}
}

func requireApplyRejection(t *testing.T, resp ApplyResponse, statusCode int, code string) {
	t.Helper()
	if resp.Error == "" {
		t.Fatal("ApplyResponse.Error is empty, want rejection")
	}
	if resp.StatusCode != statusCode || resp.Code != code {
		t.Fatalf("ApplyResponse = status %d code %q, want status %d code %q", resp.StatusCode, resp.Code, statusCode, code)
	}
}

func applyCommand(t *testing.T, fsm *FSM, cmd Command) ApplyResponse {
	t.Helper()
	data, err := EncodeCommand(cmd)
	if err != nil {
		t.Fatalf("EncodeCommand() error = %v", err)
	}
	resp, ok := fsm.Apply(&raft.Log{Data: data}).(ApplyResponse)
	if !ok {
		t.Fatalf("Apply() response type = %T, want ApplyResponse", resp)
	}
	return resp
}

type memorySink struct {
	*bytes.Buffer
}

func (s *memorySink) ID() string    { return "memory" }
func (s *memorySink) Close() error  { return nil }
func (s *memorySink) Cancel() error { return nil }
