package admin

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/proxyconfig"
	"reverseproxy-poc/internal/route"
	appruntime "reverseproxy-poc/internal/runtime"
	"reverseproxy-poc/internal/upstream"
)

type testRuntime struct {
	configPath string
	snapshot   appruntime.Snapshot
}

func (r *testRuntime) Snapshot() appruntime.Snapshot {
	return r.snapshot
}

func (r *testRuntime) ReloadFromFile(context.Context) error {
	cfg, err := config.Load(r.configPath)
	if err != nil {
		return err
	}

	snapshot, err := buildSnapshot(cfg)
	if err != nil {
		return err
	}

	r.snapshot = snapshot
	return nil
}

func TestCreateUpstreamPoolAndRoute_WritesDefaultNamespaceAndReloads(t *testing.T) {
	service, testRuntime := newTestService(t)

	pool, err := service.CreateUpstreamPool(context.Background(), DefaultNamespace, "pool-api", proxyconfig.UpstreamPool{
		Upstreams: []string{"10.0.0.11:8080"},
	})
	if err != nil {
		t.Fatalf("CreateUpstreamPool() error = %v", err)
	}
	if got, want := len(pool.Upstreams), 1; got != want {
		t.Fatalf("len(pool.Upstreams) = %d, want %d", got, want)
	}

	routeCfg, err := service.CreateRoute(context.Background(), DefaultNamespace, proxyconfig.RouteConfig{
		ID:      "r-api",
		Enabled: true,
		Match: proxyconfig.RouteMatchConfig{
			Hosts: []string{"api.example.com"},
		},
		UpstreamPool: "pool-api",
	})
	if err != nil {
		t.Fatalf("CreateRoute() error = %v", err)
	}
	if got, want := routeCfg.ID, "r-api"; got != want {
		t.Fatalf("routeCfg.ID = %q, want %q", got, want)
	}

	loaded, err := proxyconfig.LoadFile(filepath.Join(testRuntime.snapshot.AppConfig.ProxyConfigDir, "default.json"))
	if err != nil {
		t.Fatalf("LoadFile(default.json) error = %v", err)
	}
	if got, want := len(loaded.Config.Routes), 1; got != want {
		t.Fatalf("len(loaded.Config.Routes) = %d, want %d", got, want)
	}
	if _, ok := loaded.Config.UpstreamPools["pool-api"]; !ok {
		t.Fatal("loaded.Config.UpstreamPools[pool-api] was not written")
	}

	snapshot := testRuntime.Snapshot()
	if got, want := len(snapshot.ProxyConfigs), 1; got != want {
		t.Fatalf("len(snapshot.ProxyConfigs) = %d, want %d", got, want)
	}
	if got, want := len(snapshot.RouteTable), 1; got != want {
		t.Fatalf("len(snapshot.RouteTable) = %d, want %d", got, want)
	}
	if _, ok := snapshot.Upstreams.Get("default:pool-api"); !ok {
		t.Fatal("snapshot.Upstreams.Get(default:pool-api) returned no pool")
	}
}

func TestDeleteUpstreamPool_RejectsReferencedPool(t *testing.T) {
	service, testRuntime := newTestService(t)
	writeTestJSON(t, filepath.Join(testRuntime.snapshot.AppConfig.ProxyConfigDir, "default.json"), `{
  "routes": [
    {
      "id": "r-api",
      "enabled": true,
      "match": {
        "hosts": ["api.example.com"]
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

	if err := testRuntime.ReloadFromFile(context.Background()); err != nil {
		t.Fatalf("ReloadFromFile() error = %v", err)
	}

	err := service.DeleteUpstreamPool(context.Background(), DefaultNamespace, "pool-api")
	if err == nil {
		t.Fatal("DeleteUpstreamPool() error = nil, want conflict")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("DeleteUpstreamPool() error type = %T, want *APIError", err)
	}
	if got, want := apiErr.StatusCode, http.StatusConflict; got != want {
		t.Fatalf("apiErr.StatusCode = %d, want %d", got, want)
	}
}

func TestListNamespaces_IncludesDefaultWhenMissing(t *testing.T) {
	service, _ := newTestService(t)

	items, err := service.ListNamespaces(context.Background())
	if err != nil {
		t.Fatalf("ListNamespaces() error = %v", err)
	}

	if got, want := len(items), 1; got != want {
		t.Fatalf("len(items) = %d, want %d", got, want)
	}
	if got, want := items[0].Namespace, DefaultNamespace; got != want {
		t.Fatalf("items[0].Namespace = %q, want %q", got, want)
	}
	if items[0].Exists {
		t.Fatal("items[0].Exists = true, want false")
	}
}

func newTestService(t *testing.T) (Service, *testRuntime) {
	t.Helper()

	dir := t.TempDir()
	proxyDir := filepath.Join(dir, "proxy")
	if err := os.MkdirAll(proxyDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(proxyDir) error = %v", err)
	}

	appConfigPath := filepath.Join(dir, "app.json")
	if err := os.WriteFile(appConfigPath, []byte(`{
  "proxyListenAddr": ":8080",
  "dashboardListenAddr": ":9090",
  "proxyConfigDir": "`+proxyDir+`"
}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile(app.json) error = %v", err)
	}

	cfg, err := config.Load(appConfigPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	snapshot, err := buildSnapshot(cfg)
	if err != nil {
		t.Fatalf("buildSnapshot() error = %v", err)
	}

	testRuntime := &testRuntime{
		configPath: appConfigPath,
		snapshot:   snapshot,
	}

	return New(testRuntime), testRuntime
}

func buildSnapshot(appCfg config.AppConfig) (appruntime.Snapshot, error) {
	proxyCfgs, err := proxyconfig.LoadDir(appCfg.ProxyConfigDir)
	if err != nil {
		return appruntime.Snapshot{}, err
	}

	routes, err := route.BuildTable(proxyCfgs)
	if err != nil {
		return appruntime.Snapshot{}, err
	}

	upstreams, err := upstream.BuildRegistry(proxyCfgs)
	if err != nil {
		return appruntime.Snapshot{}, err
	}

	return appruntime.NewSnapshot(appCfg, proxyCfgs, routes, upstreams), nil
}

func writeTestJSON(t *testing.T, path, body string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
}

func TestNamespaceConfigAppliedAtUsesRuntimeSnapshot(t *testing.T) {
	service, testRuntime := newTestService(t)
	testRuntime.snapshot.AppliedAt = time.Unix(1700000000, 0).UTC()

	view, err := service.GetNamespaceConfig(context.Background(), DefaultNamespace)
	if err != nil {
		t.Fatalf("GetNamespaceConfig() error = %v", err)
	}
	if got, want := view.AppliedAt, testRuntime.snapshot.AppliedAt; got != want {
		t.Fatalf("AppliedAt = %v, want %v", got, want)
	}
}
