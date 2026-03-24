package proxyconfig

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

func (c Config) Validate() []ValidationError {
	var errs []ValidationError

	poolIDs := make(map[string]struct{}, len(c.UpstreamPools))
	for poolID, pool := range c.UpstreamPools {
		if strings.TrimSpace(poolID) == "" {
			errs = append(errs, ValidationError{
				Field:   "upstream_pools",
				Message: "pool id must not be empty",
			})
			continue
		}

		poolIDs[poolID] = struct{}{}
		errs = append(errs, validateUpstreamPool(poolID, pool)...)
	}

	routeIDs := make(map[string]struct{}, len(c.Routes))
	for i, route := range c.Routes {
		errs = append(errs, validateRoute(i, route, routeIDs, poolIDs)...)
	}

	return errs
}

func validateRoute(index int, route RouteConfig, routeIDs map[string]struct{}, poolIDs map[string]struct{}) []ValidationError {
	var errs []ValidationError
	base := fmt.Sprintf("routes[%d]", index)

	if strings.TrimSpace(route.ID) == "" {
		errs = append(errs, ValidationError{
			Field:   base + ".id",
			Message: "route id must not be empty",
		})
	} else {
		if _, exists := routeIDs[route.ID]; exists {
			errs = append(errs, ValidationError{
				Field:   base + ".id",
				Message: "duplicate route id",
			})
		}
		routeIDs[route.ID] = struct{}{}
	}

	if len(route.Match.Hosts) == 0 {
		errs = append(errs, ValidationError{
			Field:   base + ".match.hosts",
			Message: "hosts must contain at least one host",
		})
	} else {
		for i, host := range route.Match.Hosts {
			if strings.TrimSpace(host) == "" {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("%s.match.hosts[%d]", base, i),
					Message: "host must not be empty",
				})
			}
			if strings.Contains(host, "*") {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("%s.match.hosts[%d]", base, i),
					Message: "wildcard host is not supported in the current schema",
				})
			}
		}
	}

	if route.Match.Path != nil {
		errs = append(errs, validatePathMatch(base+".match.path", *route.Match.Path)...)
	}

	if strings.TrimSpace(route.UpstreamPool) == "" {
		errs = append(errs, ValidationError{
			Field:   base + ".upstream_pool",
			Message: "upstream_pool must not be empty",
		})
	} else if _, exists := poolIDs[route.UpstreamPool]; !exists {
		errs = append(errs, ValidationError{
			Field:   base + ".upstream_pool",
			Message: "referenced upstream_pool does not exist",
		})
	}

	return errs
}

func validatePathMatch(field string, path PathMatchConfig) []ValidationError {
	var errs []ValidationError

	if strings.TrimSpace(path.Value) == "" {
		errs = append(errs, ValidationError{
			Field:   field + ".value",
			Message: "path value must not be empty",
		})
		return errs
	}

	switch path.Type {
	case PathMatchExact:
		if !strings.HasPrefix(path.Value, "/") {
			errs = append(errs, ValidationError{
				Field:   field + ".value",
				Message: "exact path must start with '/'",
			})
		}

	case PathMatchPrefix:
		if !strings.HasPrefix(path.Value, "/") {
			errs = append(errs, ValidationError{
				Field:   field + ".value",
				Message: "prefix path must start with '/'",
			})
		}
		if path.Value != "/" && !strings.HasSuffix(path.Value, "/") {
			errs = append(errs, ValidationError{
				Field:   field + ".value",
				Message: "prefix path must be '/' or end with '/'",
			})
		}

	case PathMatchRegex:
		if _, err := regexp.Compile(path.Value); err != nil {
			errs = append(errs, ValidationError{
				Field:   field + ".value",
				Message: "invalid regex: " + err.Error(),
			})
		}

	default:
		errs = append(errs, ValidationError{
			Field:   field + ".type",
			Message: "path.type must be one of: exact, prefix, regex",
		})
	}

	return errs
}

func validateUpstreamPool(poolID string, pool UpstreamPool) []ValidationError {
	var errs []ValidationError
	base := "upstream_pools." + poolID

	if len(pool.Upstreams) == 0 {
		errs = append(errs, ValidationError{
			Field:   base + ".upstreams",
			Message: "upstreams must contain at least one entry",
		})
	} else {
		for i, upstream := range pool.Upstreams {
			if strings.TrimSpace(upstream) == "" {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("%s.upstreams[%d]", base, i),
					Message: "upstream must not be empty",
				})
				continue
			}

			host, port, err := net.SplitHostPort(upstream)
			if err != nil || host == "" || port == "" {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("%s.upstreams[%d]", base, i),
					Message: "upstream must be in host:port or [ipv6]:port form",
				})
			}
		}
	}

	if pool.HealthCheck != nil {
		errs = append(errs, validateHealthCheck(base+".health_check", *pool.HealthCheck)...)
	}

	return errs
}

func validateHealthCheck(field string, hc HealthCheckConfig) []ValidationError {
	var errs []ValidationError

	if !strings.HasPrefix(hc.Path, "/") {
		errs = append(errs, ValidationError{
			Field:   field + ".path",
			Message: "health_check.path must start with '/'",
		})
	}

	if _, err := hc.Interval.Parse(); err != nil {
		errs = append(errs, ValidationError{
			Field:   field + ".interval",
			Message: "invalid duration: " + err.Error(),
		})
	}

	if _, err := hc.Timeout.Parse(); err != nil {
		errs = append(errs, ValidationError{
			Field:   field + ".timeout",
			Message: "invalid duration: " + err.Error(),
		})
	}

	if hc.ExpectStatus < 100 || hc.ExpectStatus > 599 {
		errs = append(errs, ValidationError{
			Field:   field + ".expect_status",
			Message: "expect_status must be between 100 and 599",
		})
	}

	return errs
}
