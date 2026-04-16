package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"reverseproxy-poc/internal/admin"
	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/proxyconfig"
	"reverseproxy-poc/internal/runtime"
)

type stubService struct {
	listNamespacesFn        func() ([]admin.NamespaceView, error)
	createNamespaceFn       func(namespace string) (admin.NamespaceView, error)
	deleteNamespaceFn       func(namespace string) error
	getNamespaceConfigFn    func(namespace string) (admin.NamespaceConfigView, error)
	getNamespaceRoutesFn    func(namespace string) ([]proxyconfig.RouteConfig, error)
	createRouteFn           func(namespace string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error)
	updateRouteFn           func(namespace, id string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error)
	deleteRouteFn           func(namespace, id string) error
	getNamespaceUpstreamsFn func(namespace string) (map[string]proxyconfig.UpstreamPool, error)
	createUpstreamPoolFn    func(namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error)
	updateUpstreamPoolFn    func(namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error)
	deleteUpstreamPoolFn    func(namespace, id string) error
}

func (s stubService) ListNamespaces(_ context.Context) ([]admin.NamespaceView, error) {
	if s.listNamespacesFn != nil {
		return s.listNamespacesFn()
	}
	return nil, nil
}

func (s stubService) CreateNamespace(_ context.Context, namespace string) (admin.NamespaceView, error) {
	if s.createNamespaceFn != nil {
		return s.createNamespaceFn(namespace)
	}
	return admin.NamespaceView{}, nil
}

func (s stubService) DeleteNamespace(_ context.Context, namespace string) error {
	if s.deleteNamespaceFn != nil {
		return s.deleteNamespaceFn(namespace)
	}
	return nil
}

func (s stubService) GetNamespaceConfig(_ context.Context, namespace string) (admin.NamespaceConfigView, error) {
	if s.getNamespaceConfigFn != nil {
		return s.getNamespaceConfigFn(namespace)
	}
	return admin.NamespaceConfigView{}, nil
}

func (s stubService) GetNamespaceRoutes(_ context.Context, namespace string) ([]proxyconfig.RouteConfig, error) {
	if s.getNamespaceRoutesFn != nil {
		return s.getNamespaceRoutesFn(namespace)
	}
	return nil, nil
}

func (s stubService) CreateRoute(_ context.Context, namespace string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error) {
	if s.createRouteFn != nil {
		return s.createRouteFn(namespace, route)
	}
	return proxyconfig.RouteConfig{}, nil
}

func (s stubService) UpdateRoute(_ context.Context, namespace, id string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error) {
	if s.updateRouteFn != nil {
		return s.updateRouteFn(namespace, id, route)
	}
	return proxyconfig.RouteConfig{}, nil
}

func (s stubService) DeleteRoute(_ context.Context, namespace, id string) error {
	if s.deleteRouteFn != nil {
		return s.deleteRouteFn(namespace, id)
	}
	return nil
}

func (s stubService) GetNamespaceUpstreamPools(_ context.Context, namespace string) (map[string]proxyconfig.UpstreamPool, error) {
	if s.getNamespaceUpstreamsFn != nil {
		return s.getNamespaceUpstreamsFn(namespace)
	}
	return nil, nil
}

func (s stubService) CreateUpstreamPool(_ context.Context, namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error) {
	if s.createUpstreamPoolFn != nil {
		return s.createUpstreamPoolFn(namespace, id, pool)
	}
	return proxyconfig.UpstreamPool{}, nil
}

func (s stubService) UpdateUpstreamPool(_ context.Context, namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error) {
	if s.updateUpstreamPoolFn != nil {
		return s.updateUpstreamPoolFn(namespace, id, pool)
	}
	return proxyconfig.UpstreamPool{}, nil
}

func (s stubService) DeleteUpstreamPool(_ context.Context, namespace, id string) error {
	if s.deleteUpstreamPoolFn != nil {
		return s.deleteUpstreamPoolFn(namespace, id)
	}
	return nil
}

