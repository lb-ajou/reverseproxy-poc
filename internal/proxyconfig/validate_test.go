package proxyconfig

import "testing"

func TestConfigValidate_Success(t *testing.T) {
	cfg := Config{
		Routes: []RouteConfig{
			{
				ID:      "r-api",
				Enabled: true,
				Match: RouteMatchConfig{
					Hosts: []string{"api.example.com"},
					Path: &PathMatchConfig{
						Type:  PathMatchPrefix,
						Value: "/api/",
					},
				},
				UpstreamPool: "pool-api",
			},
		},
		UpstreamPools: map[string]UpstreamPool{
			"pool-api": {
				Upstreams: []string{"10.0.0.11:8080"},
			},
		},
	}

	if errs := cfg.Validate(); len(errs) > 0 {
		t.Fatalf("Validate() errors = %v", errs)
	}
}

func TestConfigValidate_UnknownUpstreamPool(t *testing.T) {
	cfg := Config{
		Routes: []RouteConfig{
			{
				ID:      "r-api",
				Enabled: true,
				Match: RouteMatchConfig{
					Hosts: []string{"api.example.com"},
				},
				UpstreamPool: "missing-pool",
			},
		},
	}

	errs := cfg.Validate()
	if len(errs) == 0 {
		t.Fatal("Validate() returned no errors")
	}
}

func TestConfigValidate_PrefixMustEndWithSlashUnlessRoot(t *testing.T) {
	cfg := Config{
		Routes: []RouteConfig{
			{
				ID:      "r-api",
				Enabled: true,
				Match: RouteMatchConfig{
					Hosts: []string{"api.example.com"},
					Path: &PathMatchConfig{
						Type:  PathMatchPrefix,
						Value: "/api",
					},
				},
				UpstreamPool: "pool-api",
			},
		},
		UpstreamPools: map[string]UpstreamPool{
			"pool-api": {
				Upstreams: []string{"10.0.0.11:8080"},
			},
		},
	}

	errs := cfg.Validate()
	if len(errs) == 0 {
		t.Fatal("Validate() returned no errors")
	}
}
