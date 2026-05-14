package app

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reverseproxy-poc/internal/config"
	appruntime "reverseproxy-poc/internal/runtime"
	"reverseproxy-poc/internal/upstream"
)

func TestBuildSnapshot_LoadsProxyConfigsAndBuildsRuntimeState(t *testing.T) {
	dir := t.TempDir()

	writeTestJSON(t, filepath.Join(dir, "default.json"), `{
  "routes": [
    {
      "id": "r-api",
      "enabled": true,
      "match": {
        "hosts": ["api.example.com"],
        "path": { "type": "prefix", "value": "/api/" }
      },
      "upstream_pool": "pool-api"
    }
  ],
  "upstream_pools": {
    "pool-api": {
      "upstreams": ["10.0.0.11:8080"]
    }
  }
}`)

	writeTestJSON(t, filepath.Join(dir, "admin.json"), `{
  "routes": [
    {
      "id": "r-login",
      "enabled": true,
      "match": {
        "hosts": ["api.example.com"],
        "path": { "type": "exact", "value": "/login" }
      },
      "upstream_pool": "pool-admin"
    }
  ],
  "upstream_pools": {
    "pool-admin": {
      "upstreams": ["10.0.1.10:9000"]
    }
  }
}`)

	cfg := config.AppConfig{
		ProxyListenAddr:     ":8080",
		DashboardListenAddr: ":9090",
		ProxyConfigDir:      dir,
	}

	snapshot, err := buildSnapshot(cfg)
	if err != nil {
		t.Fatalf("buildSnapshot() error = %v", err)
	}

	if got, want := len(snapshot.ProxyConfigs), 2; got != want {
		t.Fatalf("len(snapshot.ProxyConfigs) = %d, want %d", got, want)
	}
	if got, want := len(snapshot.RouteTable), 2; got != want {
		t.Fatalf("len(snapshot.RouteTable) = %d, want %d", got, want)
	}
	if got, want := snapshot.RouteTable[0].GlobalID, "admin:r-login"; got != want {
		t.Fatalf("snapshot.RouteTable[0].GlobalID = %q, want %q", got, want)
	}
	if snapshot.Upstreams == nil {
		t.Fatal("snapshot.Upstreams is nil")
	}
	if _, ok := snapshot.Upstreams.Get("default:pool-api"); !ok {
		t.Fatal("snapshot.Upstreams.Get(default:pool-api) returned no pool")
	}
}

func writeTestJSON(t *testing.T, path, body string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
}

