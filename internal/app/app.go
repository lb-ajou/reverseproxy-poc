package app

import (
	"fmt"
	"log"
	"net/http"

	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/dashboard"
	"reverseproxy-poc/internal/proxy"
	appruntime "reverseproxy-poc/internal/runtime"
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

	state := appruntime.NewState(appruntime.NewSnapshot(cfg, nil, nil, nil))

	app := &App{
		logger:           logger,
		configPath:       configPath,
		state:            state,
		proxyHandler:     proxy.NewHandler(),
		dashboardHandler: dashboard.NewHandler(state),
	}

	app.proxyServer = newServer(cfg.ProxyListenAddr, app.proxyHandler)
	app.dashboardServer = newServer(cfg.DashboardListenAddr, app.dashboardHandler)

	return app, nil
}

func (a *App) Snapshot() appruntime.Snapshot {
	return a.state.Snapshot()
}
