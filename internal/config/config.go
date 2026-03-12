package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type Config struct {
	ProxyListenAddr     string `json:"proxyListenAddr"`
	DashboardListenAddr string `json:"dashboardListenAddr"`
}

func Default() Config {
	return Config{
		ProxyListenAddr:     ":8080",
		DashboardListenAddr: ":9090",
	}
}

func Load(path string) (Config, error) {
	if path == "" {
		return Config{}, errors.New("config path is required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}

	if err := Validate(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func Validate(cfg Config) error {
	if cfg.ProxyListenAddr == "" {
		return errors.New("proxy listen address is required")
	}
	if cfg.DashboardListenAddr == "" {
		return errors.New("dashboard listen address is required")
	}

	return nil
}
