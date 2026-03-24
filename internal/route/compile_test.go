package route

import (
	"testing"

	"reverseproxy-poc/internal/proxyconfig"
)

func TestBuildTable_GlobalizesIDsAndSortsRoutes(t *testing.T) {
	configs := []proxyconfig.LoadedConfig{
		{
			Source: "default",
			Config: proxyconfig.Config{
				Routes: []proxyconfig.RouteConfig{
					{
						ID:      "catchall",
						Enabled: true,
						Match: proxyconfig.RouteMatchConfig{
							Hosts: []string{"api.example.com"},
						},
						UpstreamPool: "pool-default",
					},
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
						ID:      "login",
						Enabled: true,
						Match: proxyconfig.RouteMatchConfig{
							Hosts: []string{"api.example.com"},
							Path: &proxyconfig.PathMatchConfig{
								Type:  proxyconfig.PathMatchExact,
								Value: "/login",
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

	if got, want := routes[0].GlobalID, "admin:login"; got != want {
		t.Fatalf("routes[0].GlobalID = %q, want %q", got, want)
	}
	if got, want := routes[1].GlobalID, "default:api"; got != want {
		t.Fatalf("routes[1].GlobalID = %q, want %q", got, want)
	}
	if got, want := routes[2].GlobalID, "default:catchall"; got != want {
		t.Fatalf("routes[2].GlobalID = %q, want %q", got, want)
	}
	if got, want := routes[1].UpstreamPool, "default:pool-api"; got != want {
		t.Fatalf("routes[1].UpstreamPool = %q, want %q", got, want)
	}
}

func TestBuildRoutes_CompilesRegex(t *testing.T) {
	cfg := proxyconfig.Config{
		Routes: []proxyconfig.RouteConfig{
			{
				ID:      "user",
				Enabled: true,
				Match: proxyconfig.RouteMatchConfig{
					Hosts: []string{"api.example.com"},
					Path: &proxyconfig.PathMatchConfig{
						Type:  proxyconfig.PathMatchRegex,
						Value: "^/users/[0-9]+$",
					},
				},
				UpstreamPool: "pool-api",
			},
		},
	}

	routes, err := BuildRoutes("default", cfg)
	if err != nil {
		t.Fatalf("BuildRoutes() error = %v", err)
	}

	if routes[0].Path.Regex == nil {
		t.Fatal("BuildRoutes() did not compile regex")
	}
}
