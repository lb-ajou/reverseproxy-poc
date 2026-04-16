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
	"reverseproxy-poc/internal/route"
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

func performDashboardRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, target interface{}) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(target); err != nil {
		t.Fatalf("json decode error = %v", err)
	}
}

func requireStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if got := rec.Result().StatusCode; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func defaultNamespaceConfig() admin.NamespaceConfigView {
	return admin.NamespaceConfigView{
		Namespace: admin.DefaultNamespace,
		Exists:    true,
		Routes: []proxyconfig.RouteConfig{{
			ID: "r-api", Enabled: true,
			Match:        proxyconfig.RouteMatchConfig{Hosts: []string{"api.example.com"}},
			UpstreamPool: "pool-api",
		}},
		UpstreamPools: map[string]proxyconfig.UpstreamPool{
			"pool-api": {Upstreams: []string{"10.0.0.11:8080"}},
		},
	}
}

func stickyRouteSnapshot() runtime.Snapshot {
	return runtime.Snapshot{
		RouteTable: []route.Route{{
			GlobalID: "default:r-api", LocalID: "r-api", Source: "default",
			Enabled: true, Hosts: []string{"api.example.com"},
			Path:      route.PathMatcher{Kind: route.PathKindPrefix, Value: "/"},
			Algorithm: "sticky_cookie", UpstreamPool: "default:pool-api",
		}},
	}
}

func runtimeConfigSnapshot() runtime.Snapshot {
	return runtime.Snapshot{
		AppConfig: config.AppConfig{
			ProxyListenAddr: ":8080", DashboardListenAddr: ":9090", ProxyConfigDir: "configs/proxy",
		},
		AppliedAt: time.Unix(1700000000, 0).UTC(),
	}
}

const createStickyRouteBody = `{
	"id":"r-api",
	"enabled":true,
	"algorithm":"sticky_cookie",
	"match":{"hosts":["api.example.com"]},
	"upstream_pool":"pool-api"
}`

const createRouteValidationBody = `{
	"id":"r-api",
	"enabled":true,
	"match":{"hosts":["api.example.com"]},
	"upstream_pool":"pool-api"
}`

func namespacedConfigHandler(t *testing.T) http.Handler {
	return NewHandler(runtime.NewState(runtime.Snapshot{}), stubService{
		getNamespaceConfigFn: func(namespace string) (admin.NamespaceConfigView, error) {
			requireDefaultNamespace(t, namespace)
			return defaultNamespaceConfig(), nil
		},
	})
}

func createRouteHandler(t *testing.T) http.Handler {
	return NewHandler(runtime.NewState(runtime.Snapshot{}), stubService{
		createRouteFn: func(namespace string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error) {
			requireDefaultNamespace(t, namespace)
			requireRouteID(t, route.ID, "r-api")
			return route, nil
		},
	})
}

func createNamespaceHandler(t *testing.T) http.Handler {
	return NewHandler(runtime.NewState(runtime.Snapshot{}), stubService{
		createNamespaceFn: func(namespace string) (admin.NamespaceView, error) {
			requireNamespace(t, namespace, "admin")
			return admin.NamespaceView{Namespace: namespace, Path: "configs/proxy/admin.json", Exists: true}, nil
		},
	})
}

func validationErrorHandler() http.Handler {
	return NewHandler(runtime.NewState(runtime.Snapshot{}), stubService{
		createRouteFn: func(string, proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error) {
			return proxyconfig.RouteConfig{}, duplicateRouteAPIError()
		},
	})
}

func requireDefaultNamespace(t *testing.T, namespace string) {
	requireNamespace(t, namespace, admin.DefaultNamespace)
}

func requireNamespace(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("namespace = %q, want %q", got, want)
	}
}

func requireRouteID(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("route.ID = %q, want %q", got, want)
	}
}

func requireNamespaceConfigCounts(t *testing.T, body admin.NamespaceConfigView) {
	t.Helper()
	requireNamespace(t, body.Namespace, admin.DefaultNamespace)
	requireCount(t, "Routes", len(body.Routes), 1)
	requireCount(t, "UpstreamPools", len(body.UpstreamPools), 1)
}

