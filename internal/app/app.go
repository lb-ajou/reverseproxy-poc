package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"

	"reverseproxy-poc/internal/admin"
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
	mu               sync.Mutex
	runCtx           context.Context
	healthCtx        context.Context
	healthCancel     context.CancelFunc
	healthChecker    *upstream.Checker
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
		logger:        logger,
		configPath:    configPath,
		state:         state,
		healthChecker: upstream.NewChecker(snapshot.Upstreams),
		proxyHandler:  proxy.NewHandler(state),
	}

	app.dashboardHandler = dashboard.NewHandler(state, admin.New(app))

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

func (a *App) startHealthChecker(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.healthCancel != nil {
		return
	}

	healthCtx, cancel := context.WithCancel(ctx)
	a.runCtx = ctx
	a.healthCtx = healthCtx
	a.healthCancel = cancel

	if a.healthChecker != nil {
		a.healthChecker.Start(healthCtx)
	}
}

func (a *App) stopHealthChecker() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.healthCancel != nil {
		a.healthCancel()
	}

	a.healthCtx = nil
	a.healthCancel = nil
	a.runCtx = nil
}

func (a *App) swapHealthChecker(registry *upstream.Registry) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.healthCancel != nil {
		a.healthCancel()
		a.healthCancel = nil
	}

	a.healthChecker = upstream.NewChecker(registry)

	if a.runCtx != nil && a.healthChecker != nil {
		healthCtx, cancel := context.WithCancel(a.runCtx)
		a.healthCtx = healthCtx
		a.healthCancel = cancel
		a.healthChecker.Start(healthCtx)
	}
}
