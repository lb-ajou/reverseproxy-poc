package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"reverseproxy-poc/internal/route"
	"reverseproxy-poc/internal/runtime"
)

type Handler struct {
	state     *runtime.State
	transport http.RoundTripper
}

func NewHandler(state *runtime.State) http.Handler {
	return &Handler{
		state:     state,
		transport: http.DefaultTransport,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.state == nil {
		http.Error(w, "proxy runtime state is not configured", http.StatusBadGateway)
		return
	}

	snapshot := h.state.Snapshot()
	matchedRoute, ok := route.Resolve(snapshot.RouteTable, r.Host, r.URL.Path)
	if !ok {
		http.Error(w, "no matching route", http.StatusNotFound)
		return
	}

	if snapshot.Upstreams == nil {
		http.Error(w, "upstream registry is not configured", http.StatusBadGateway)
		return
	}

	pool, ok := snapshot.Upstreams.Get(matchedRoute.UpstreamPool)
	if !ok {
		http.Error(w, "matched upstream pool does not exist", http.StatusBadGateway)
		return
	}

	target, ok := pool.NextTarget()
	if !ok {
		http.Error(w, "matched upstream pool has no healthy targets", http.StatusBadGateway)
		return
	}

	targetURL, err := upstreamURL(target.Raw)
	if err != nil {
		http.Error(w, "invalid upstream target", http.StatusBadGateway)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.Transport = h.transport
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
		http.Error(rw, fmt.Sprintf("proxy upstream error: %v", proxyErr), http.StatusBadGateway)
	}
	proxy.ServeHTTP(w, r)
}

func upstreamURL(raw string) (*url.URL, error) {
	return url.Parse("http://" + raw)
}