func requireCount(t *testing.T, name string, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("len(%s) = %d, want %d", name, got, want)
	}
}

func duplicateRouteAPIError() *admin.APIError {
	return &admin.APIError{
		StatusCode: http.StatusUnprocessableEntity,
		Message:    "validation failed",
		ValidationErrors: []proxyconfig.ValidationError{
			{Field: "routes[0].id", Message: "duplicate route id"},
		},
	}
}

func TestNamespacedConfigEndpoint_ReturnsEditableConfig(t *testing.T) {
	rec := performDashboardRequest(namespacedConfigHandler(t), http.MethodGet, "/api/namespaces/default/config", "")
	requireStatus(t, rec, http.StatusOK)
	var body admin.NamespaceConfigView
	decodeJSON(t, rec, &body)
	requireNamespaceConfigCounts(t, body)
}

func TestCreateRouteEndpoint_CreatesRouteInNamespace(t *testing.T) {
	rec := performDashboardRequest(createRouteHandler(t), http.MethodPost, "/api/namespaces/default/routes", createStickyRouteBody)
	requireStatus(t, rec, http.StatusCreated)
	var body proxyconfig.RouteConfig
	decodeJSON(t, rec, &body)
	requireRouteID(t, body.ID, "r-api")
	if got, want := string(body.Algorithm), "sticky_cookie"; got != want {
		t.Fatalf("Algorithm = %q, want %q", got, want)
	}
}

func TestRuntimeConfigEndpoint_ExposesRouteAlgorithm(t *testing.T) {
	handler := NewHandler(runtime.NewState(stickyRouteSnapshot()), stubService{})
	rec := performDashboardRequest(handler, http.MethodGet, "/api/runtime/routes", "")
	requireStatus(t, rec, http.StatusOK)
	var routes []RouteView
	decodeJSON(t, rec, &routes)
	if len(routes) != 1 {
		t.Fatalf("len(routes) = %d, want 1", len(routes))
	}
	if got, want := routes[0].Algorithm, "sticky_cookie"; got != want {
		t.Fatalf("Algorithm = %q, want %q", got, want)
	}
}

func TestCreateNamespaceEndpoint_CreatesNamespace(t *testing.T) {
	rec := performDashboardRequest(createNamespaceHandler(t), http.MethodPost, "/api/namespaces", `{"namespace":"admin"}`)
	requireStatus(t, rec, http.StatusCreated)
	var body admin.NamespaceView
	decodeJSON(t, rec, &body)
	requireNamespace(t, body.Namespace, "admin")
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

	rec := performDashboardRequest(handler, http.MethodDelete, "/api/namespaces/admin", "")
	requireStatus(t, rec, http.StatusNoContent)
}

func TestRuntimeConfigEndpoint_ReturnsStructuredSnapshotView(t *testing.T) {
	handler := NewHandler(runtime.NewState(runtimeConfigSnapshot()), stubService{})
	rec := performDashboardRequest(handler, http.MethodGet, "/api/runtime/config", "")
	requireStatus(t, rec, http.StatusOK)
	var body SnapshotView
	decodeJSON(t, rec, &body)
	if got, want := body.AppConfig.ProxyConfigDir, "configs/proxy"; got != want {
		t.Fatalf("AppConfig.ProxyConfigDir = %q, want %q", got, want)
	}
}

func TestValidationError_ReturnsStructuredErrorBody(t *testing.T) {
	rec := performDashboardRequest(validationErrorHandler(), http.MethodPost, "/api/namespaces/default/routes", createRouteValidationBody)
	requireStatus(t, rec, http.StatusUnprocessableEntity)
	var body admin.APIError
	decodeJSON(t, rec, &body)
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
	rec := performDashboardRequest(handler, http.MethodGet, "/routes", "")
	requireStatus(t, rec, http.StatusOK)
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