func TestNamespacedConfigEndpoint_ReturnsEditableConfig(t *testing.T) {
	handler := NewHandler(runtime.NewState(runtime.Snapshot{}), stubService{
		getNamespaceConfigFn: func(namespace string) (admin.NamespaceConfigView, error) {
			if namespace != admin.DefaultNamespace {
				t.Fatalf("namespace = %q, want %q", namespace, admin.DefaultNamespace)
			}
			return admin.NamespaceConfigView{
				Namespace: namespace,
				Exists:    true,
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
					"pool-api": {Upstreams: []string{"10.0.0.11:8080"}},
				},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/namespaces/default/config", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got, want := rec.Result().StatusCode, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}

	var body admin.NamespaceConfigView
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("json decode error = %v", err)
	}

	if got, want := body.Namespace, admin.DefaultNamespace; got != want {
		t.Fatalf("Namespace = %q, want %q", got, want)
	}
	if got, want := len(body.Routes), 1; got != want {
		t.Fatalf("len(Routes) = %d, want %d", got, want)
	}
	if got, want := len(body.UpstreamPools), 1; got != want {
		t.Fatalf("len(UpstreamPools) = %d, want %d", got, want)
	}
}

func TestCreateRouteEndpoint_CreatesRouteInNamespace(t *testing.T) {
	handler := NewHandler(runtime.NewState(runtime.Snapshot{}), stubService{
		createRouteFn: func(namespace string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error) {
			if namespace != admin.DefaultNamespace {
				t.Fatalf("namespace = %q, want %q", namespace, admin.DefaultNamespace)
			}
			if route.ID != "r-api" {
				t.Fatalf("route.ID = %q, want %q", route.ID, "r-api")
			}
			return route, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/namespaces/default/routes", strings.NewReader(`{
		"id":"r-api",
		"enabled":true,
		"match":{"hosts":["api.example.com"]},
		"upstream_pool":"pool-api"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got, want := rec.Result().StatusCode, http.StatusCreated; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}

	var body proxyconfig.RouteConfig
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("json decode error = %v", err)
	}
	if got, want := body.ID, "r-api"; got != want {
		t.Fatalf("ID = %q, want %q", got, want)
	}
}

func TestCreateNamespaceEndpoint_CreatesNamespace(t *testing.T) {
	handler := NewHandler(runtime.NewState(runtime.Snapshot{}), stubService{
		createNamespaceFn: func(namespace string) (admin.NamespaceView, error) {
			if namespace != "admin" {
				t.Fatalf("namespace = %q, want %q", namespace, "admin")
			}
			return admin.NamespaceView{
				Namespace: namespace,
				Path:      "configs/proxy/admin.json",
				Exists:    true,
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/namespaces", strings.NewReader(`{"namespace":"admin"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got, want := rec.Result().StatusCode, http.StatusCreated; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}

	var body admin.NamespaceView
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("json decode error = %v", err)
	}
	if got, want := body.Namespace, "admin"; got != want {
		t.Fatalf("Namespace = %q, want %q", got, want)
	}
}

func TestDeleteNamespaceEndpoint_DeletesNamespace(t *testing.T) {
	handler := NewHandler(runtime.NewState(runtime.Snapshot{}), stubService{
		deleteNamespaceFn: func(namespace string) error {
			if namespace != "admin" {
				t.Fatalf("namespace = %q, want %q", namespace, "admin")
			}
			return nil
		},
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/namespaces/admin", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got, want := rec.Result().StatusCode, http.StatusNoContent; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestRuntimeConfigEndpoint_ReturnsStructuredSnapshotView(t *testing.T) {
	snapshot := runtime.Snapshot{
		AppConfig: config.AppConfig{
			ProxyListenAddr:     ":8080",
			DashboardListenAddr: ":9090",
			ProxyConfigDir:      "configs/proxy",
		},
		AppliedAt: time.Unix(1700000000, 0).UTC(),
	}

	handler := NewHandler(runtime.NewState(snapshot), stubService{})
	req := httptest.NewRequest(http.MethodGet, "/api/runtime/config", nil)
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
}

func TestValidationError_ReturnsStructuredErrorBody(t *testing.T) {
	handler := NewHandler(runtime.NewState(runtime.Snapshot{}), stubService{
		createRouteFn: func(namespace string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error) {
			return proxyconfig.RouteConfig{}, &admin.APIError{
				StatusCode: http.StatusUnprocessableEntity,
				Message:    "validation failed",
				ValidationErrors: []proxyconfig.ValidationError{
					{Field: "routes[0].id", Message: "duplicate route id"},
				},
			}
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/namespaces/default/routes", strings.NewReader(`{
		"id":"r-api",
		"enabled":true,
		"match":{"hosts":["api.example.com"]},
		"upstream_pool":"pool-api"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got, want := rec.Result().StatusCode, http.StatusUnprocessableEntity; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}

	var body admin.APIError
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("json decode error = %v", err)
	}
	if got, want := body.Message, "validation failed"; got != want {
		t.Fatalf("Message = %q, want %q", got, want)
	}
	if got, want := len(body.ValidationErrors), 1; got != want {
		t.Fatalf("len(ValidationErrors) = %d, want %d", got, want)
	}
}

func TestLegacyConfigEndpoint_ReturnsNotFound(t *testing.T) {
	handler := NewHandler(runtime.NewState(runtime.Snapshot{}), stubService{})
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got, want := rec.Result().StatusCode, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestSPAPath_ReturnsDashboardHTML(t *testing.T) {
	handler := NewHandler(runtime.NewState(runtime.Snapshot{}), stubService{})
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
	handler := NewHandler(runtime.NewState(runtime.Snapshot{}), stubService{})
	req := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got, want := rec.Result().StatusCode, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}
