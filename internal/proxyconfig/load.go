package proxyconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func LoadFile(path string) (LoadedConfig, error) {
	source, err := SourceFromPath(path)
	if err != nil {
		return LoadedConfig{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return LoadedConfig{}, fmt.Errorf("read proxy config %q: %w", path, err)
	}

	return Decode(source, path, data)
}

func LoadDir(dir string) ([]LoadedConfig, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("proxy config directory is required")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read proxy config directory %q: %w", dir, err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		paths = append(paths, filepath.Join(dir, entry.Name()))
	}

	sort.Strings(paths)

	loaded := make([]LoadedConfig, 0, len(paths))
	for _, path := range paths {
		cfg, err := LoadFile(path)
		if err != nil {
			return nil, err
		}
		loaded = append(loaded, cfg)
	}

	return loaded, nil
}

func Decode(source, path string, data []byte) (LoadedConfig, error) {
	if strings.TrimSpace(source) == "" {
		return LoadedConfig{}, errors.New("proxy config source is required")
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return LoadedConfig{}, fmt.Errorf("decode proxy config %q: %w", path, err)
	}

	if errs := cfg.Validate(); len(errs) > 0 {
		return LoadedConfig{}, ValidationErrors(errs)
	}

	return LoadedConfig{
		Source: source,
		Path:   path,
		Config: cfg,
	}, nil
}

func SourceFromPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("proxy config path is required")
	}

	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != ".json" {
		return "", fmt.Errorf("proxy config %q must use .json extension", path)
	}

	source := strings.TrimSuffix(base, ext)
	source = strings.TrimSpace(source)
	if source == "" {
		return "", fmt.Errorf("proxy config %q has empty source name", path)
	}

	return source, nil
}
