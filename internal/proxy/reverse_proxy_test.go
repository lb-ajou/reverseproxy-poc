package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/route"
	"reverseproxy-poc/internal/runtime"
	"reverseproxy-poc/internal/upstream"
)

func TestHandlerServeHTTP_ProxiesMatchedRequest(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend:" + r.URL.Path))
	}))
	defer backend.Close()

	targetHost := backend.Listener.Addr().String()
	registry, err := upstream.NewRegistry([]upstream.Pool{
		{
			GlobalID: "default:pool-api",
			Targets: []upstream.Target{
				{Raw: targetHost},
			},
		},
	})
	if err != nil {
		t.Fatalf("upstream.NewRegistry() error = %v", err)
	}

	snapshot := runtime.NewSnapshot(
		config.AppConfig{},
		nil,
		[]route.Route{
			{
				GlobalID:     "default:r-api",
				Enabled:      true,
				Hosts:        []string{"api.example.com"},
				Path:         route.PathMatcher{Kind: route.PathKindPrefix, Value: "/api/"},
				UpstreamPool: "default:pool-api",
			},
		},
		registry,
	)

	handler := NewHandler(runtime.NewState(snapshot))

	req := httptest.NewRequest(http.MethodGet, "http://api.example.com/api/users", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}

	if got, want := res.StatusCode, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := string(body), "backend:/api/users"; got != want {
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
