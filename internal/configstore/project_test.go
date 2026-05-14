package configstore

import (
	"testing"

	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/proxyconfig"
)

func TestProjectSnapshot_BuildsRuntimeFromNamespaceMap(t *testing.T) {
	appCfg := config.AppConfig{
		ProxyListenAddr:     ":8080",
		DashboardListenAddr: ":9090",
		ProxyConfigDir:      "configs/proxy",
	}
	state := DesiredState{
		Namespaces: map[string]proxyconfig.Config{
			"default": {
				Routes: []proxyconfig.RouteConfig{{
					ID:      "r-api",
					Enabled: true,
					Match: proxyconfig.RouteMatchConfig{
						Hosts: []string{"api.example.com"},
					},
					UpstreamPool: "pool-api",
				}},
				UpstreamPools: map[string]proxyconfig.UpstreamPool{
					"pool-api": {Upstreams: []string{"10.0.0.11:8080"}},
				},
			},
		},
	}

	snapshot, err := ProjectSnapshot(appCfg, state)
	if err != nil {
		t.Fatalf("ProjectSnapshot() error = %v", err)
	}
	if got, want := len(snapshot.ProxyConfigs), 1; got != want {
		t.Fatalf("len(snapshot.ProxyConfigs) = %d, want %d", got, want)
	}
	if got, want := snapshot.ProxyConfigs[0].Source, "default"; got != want {
		t.Fatalf("snapshot.ProxyConfigs[0].Source = %q, want %q", got, want)
	}
	if got, want := len(snapshot.RouteTable), 1; got != want {
		t.Fatalf("len(snapshot.RouteTable) = %d, want %d", got, want)
	}
	if got, want := snapshot.RouteTable[0].GlobalID, "default:r-api"; got != want {
		t.Fatalf("snapshot.RouteTable[0].GlobalID = %q, want %q", got, want)
	}
	if _, ok := snapshot.Upstreams.Get("default:pool-api"); !ok {
		t.Fatal("snapshot.Upstreams.Get(default:pool-api) returned no pool")
	}
}

func TestProjectSnapshot_RejectsInvalidDesiredConfig(t *testing.T) {
	_, err := ProjectSnapshot(config.AppConfig{}, DesiredState{
		Namespaces: map[string]proxyconfig.Config{
			"default": {
				Routes: []proxyconfig.RouteConfig{{
					ID:           "r-api",
					Enabled:      true,
					Match:        proxyconfig.RouteMatchConfig{Hosts: []string{"api.example.com"}},
					UpstreamPool: "missing",
				}},
				UpstreamPools: map[string]proxyconfig.UpstreamPool{},
			},
		},
	})
	if err == nil {
		t.Fatal("ProjectSnapshot() error = nil, want validation error")
	}
}
