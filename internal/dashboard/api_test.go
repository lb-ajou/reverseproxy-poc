package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/proxyconfig"
	"reverseproxy-poc/internal/route"
	"reverseproxy-poc/internal/runtime"
	"reverseproxy-poc/internal/upstream"
)

func TestConfigEndpoint_ReturnsStructuredSnapshotView(t *testing.T) {
	registry, err := upstream.NewRegistry([]upstream.Pool{
		{
			GlobalID: "default:pool-api",
			LocalID:  "pool-api",
			Source:   "default",
			Targets: []upstream.Target{
				{Raw: "10.0.0.11:8080"},
			},
		},
	})
	if err != nil {
		t.Fatalf("upstream.NewRegistry() error = %v", err)
	}

	snapshot := runtime.Snapshot{
		AppConfig: config.AppConfig{
			ProxyListenAddr:     ":8080",
			DashboardListenAddr: ":9090",
			ProxyConfigDir:      "configs/proxy",
		},
		ProxyConfigs: []proxyconfig.LoadedConfig{
			{
				Source: "default",
				Path:   "configs/proxy/default.json",
				Config: proxyconfig.Config{
					Name: "default",
					Routes: []proxyconfig.RouteConfig{
						{
							ID:      "r-api",
							Enabled: true,
							Match: proxyconfig.RouteMatchConfig{
								Hosts: []string{"api.example.com"},
							},
							UpstreamPool: "pool-api",
						},
					},
					UpstreamPools: map[string]proxyconfig.UpstreamPool{
						"pool-api": {
							Upstreams: []string{"10.0.0.11:8080"},
						},
					},
				},
			},
		},
		RouteTable: []route.Route{
			{
				GlobalID:     "default:r-api",
				LocalID:      "r-api",
				Source:       "default",
				Enabled:      true,
				Hosts:        []string{"api.example.com"},
				Path:         route.PathMatcher{Kind: route.PathKindAny},
				UpstreamPool: "default:pool-api",
			},
		},
		Upstreams: registry,
		AppliedAt: time.Unix(1700000000, 0).UTC(),
	}

	handler := NewHandler(runtime.NewState(snapshot))
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got, want := rec.Result().StatusCode, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}

	var body SnapshotView
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("json decode error = %v", err)
	}

	if got, want := body.AppConfig.ProxyConfigDir, "configs/proxy"; got != want {
		t.Fatalf("AppConfig.ProxyConfigDir = %q, want %q", got, want)
	}
	if got, want := len(body.ProxyConfigs), 1; got != want {
		t.Fatalf("len(ProxyConfigs) = %d, want %d", got, want)
	}
	if got, want := len(body.RouteTable), 1; got != want {
		t.Fatalf("len(RouteTable) = %d, want %d", got, want)
	}
	if got, want := len(body.Upstreams), 1; got != want {
		t.Fatalf("len(Upstreams) = %d, want %d", got, want)
	}
	if got, want := body.Upstreams[0].GlobalID, "default:pool-api"; got != want {
		t.Fatalf("Upstreams[0].GlobalID = %q, want %q", got, want)
	}
}

func TestRoutesEndpoint_RejectsNonGet(t *testing.T) {
	handler := NewHandler(runtime.NewState(runtime.Snapshot{}))
	req := httptest.NewRequest(http.MethodPost, "/api/routes", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got, want := rec.Result().StatusCode, http.StatusMethodNotAllowed; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestSPAPath_ReturnsDashboardHTML(t *testing.T) {
	handler := NewHandler(runtime.NewState(runtime.Snapshot{}))
	req := httptest.NewRequest(http.MethodGet, "/routes", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got, want := rec.Result().StatusCode, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got := rec.Result().Header.Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, "<!doctype html>") {
		t.Fatalf("body did not contain HTML document")
	}
}

func TestUnknownAPIPath_ReturnsNotFound(t *testing.T) {
	handler := NewHandler(runtime.NewState(runtime.Snapshot{}))
	req := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got, want := rec.Result().StatusCode, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}
