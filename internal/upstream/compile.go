package upstream

import (
	"fmt"

	"reverseproxy-poc/internal/proxyconfig"
)

func BuildRegistry(configs []proxyconfig.LoadedConfig) (*Registry, error) {
	pools := make([]Pool, 0)
	for _, loaded := range configs {
		compiled, err := BuildPools(loaded.Source, loaded.Config)
		if err != nil {
			return nil, err
		}
		pools = append(pools, compiled...)
	}

	return NewRegistry(pools)
}

func BuildPools(source string, cfg proxyconfig.Config) ([]Pool, error) {
	pools := make([]Pool, 0, len(cfg.UpstreamPools))
	for localID, poolCfg := range cfg.UpstreamPools {
		pool, err := buildPool(source, localID, poolCfg)
		if err != nil {
			return nil, fmt.Errorf("build upstream pool %q from source %q: %w", localID, source, err)
		}
		pools = append(pools, pool)
	}

	return pools, nil
}

func GlobalPoolID(source, localID string) string {
	return source + ":" + localID
}

func buildPool(source, localID string, poolCfg proxyconfig.UpstreamPool) (Pool, error) {
	targets := make([]Target, 0, len(poolCfg.Upstreams))
	for _, upstream := range poolCfg.Upstreams {
		targets = append(targets, Target{Raw: upstream})
	}

	targetStates := make([]TargetState, 0, len(targets))
	for range targets {
		targetStates = append(targetStates, TargetState{
			Healthy: true,
		})
	}

	return Pool{
		GlobalID:    GlobalPoolID(source, localID),
		LocalID:     localID,
		Source:      source,
		Targets:     targets,
		HealthCheck: buildHealthCheck(poolCfg.HealthCheck),
		targetState: targetStates,
	}, nil
}

func buildHealthCheck(hc *proxyconfig.HealthCheckConfig) *HealthCheck {
	if hc == nil {
		return nil
	}

	return &HealthCheck{
		Path:         hc.Path,
		Interval:     string(hc.Interval),
		Timeout:      string(hc.Timeout),
		ExpectStatus: hc.ExpectStatus,
	}
}
