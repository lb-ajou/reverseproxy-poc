package runtime

import (
	"testing"

	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/proxyconfig"
	"reverseproxy-poc/internal/route"
)

func TestNewSnapshot_CopiesSlices(t *testing.T) {
	appCfg := config.AppConfig{
		ProxyListenAddr:     ":8080",
		DashboardListenAddr: ":9090",
		ProxyConfigDir:      "configs/proxy",
	}
	proxyCfgs := []proxyconfig.LoadedConfig{
		{Source: "default"},
	}
	routes := []route.Route{
		{GlobalID: "default:r-api"},
	}

	snapshot := NewSnapshot(appCfg, proxyCfgs, routes, nil)

	proxyCfgs[0].Source = "changed"
	routes[0].GlobalID = "changed"

	if got, want := snapshot.ProxyConfigs[0].Source, "default"; got != want {
		t.Fatalf("snapshot.ProxyConfigs[0].Source = %q, want %q", got, want)
	}
	if got, want := snapshot.RouteTable[0].GlobalID, "default:r-api"; got != want {
		t.Fatalf("snapshot.RouteTable[0].GlobalID = %q, want %q", got, want)
	}
}

func TestStateSwap_ReplacesSnapshot(t *testing.T) {
	initial := NewSnapshot(
		config.AppConfig{ProxyListenAddr: ":8080", DashboardListenAddr: ":9090", ProxyConfigDir: "configs/proxy"},
		nil,
		nil,
		nil,
	)

	state := NewState(initial)

	next := NewSnapshot(
		config.AppConfig{ProxyListenAddr: ":8081", DashboardListenAddr: ":9091", ProxyConfigDir: "configs/proxy"},
		nil,
		nil,
		nil,
	)

	state.Swap(next)

	if got, want := state.Snapshot().AppConfig.ProxyListenAddr, ":8081"; got != want {
		t.Fatalf("state.Snapshot().AppConfig.ProxyListenAddr = %q, want %q", got, want)
	}
}
