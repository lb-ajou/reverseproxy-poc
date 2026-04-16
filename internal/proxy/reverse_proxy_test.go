package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/route"
	"reverseproxy-poc/internal/runtime"
	"reverseproxy-poc/internal/upstream"
)

func newTestRoute(algorithm string) route.Route {
	return route.Route{
		GlobalID: "default:r-api", LocalID: "r-api", Source: "default",
		Enabled: true, Hosts: []string{"api.example.com"},
		Path:      route.PathMatcher{Kind: route.PathKindPrefix, Value: "/api/"},
		Algorithm: algorithm, UpstreamPool: "default:pool-api",
	}
}

func newRegistry(t *testing.T, raws ...string) *upstream.Registry {
	t.Helper()
	targets := make([]upstream.Target, 0, len(raws))
	for _, raw := range raws {
		targets = append(targets, upstream.Target{Raw: raw})
	}
	registry, err := upstream.NewRegistry([]upstream.Pool{{GlobalID: "default:pool-api", Targets: targets}})
	if err != nil {
		t.Fatalf("upstream.NewRegistry() error = %v", err)
	}
	return registry
}

func newProxyHandler(routes []route.Route, registry *upstream.Registry) http.Handler {
	snapshot := runtime.NewSnapshot(config.AppConfig{}, nil, routes, registry)
	return NewHandler(runtime.NewState(snapshot))
}

func requireBody(t *testing.T, body io.Reader) string {
	t.Helper()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	return string(data)
}

func newBackendServer(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body + r.URL.Path))
	}))
}

func newJSONServer(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
}

func requireStatusCode(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if got := rec.Result().StatusCode; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func markAllTargetsUnhealthy(t *testing.T, registry *upstream.Registry) {
	t.Helper()
	pool, ok := registry.Get("default:pool-api")
	if !ok {
		t.Fatal("registry.Get() returned no pool")
	}
	now := time.Now()
	pool.SetTargetUnhealthy(0, now, "timeout")
	pool.SetTargetUnhealthy(1, now, "timeout")
}

func stickyBodies(t *testing.T, handler http.Handler) (string, string) {
	t.Helper()
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, httptest.NewRequest(http.MethodGet, "http://api.example.com/api/info", nil))
	cookies := firstRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected sticky cookie to be set")
	}
	secondReq := httptest.NewRequest(http.MethodGet, "http://api.example.com/api/info", nil)
	secondReq.AddCookie(cookies[0])
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	return requireBody(t, firstRec.Result().Body), requireBody(t, secondRec.Result().Body)
}

func TestHandlerServeHTTP_ProxiesMatchedRequest(t *testing.T) {
	backend := newBackendServer("backend:")
	defer backend.Close()
	handler := newProxyHandler([]route.Route{newTestRoute("round_robin")}, newRegistry(t, backend.Listener.Addr().String()))
	req := httptest.NewRequest(http.MethodGet, "http://api.example.com/api/users", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	res := rec.Result()
	defer res.Body.Close()
	requireStatusCode(t, rec, http.StatusOK)
	if got, want := requireBody(t, res.Body), "backend:/api/users"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestHandlerServeHTTP_ReturnsNotFoundWhenNoRouteMatches(t *testing.T) {
	snapshot := runtime.NewSnapshot(config.AppConfig{}, nil, nil, nil)
	handler := NewHandler(runtime.NewState(snapshot))

	req := httptest.NewRequest(http.MethodGet, "http://api.example.com/api/users", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got, want := rec.Result().StatusCode, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestHandlerServeHTTP_ReturnsBadGatewayWhenNoHealthyTargets(t *testing.T) {
	registry := newRegistry(t, "10.0.0.11:8080", "10.0.0.12:8080")
	markAllTargetsUnhealthy(t, registry)
	handler := newProxyHandler([]route.Route{newTestRoute("round_robin")}, registry)
	req := httptest.NewRequest(http.MethodGet, "http://api.example.com/api/users", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	requireStatusCode(t, rec, http.StatusBadGateway)
}

func TestHandlerServeHTTP_StickyCookieSetsCookieAndReusesTarget(t *testing.T) {
	serverA := newJSONServer(`{"server":"a"}`)
	defer serverA.Close()
	serverB := newJSONServer(`{"server":"b"}`)
	defer serverB.Close()
	registry := newRegistry(t, serverA.Listener.Addr().String(), serverB.Listener.Addr().String())
	handler := newProxyHandler([]route.Route{newTestRoute("sticky_cookie")}, registry)
	firstBody, secondBody := stickyBodies(t, handler)
	if firstBody != secondBody {
		t.Fatalf("sticky response mismatch: %q != %q", firstBody, secondBody)
	}
}

func TestFindHealthyTarget_ReturnsFalseForUnhealthyTarget(t *testing.T) {
	pool := &upstream.Pool{
		Targets: []upstream.Target{
			{Raw: "127.0.0.1:18081"},
			{Raw: "127.0.0.1:18082"},
		},
	}
	pool.SetTargetUnhealthy(1, time.Now(), "failed")

	if _, ok := findHealthyTarget(pool, "127.0.0.1:18082"); ok {
		t.Fatal("findHealthyTarget() returned unhealthy target")
	}
}
