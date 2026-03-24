package config

import "errors"

func Validate(cfg AppConfig) error {
	if cfg.ProxyListenAddr == "" {
		return errors.New("proxy listen address is required")
	}
	if cfg.DashboardListenAddr == "" {
		return errors.New("dashboard listen address is required")
	}
	if cfg.ProxyConfigDir == "" {
		return errors.New("proxy config directory is required")
	}

	return nil
}
