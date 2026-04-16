package route

import (
	"fmt"
	"regexp"

	"reverseproxy-poc/internal/proxyconfig"
)

func BuildTable(configs []proxyconfig.LoadedConfig) ([]Route, error) {
	routes := make([]Route, 0)
	for _, loaded := range configs {
		compiled, err := BuildRoutes(loaded.Source, loaded.Config)
		if err != nil {
			return nil, err
		}
		routes = append(routes, compiled...)
	}

	Sort(routes)

	return routes, nil
}

func BuildRoutes(source string, cfg proxyconfig.Config) ([]Route, error) {
	routes := make([]Route, 0, len(cfg.Routes))
	for _, routeCfg := range cfg.Routes {
		path, err := compilePathMatcher(routeCfg.Match.Path)
		if err != nil {
			return nil, fmt.Errorf("compile route %q from source %q: %w", routeCfg.ID, source, err)
		}

		routes = append(routes, Route{
			GlobalID:     GlobalRouteID(source, routeCfg.ID),
			LocalID:      routeCfg.ID,
			Source:       source,
			Enabled:      routeCfg.Enabled,
			Hosts:        append([]string(nil), routeCfg.Match.Hosts...),
			Path:         path,
			UpstreamPool: GlobalPoolID(source, routeCfg.UpstreamPool),
		})
	}

	return routes, nil
}

func GlobalRouteID(source, localID string) string {
	return source + ":" + localID
}

func GlobalPoolID(source, localID string) string {
	return source + ":" + localID
}

func compilePathMatcher(path *proxyconfig.PathMatchConfig) (PathMatcher, error) {
	if path == nil {
		return PathMatcher{Kind: PathKindAny}, nil
	}

	switch path.Type {
	case proxyconfig.PathMatchExact:
		return PathMatcher{
			Kind:  PathKindExact,
			Value: path.Value,
		}, nil
	case proxyconfig.PathMatchPrefix:
		return PathMatcher{
			Kind:  PathKindPrefix,
			Value: path.Value,
		}, nil
	case proxyconfig.PathMatchRegex:
		compiled, err := regexp.Compile(path.Value)
		if err != nil {
			return PathMatcher{}, err
		}
		return PathMatcher{
			Kind:  PathKindRegex,
			Value: path.Value,
			Regex: compiled,
		}, nil
	default:
		return PathMatcher{}, fmt.Errorf("unsupported path match type %q", path.Type)
	}
}
