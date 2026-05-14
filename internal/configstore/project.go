package configstore

import (
	"fmt"
	"path/filepath"
	"sort"

	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/proxyconfig"
	"reverseproxy-poc/internal/route"
	appruntime "reverseproxy-poc/internal/runtime"
	"reverseproxy-poc/internal/upstream"
)

func ProjectSnapshot(appCfg config.AppConfig, desired DesiredState) (appruntime.Snapshot, error) {
	loaded, err := LoadedConfigs(appCfg.ProxyConfigDir, desired)
	if err != nil {
		return appruntime.Snapshot{}, err
	}
	routes, err := route.BuildTable(loaded)
	if err != nil {
		return appruntime.Snapshot{}, fmt.Errorf("build route table: %w", err)
	}
	upstreams, err := upstream.BuildRegistry(loaded)
	if err != nil {
		return appruntime.Snapshot{}, fmt.Errorf("build upstream registry: %w", err)
	}
	snapshot := appruntime.NewSnapshot(appCfg, loaded, routes, upstreams)
	if !desired.AppliedAt.IsZero() {
		snapshot.AppliedAt = desired.AppliedAt
	}
	return snapshot, nil
}

func LoadedConfigs(dir string, desired DesiredState) ([]proxyconfig.LoadedConfig, error) {
	namespaces := sortedNamespaces(desired.Namespaces)
	loaded := make([]proxyconfig.LoadedConfig, 0, len(namespaces))
	for _, namespace := range namespaces {
		cfg := normalizeConfig(desired.Namespaces[namespace])
		if errs := cfg.Validate(); len(errs) > 0 {
			return nil, proxyconfig.ValidationErrors(errs)
		}
		loaded = append(loaded, proxyconfig.LoadedConfig{
			Source: namespace,
			Path:   filepath.Join(dir, namespace+".json"),
			Config: cfg,
		})
	}
	return loaded, nil
}

func sortedNamespaces(configs map[string]proxyconfig.Config) []string {
	namespaces := make([]string, 0, len(configs))
	for namespace := range configs {
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)
	return namespaces
}

func normalizeConfig(cfg proxyconfig.Config) proxyconfig.Config {
	if cfg.Routes == nil {
		cfg.Routes = []proxyconfig.RouteConfig{}
	}
	if cfg.UpstreamPools == nil {
		cfg.UpstreamPools = map[string]proxyconfig.UpstreamPool{}
	}
	return cfg
}
