package dashboard

import (
	"net/http"

	"reverseproxy-poc/internal/runtime"
)

func registerRuntimeAPI(mux *http.ServeMux, state *runtime.State) {
	mux.HandleFunc("/api/runtime/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeAPIError(w, newMethodNotAllowedError())
			return
		}

		writeJSON(w, buildSnapshotView(state.Snapshot()))
	})
	mux.HandleFunc("/api/app-config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeAPIError(w, newMethodNotAllowedError())
			return
		}

		writeJSON(w, state.Snapshot().AppConfig)
	})
	mux.HandleFunc("/api/proxy-configs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeAPIError(w, newMethodNotAllowedError())
			return
		}

		writeJSON(w, buildProxyConfigViews(state.Snapshot().ProxyConfigs))
	})
	mux.HandleFunc("/api/runtime/routes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeAPIError(w, newMethodNotAllowedError())
			return
		}

		writeJSON(w, buildRouteViews(state.Snapshot().RouteTable))
	})
	mux.HandleFunc("/api/upstreams", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeAPIError(w, newMethodNotAllowedError())
			return
		}

		writeJSON(w, buildUpstreamViews(state.Snapshot().Upstreams))
	})
}
