package dashboard

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"reverseproxy-poc/internal/admin"
	"reverseproxy-poc/internal/proxyconfig"
)

type upstreamPoolRequest struct {
	ID          string                         `json:"id"`
	Upstreams   []string                       `json:"upstreams"`
	HealthCheck *proxyconfig.HealthCheckConfig `json:"health_check,omitempty"`
}

type upstreamPoolResponse struct {
	ID   string                   `json:"id"`
	Pool proxyconfig.UpstreamPool `json:"pool"`
}

type namespaceRequest struct {
	Namespace string `json:"namespace"`
}

func registerConfigAPI(mux *http.ServeMux, service admin.Service) {
	mux.HandleFunc("/api/namespaces", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			items, err := service.ListNamespaces(r.Context())
			if err != nil {
				writeAPIError(w, err)
				return
			}

			writeJSON(w, admin.NamespaceListView{
				Items:            items,
				DefaultNamespace: admin.DefaultNamespace,
			})
		case http.MethodPost:
			var request namespaceRequest
			if err := decodeJSONBody(r, &request); err != nil {
				writeAPIError(w, err)
				return
			}

			created, err := service.CreateNamespace(r.Context(), request.Namespace)
			if err != nil {
				writeAPIError(w, err)
				return
			}

			writeJSONStatus(w, http.StatusCreated, created)
		default:
			writeAPIError(w, newMethodNotAllowedError())
		}
	})
	mux.HandleFunc("/api/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		namespace, rest, ok := namespacePathParts(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if rest == "" {
			handleNamespaceRoot(w, r, service, namespace)
			return
		}

		switch {
		case rest == "config":
			handleNamespaceConfig(w, r, service, namespace)
		case rest == "routes":
			handleNamespaceRoutes(w, r, service, namespace)
		case strings.HasPrefix(rest, "routes/"):
			handleNamespaceRouteByID(w, r, service, namespace, strings.TrimPrefix(rest, "routes/"))
		case rest == "upstream-pools":
			handleNamespaceUpstreamPools(w, r, service, namespace)
		case strings.HasPrefix(rest, "upstream-pools/"):
			handleNamespaceUpstreamPoolByID(w, r, service, namespace, strings.TrimPrefix(rest, "upstream-pools/"))
		default:
			http.NotFound(w, r)
		}
	})
}

func handleNamespaceRoot(w http.ResponseWriter, r *http.Request, service admin.Service, namespace string) {
	switch r.Method {
	case http.MethodDelete:
		if err := service.DeleteNamespace(r.Context(), namespace); err != nil {
			writeAPIError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeAPIError(w, newMethodNotAllowedError())
	}
}

func handleNamespaceConfig(w http.ResponseWriter, r *http.Request, service admin.Service, namespace string) {
	if r.Method != http.MethodGet {
		writeAPIError(w, newMethodNotAllowedError())
		return
	}

	configView, err := service.GetNamespaceConfig(r.Context(), namespace)
	if err != nil {
		writeAPIError(w, err)
		return
	}

	writeJSON(w, configView)
}

func handleNamespaceRoutes(w http.ResponseWriter, r *http.Request, service admin.Service, namespace string) {
	switch r.Method {
	case http.MethodGet:
		routes, err := service.GetNamespaceRoutes(r.Context(), namespace)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, routes)
	case http.MethodPost:
		var routeCfg proxyconfig.RouteConfig
		if err := decodeJSONBody(r, &routeCfg); err != nil {
			writeAPIError(w, err)
			return
		}

		created, err := service.CreateRoute(r.Context(), namespace, routeCfg)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSONStatus(w, http.StatusCreated, created)
	default:
		writeAPIError(w, newMethodNotAllowedError())
	}
}

func handleNamespaceRouteByID(w http.ResponseWriter, r *http.Request, service admin.Service, namespace, encodedID string) {
	id, err := url.PathUnescape(encodedID)
	if err != nil || id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var routeCfg proxyconfig.RouteConfig
		if err := decodeJSONBody(r, &routeCfg); err != nil {
			writeAPIError(w, err)
			return
		}

		updated, err := service.UpdateRoute(r.Context(), namespace, id, routeCfg)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, updated)
	case http.MethodDelete:
		if err := service.DeleteRoute(r.Context(), namespace, id); err != nil {
			writeAPIError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeAPIError(w, newMethodNotAllowedError())
	}
}

func handleNamespaceUpstreamPools(w http.ResponseWriter, r *http.Request, service admin.Service, namespace string) {
	switch r.Method {
	case http.MethodGet:
		pools, err := service.GetNamespaceUpstreamPools(r.Context(), namespace)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, pools)
	case http.MethodPost:
		var request upstreamPoolRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, err)
			return
		}

		created, err := service.CreateUpstreamPool(r.Context(), namespace, request.ID, request.pool())
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSONStatus(w, http.StatusCreated, upstreamPoolResponse{ID: request.ID, Pool: created})
	default:
		writeAPIError(w, newMethodNotAllowedError())
	}
}

func handleNamespaceUpstreamPoolByID(w http.ResponseWriter, r *http.Request, service admin.Service, namespace, encodedID string) {
	id, err := url.PathUnescape(encodedID)
	if err != nil || id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var pool proxyconfig.UpstreamPool
		if err := decodeJSONBody(r, &pool); err != nil {
			writeAPIError(w, err)
			return
		}

		updated, err := service.UpdateUpstreamPool(r.Context(), namespace, id, pool)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, updated)
	case http.MethodDelete:
		if err := service.DeleteUpstreamPool(r.Context(), namespace, id); err != nil {
			writeAPIError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeAPIError(w, newMethodNotAllowedError())
	}
}

func pathTail(path, prefix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}

	encodedTail := strings.TrimPrefix(path, prefix)
	if encodedTail == "" {
		return "", false
	}

	tail, err := url.PathUnescape(encodedTail)
	if err != nil || tail == "" || strings.Contains(tail, "/") {
		return "", false
	}

	return tail, true
}

func namespacePathParts(path string) (namespace, rest string, ok bool) {
	const prefix = "/api/namespaces/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}

	trimmed := strings.TrimPrefix(path, prefix)
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	namespace, err := url.PathUnescape(parts[0])
	if err != nil || namespace == "" || strings.Contains(namespace, "/") {
		return "", "", false
	}

	if len(parts) == 1 {
		return namespace, "", true
	}

	for _, part := range parts[1:] {
		if part == "" {
			return "", "", false
		}
	}

	return namespace, strings.Join(parts[1:], "/"), true
}

func (r upstreamPoolRequest) pool() proxyconfig.UpstreamPool {
	return proxyconfig.UpstreamPool{
		Upstreams:   append([]string(nil), r.Upstreams...),
		HealthCheck: cloneHealthCheck(r.HealthCheck),
	}
}

func decodeJSONBody(r *http.Request, target interface{}) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return &admin.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "invalid JSON request body",
			Err:        err,
		}
	}
	if err := decoder.Decode(&struct{}{}); err != nil && !errors.Is(err, io.EOF) {
		return &admin.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "request body must contain a single JSON object",
		}
	}
	return nil
}

func cloneHealthCheck(healthCheck *proxyconfig.HealthCheckConfig) *proxyconfig.HealthCheckConfig {
	if healthCheck == nil {
		return nil
	}

	cloned := *healthCheck
	return &cloned
}
