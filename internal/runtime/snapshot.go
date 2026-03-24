package runtime

import (
	"time"

	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/proxyconfig"
	"reverseproxy-poc/internal/route"
	"reverseproxy-poc/internal/upstream"
)

type Snapshot struct {
	AppConfig    config.AppConfig
	ProxyConfigs []proxyconfig.LoadedConfig
	RouteTable   []route.Route
	Upstreams    *upstream.Registry
	AppliedAt    time.Time
}
