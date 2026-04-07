package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reverseproxy-poc/internal/config"
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
