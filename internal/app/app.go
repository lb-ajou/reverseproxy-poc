package app

import (
	"fmt"
	"log"
	"net/http"

	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/dashboard"
	"reverseproxy-poc/internal/proxy"
	"reverseproxy-poc/internal/proxyconfig"
	"reverseproxy-poc/internal/route"
	appruntime "reverseproxy-poc/internal/runtime"
	"reverseproxy-poc/internal/upstream"
)

type App struct {
	logger           *log.Logger
	configPath       string
	state            *appruntime.State
	proxyHandler     http.Handler
	dashboardHandler http.Handler
	proxyServer      *http.Server
	dashboardServer  *http.Server
}

func New(cfg config.AppConfig, configPath string, logger *log.Logger) (*App, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	if err := config.Validate(cfg); err != nil {
		return nil, err
	}

	snapshot, err := buildSnapshot(cfg)
	if err != nil {
		return nil, err
	}

	state := appruntime.NewState(snapshot)

	app := &App{
		logger:           logger,
		configPath:       configPath,
		state:            state,
		proxyHandler:     proxy.NewHandler(state),
		dashboardHandler: dashboard.NewHandler(state),
	}

	app.proxyServer = newServer(cfg.ProxyListenAddr, app.proxyHandler)
	app.dashboardServer = newServer(cfg.DashboardListenAddr, app.dashboardHandler)

	return app, nil
}

func (a *App) Snapshot() appruntime.Snapshot {
	return a.state.Snapshot()
}

func buildSnapshot(appCfg config.AppConfig) (appruntime.Snapshot, error) {
	proxyCfgs, err := proxyconfig.LoadDir(appCfg.ProxyConfigDir)
	if err != nil {
		return appruntime.Snapshot{}, fmt.Errorf("load proxy configs: %w", err)
	}

	routes, err := route.BuildTable(proxyCfgs)
	if err != nil {
		return appruntime.Snapshot{}, fmt.Errorf("build route table: %w", err)
	}

	upstreams, err := upstream.BuildRegistry(proxyCfgs)
	if err != nil {
		return appruntime.Snapshot{}, fmt.Errorf("build upstream registry: %w", err)
	}

	return appruntime.NewSnapshot(appCfg, proxyCfgs, routes, upstreams), nil
}
