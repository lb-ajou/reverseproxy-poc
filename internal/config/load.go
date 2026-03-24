package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

func Load(path string) (AppConfig, error) {
	if path == "" {
		return AppConfig{}, errors.New("config path is required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return AppConfig{}, fmt.Errorf("read config: %w", err)
	}

	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return AppConfig{}, fmt.Errorf("decode config: %w", err)
	}

	if err := Validate(cfg); err != nil {
		return AppConfig{}, err
	}

	return cfg, nil
}
