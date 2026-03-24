package upstream

import (
	"testing"

	"reverseproxy-poc/internal/proxyconfig"
)

func TestBuildRegistry_GlobalizesPoolIDs(t *testing.T) {
	configs := []proxyconfig.LoadedConfig{
		{
			Source: "default",
			Config: proxyconfig.Config{
				UpstreamPools: map[string]proxyconfig.UpstreamPool{
					"pool-api": {
						Upstreams: []string{"10.0.0.11:8080"},
					},
				},
			},
		},
		{
			Source: "admin",
			Config: proxyconfig.Config{
				UpstreamPools: map[string]proxyconfig.UpstreamPool{
					"pool-api": {
						Upstreams: []string{"10.0.1.10:9000"},
					},
				},
			},
		},
	}

	registry, err := BuildRegistry(configs)
	if err != nil {
		t.Fatalf("BuildRegistry() error = %v", err)
	}

	if _, ok := registry.Get("default:pool-api"); !ok {
		t.Fatal("registry.Get(default:pool-api) returned no pool")
	}
	if _, ok := registry.Get("admin:pool-api"); !ok {
		t.Fatal("registry.Get(admin:pool-api) returned no pool")
	}
}

func TestBuildPools_CopiesHealthCheck(t *testing.T) {
	cfg := proxyconfig.Config{
		UpstreamPools: map[string]proxyconfig.UpstreamPool{
			"pool-api": {
				Upstreams: []string{"10.0.0.11:8080"},
				HealthCheck: &proxyconfig.HealthCheckConfig{
					Path:         "/health",
					Interval:     "30s",
					Timeout:      "3s",
					ExpectStatus: 200,
				},
			},
		},
	}

	pools, err := BuildPools("default", cfg)
	if err != nil {
		t.Fatalf("BuildPools() error = %v", err)
	}

	if pools[0].HealthCheck == nil {
		t.Fatal("BuildPools() did not copy health check")
	}
	if got, want := pools[0].HealthCheck.Path, "/health"; got != want {
		t.Fatalf("HealthCheck.Path = %q, want %q", got, want)
	}
}
