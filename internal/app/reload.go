package app

import (
	"context"
	"fmt"

	"reverseproxy-poc/internal/config"
)

func (a *App) Reload(_ context.Context, cfg config.AppConfig) error {
	if err := config.Validate(cfg); err != nil {
		return err
	}

	current := a.state.Snapshot().AppConfig
	if current.ProxyListenAddr != cfg.ProxyListenAddr ||
		current.DashboardListenAddr != cfg.DashboardListenAddr ||
		current.ProxyConfigDir != cfg.ProxyConfigDir {
		return fmt.Errorf("listen address changes require restart in current POC")
	}

	snapshot, err := buildSnapshot(cfg)
	if err != nil {
		return err
	}

	a.state.Swap(snapshot)
	a.logger.Printf("configuration reloaded from memory")

	return nil
}

func (a *App) ReloadFromFile(ctx context.Context) error {
	cfg, err := config.Load(a.configPath)
	if err != nil {
		return err
	}

	if err := a.Reload(ctx, cfg); err != nil {
		return err
	}

	a.logger.Printf("configuration reloaded from %s", a.configPath)

	return nil
}
