package app

import (
	"os"
	"path/filepath"
	"testing"

	"reverseproxy-poc/internal/config"
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
