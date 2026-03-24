package route

import (
	"testing"

	"reverseproxy-poc/internal/proxyconfig"
)

func TestBuildTableAndResolve_AcrossMultipleProxyConfigs(t *testing.T) {
	configs := []proxyconfig.LoadedConfig{
		{
			Source: "default",
			Config: proxyconfig.Config{
				Routes: []proxyconfig.RouteConfig{
					{
						ID:      "api",
						Enabled: true,
						Match: proxyconfig.RouteMatchConfig{
							Hosts: []string{"api.example.com"},
							Path: &proxyconfig.PathMatchConfig{
								Type:  proxyconfig.PathMatchPrefix,
								Value: "/api/",
							},
						},
						UpstreamPool: "pool-api",
					},
				},
			},
		},
		{
			Source: "admin",
			Config: proxyconfig.Config{
				Routes: []proxyconfig.RouteConfig{
					{
						ID:      "api-admin",
						Enabled: true,
						Match: proxyconfig.RouteMatchConfig{
							Hosts: []string{"api.example.com"},
							Path: &proxyconfig.PathMatchConfig{
								Type:  proxyconfig.PathMatchPrefix,
								Value: "/api/admin/",
							},
						},
						UpstreamPool: "pool-admin",
					},
				},
			},
		},
	}

	routes, err := BuildTable(configs)
	if err != nil {
		t.Fatalf("BuildTable() error = %v", err)
	}

	matched, ok := Resolve(routes, "api.example.com", "/api/admin/users")
	if !ok {
		t.Fatal("Resolve() returned no route")
	}

	if got, want := matched.GlobalID, "admin:api-admin"; got != want {
		t.Fatalf("Resolve() GlobalID = %q, want %q", got, want)
	}
	if got, want := matched.UpstreamPool, "admin:pool-admin"; got != want {
		t.Fatalf("Resolve() UpstreamPool = %q, want %q", got, want)
	}
}