func TestAppStartAndStopHealthChecker(t *testing.T) {
	app := &App{
		healthChecker: upstream.NewChecker(nil),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app.startHealthChecker(ctx)

	if app.runCtx == nil {
		t.Fatal("runCtx is nil")
	}
	if app.healthCtx == nil {
		t.Fatal("healthCtx is nil")
	}
	if app.healthCancel == nil {
		t.Fatal("healthCancel is nil")
	}

	app.stopHealthChecker()

	if app.runCtx != nil {
		t.Fatal("runCtx is not nil after stop")
	}
	if app.healthCtx != nil {
		t.Fatal("healthCtx is not nil after stop")
	}
	if app.healthCancel != nil {
		t.Fatal("healthCancel is not nil after stop")
	}
}

func TestAppSwapHealthChecker_ReplacesCheckerAndStartsNewContext(t *testing.T) {
	registry, err := upstream.NewRegistry([]upstream.Pool{
		{GlobalID: "default:pool-api"},
	})
	if err != nil {
		t.Fatalf("upstream.NewRegistry() error = %v", err)
	}

	app := &App{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app.startHealthChecker(ctx)
	firstHealthCtx := app.healthCtx

	app.swapHealthChecker(registry)

	if app.healthChecker == nil {
		t.Fatal("healthChecker is nil")
	}
	if app.healthCtx == nil {
		t.Fatal("healthCtx is nil")
	}
	if app.healthCtx == firstHealthCtx {
		t.Fatal("healthCtx was not replaced")
	}
	if app.healthCancel == nil {
		t.Fatal("healthCancel is nil")
	}

	app.stopHealthChecker()
}

func TestAppHealthCheckerCancelCancelsContext(t *testing.T) {
	app := &App{
		healthChecker: upstream.NewChecker(nil),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app.startHealthChecker(ctx)
	healthCtx := app.healthCtx

	app.stopHealthChecker()

	select {
	case <-healthCtx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("health context was not canceled")
	}
}

func TestNew_WiresDashboardServerHandler(t *testing.T) {
	dir := t.TempDir()
	cfg := config.AppConfig{
		ProxyListenAddr:     ":8080",
		DashboardListenAddr: ":9090",
		ProxyConfigDir:      dir,
	}

	app, err := New(cfg, filepath.Join(dir, "app.json"), log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if app.dashboardHandler == nil {
		t.Fatal("dashboardHandler is nil")
	}
	if app.dashboardServer == nil {
		t.Fatal("dashboardServer is nil")
	}
	if app.dashboardServer.Handler == nil {
		t.Fatal("dashboardServer.Handler is nil")
	}
	if app.dashboardServer.Handler != app.dashboardHandler {
		t.Fatal("dashboardServer.Handler was not wired to dashboardHandler")
	}
}

func TestNew_RaftModeUsesRaftNodeConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := config.AppConfig{
		ProxyListenAddr:     ":8080",
		DashboardListenAddr: ":9090",
		ProxyConfigDir:      dir,
		ConfigStore:         "raft",
		RaftNodeID:          "node-1",
		RaftBindAddr:        "not-a-valid-address",
		RaftAdvertiseAddr:   "127.0.0.1:7001",
		RaftDataDir:         filepath.Join(dir, "raft"),
	}

	_, err := New(cfg, filepath.Join(dir, "app.json"), log.New(io.Discard, "", 0))
	if err == nil {
		t.Fatal("New() error = nil, want raft bind error")
	}
}

func TestShouldImportSeedRules(t *testing.T) {
	base := config.AppConfig{
		ConfigStore:     "raft",
		RaftBootstrap:   true,
		ProxyConfigDir:  "configs/proxy",
		RaftJSONSeedDir: "configs/seed",
	}

	tests := []struct {
		name             string
		cfg              config.AppConfig
		hasExistingState bool
		want             bool
	}{
		{name: "bootstrap brand-new node imports", cfg: base, want: true},
		{name: "existing raft state skips", cfg: base, hasExistingState: true, want: false},
		{name: "join skips", cfg: func() config.AppConfig {
			cfg := base
			cfg.RaftJoinAddr = "http://leader:9090"
			return cfg
		}(), want: false},
		{name: "non-bootstrap skips", cfg: func() config.AppConfig {
			cfg := base
			cfg.RaftBootstrap = false
			return cfg
		}(), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldImportSeed(tt.cfg, tt.hasExistingState); got != tt.want {
				t.Fatalf("shouldImportSeed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadSeedNamespacesLoadsProxyConfigDirectory(t *testing.T) {
	dir := t.TempDir()
	writeTestJSON(t, filepath.Join(dir, "admin.json"), `{
  "routes": [],
  "upstream_pools": {
    "pool-api": { "upstreams": ["10.0.0.11:8080"] }
  }
}`)

	namespaces, err := loadSeedNamespaces(dir)
	if err != nil {
		t.Fatalf("loadSeedNamespaces() error = %v", err)
	}
	if _, ok := namespaces["admin"]; !ok {
		t.Fatal("namespaces[admin] missing")
	}
}

func TestShouldRequestRaftJoinRules(t *testing.T) {
	cfg := config.AppConfig{RaftJoinAddr: "http://leader:9090"}
	if !shouldRequestRaftJoin(cfg, false) {
		t.Fatal("shouldRequestRaftJoin() = false, want true for brand-new joining node")
	}
	if shouldRequestRaftJoin(cfg, true) {
		t.Fatal("shouldRequestRaftJoin() = true, want false with existing raft state")
	}
	cfg.RaftJoinAddr = ""
	if shouldRequestRaftJoin(cfg, false) {
		t.Fatal("shouldRequestRaftJoin() = true, want false without join address")
	}
}

func TestPostRaftJoinPostsExpectedJSON(t *testing.T) {
	var gotPath string
	var gotBody raftJoinRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode join body error = %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := postRaftJoin(context.Background(), server.Client(), server.URL, "node-2", "127.0.0.1:7002"); err != nil {
		t.Fatalf("postRaftJoin() error = %v", err)
	}
	if gotPath != "/api/raft/join" {
		t.Fatalf("path = %q, want /api/raft/join", gotPath)
	}
	if gotBody.NodeID != "node-2" || gotBody.RaftAddress != "127.0.0.1:7002" {
		t.Fatalf("join body = %+v, want node-2/127.0.0.1:7002", gotBody)
	}
}

func TestNewRaftJoinHTTPClientHasTimeout(t *testing.T) {
	client := newRaftJoinHTTPClient()
	if client == nil {
		t.Fatal("newRaftJoinHTTPClient() returned nil")
	}
	if client.Timeout <= 0 {
		t.Fatalf("client.Timeout = %s, want positive timeout", client.Timeout)
	}
}

func TestRaftJoinURLAcceptsEndpointAddress(t *testing.T) {
	got, err := raftJoinURL("http://leader:9090/api/raft/join")
	if err != nil {
		t.Fatalf("raftJoinURL() error = %v", err)
	}
	if got != "http://leader:9090/api/raft/join" {
		t.Fatalf("raftJoinURL() = %q, want endpoint unchanged", got)
	}
}

func TestReloadFromFile_DisabledInRaftMode(t *testing.T) {
	app := &App{
		configPath: filepath.Join(t.TempDir(), "app.json"),
		state: appruntime.NewState(appruntime.Snapshot{
			AppConfig: config.AppConfig{ConfigStore: "raft"},
		}),
	}

	if err := app.ReloadFromFile(context.Background()); err == nil {
		t.Fatal("ReloadFromFile() error = nil, want disabled in raft mode")
	}
}
