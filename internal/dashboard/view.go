package dashboard

import (
	"sort"
	"time"

	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/proxyconfig"
	"reverseproxy-poc/internal/route"
	"reverseproxy-poc/internal/runtime"
	"reverseproxy-poc/internal/upstream"
)

type SnapshotView struct {
	AppConfig    config.AppConfig   `json:"app_config"`
	ProxyConfigs []ProxyConfigView  `json:"proxy_configs"`
	RouteTable   []RouteView        `json:"route_table"`
	Upstreams    []UpstreamPoolView `json:"upstreams"`
	AppliedAt    time.Time          `json:"applied_at"`
}

type ProxyConfigView struct {
	Source string           `json:"source"`
	Path   string           `json:"path"`
	Name   string           `json:"name,omitempty"`
	Routes []ProxyRouteView `json:"routes"`
	Pools  []ProxyPoolView  `json:"upstream_pools"`
}

type ProxyRouteView struct {
	ID           string         `json:"id"`
	Enabled      bool           `json:"enabled"`
	Hosts        []string       `json:"hosts"`
	Path         *PathMatchView `json:"path,omitempty"`
	UpstreamPool string         `json:"upstream_pool"`
}

type ProxyPoolView struct {
	ID          string                         `json:"id"`
	Upstreams   []string                       `json:"upstreams"`
	HealthCheck *proxyconfig.HealthCheckConfig `json:"health_check,omitempty"`
}

type RouteView struct {
	GlobalID     string          `json:"global_id"`
	LocalID      string          `json:"local_id"`
	Source       string          `json:"source"`
	Enabled      bool            `json:"enabled"`
	Hosts        []string        `json:"hosts"`
	Path         PathMatcherView `json:"path"`
	UpstreamPool string          `json:"upstream_pool"`
}

type PathMatcherView struct {
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
}

type PathMatchView struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type UpstreamPoolView struct {
	GlobalID    string                `json:"global_id"`
	LocalID     string                `json:"local_id"`
	Source      string                `json:"source"`
	Targets     []string              `json:"targets"`
	HealthCheck *upstream.HealthCheck `json:"health_check,omitempty"`
}

func buildSnapshotView(snapshot runtime.Snapshot) SnapshotView {
	return SnapshotView{
		AppConfig:    snapshot.AppConfig,
		ProxyConfigs: buildProxyConfigViews(snapshot.ProxyConfigs),
		RouteTable:   buildRouteViews(snapshot.RouteTable),
		Upstreams:    buildUpstreamViews(snapshot.Upstreams),
		AppliedAt:    snapshot.AppliedAt,
	}
}

func buildProxyConfigViews(configs []proxyconfig.LoadedConfig) []ProxyConfigView {
	views := make([]ProxyConfigView, 0, len(configs))
	for _, loaded := range configs {
		views = append(views, ProxyConfigView{
			Source: loaded.Source,
			Path:   loaded.Path,
			Name:   loaded.Config.Name,
			Routes: buildProxyRouteViews(loaded.Config.Routes),
			Pools:  buildProxyPoolViews(loaded.Config.UpstreamPools),
		})
	}

	return views
}

func buildProxyRouteViews(routes []proxyconfig.RouteConfig) []ProxyRouteView {
	views := make([]ProxyRouteView, 0, len(routes))
	for _, item := range routes {
		view := ProxyRouteView{
			ID:           item.ID,
			Enabled:      item.Enabled,
			Hosts:        append([]string(nil), item.Match.Hosts...),
			UpstreamPool: item.UpstreamPool,
		}
		if item.Match.Path != nil {
			view.Path = &PathMatchView{
				Type:  string(item.Match.Path.Type),
				Value: item.Match.Path.Value,
			}
		}
		views = append(views, view)
	}

	return views
}

func buildProxyPoolViews(pools map[string]proxyconfig.UpstreamPool) []ProxyPoolView {
	ids := make([]string, 0, len(pools))
	for id := range pools {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	views := make([]ProxyPoolView, 0, len(ids))
	for _, id := range ids {
		pool := pools[id]
		views = append(views, ProxyPoolView{
			ID:          id,
			Upstreams:   append([]string(nil), pool.Upstreams...),
			HealthCheck: pool.HealthCheck,
		})
	}

	return views
}

func buildRouteViews(routes []route.Route) []RouteView {
	views := make([]RouteView, 0, len(routes))
	for _, item := range routes {
		views = append(views, RouteView{
			GlobalID:     item.GlobalID,
			LocalID:      item.LocalID,
			Source:       item.Source,
			Enabled:      item.Enabled,
			Hosts:        append([]string(nil), item.Hosts...),
			Path:         buildPathMatcherView(item.Path),
			UpstreamPool: item.UpstreamPool,
		})
	}

	return views
}

func buildPathMatcherView(path route.PathMatcher) PathMatcherView {
	return PathMatcherView{
		Kind:  pathKindString(path.Kind),
		Value: path.Value,
	}
}

func pathKindString(kind route.PathKind) string {
	switch kind {
	case route.PathKindExact:
		return "exact"
	case route.PathKindPrefix:
		return "prefix"
	case route.PathKindRegex:
		return "regex"
	case route.PathKindAny:
		return "any"
	default:
		return "unknown"
	}
}

func buildUpstreamViews(registry *upstream.Registry) []UpstreamPoolView {
	if registry == nil {
		return nil
	}

	pools := registry.All()
	sort.Slice(pools, func(i, j int) bool {
		return pools[i].GlobalID < pools[j].GlobalID
	})

	views := make([]UpstreamPoolView, 0, len(pools))
	for _, pool := range pools {
		targets := make([]string, 0, len(pool.Targets))
		for _, target := range pool.Targets {
			targets = append(targets, target.Raw)
		}

		views = append(views, UpstreamPoolView{
			GlobalID:    pool.GlobalID,
			LocalID:     pool.LocalID,
			Source:      pool.Source,
			Targets:     targets,
			HealthCheck: pool.HealthCheck,
		})
	}

	return views
}
