package configstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/proxyconfig"
)

const configFileMode = 0o644

var namespacePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type FileStore struct {
	appCfg config.AppConfig
	mu     sync.Mutex
}

type namespaceFile struct {
	path    string
	exists  bool
	rawData []byte
	config  proxyconfig.Config
}

func NewFileStore(appCfg config.AppConfig) *FileStore {
	return &FileStore{appCfg: appCfg}
}

func (s *FileStore) DesiredState(_ context.Context) (DesiredState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	loaded, err := proxyconfig.LoadDir(s.appCfg.ProxyConfigDir)
	if err != nil {
		return DesiredState{}, err
	}

	namespaces := make(map[string]proxyconfig.Config, len(loaded))
	for _, cfg := range loaded {
		namespaces[cfg.Source] = normalizeConfig(cfg.Config)
	}

	return DesiredState{
		Namespaces: namespaces,
		AppliedAt:  time.Now(),
	}, nil
}

func (s *FileStore) ListNamespaces(ctx context.Context) ([]NamespaceSummary, error) {
	state, err := s.DesiredState(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]NamespaceSummary, 0, len(state.Namespaces)+1)
	hasDefault := false
	for _, namespace := range sortedNamespaces(state.Namespaces) {
		if namespace == DefaultNamespace {
			hasDefault = true
		}
		cfg := normalizeConfig(state.Namespaces[namespace])
		items = append(items, NamespaceSummary{
			Namespace:         namespace,
			Path:              filepath.Join(s.appCfg.ProxyConfigDir, namespace+".json"),
			Exists:            true,
			RouteCount:        len(cfg.Routes),
			UpstreamPoolCount: len(cfg.UpstreamPools),
		})
	}

	if !hasDefault {
		items = append(items, NamespaceSummary{
			Namespace: DefaultNamespace,
			Path:      filepath.Join(s.appCfg.ProxyConfigDir, DefaultNamespace+".json"),
			Exists:    false,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Namespace < items[j].Namespace
	})

	return items, nil
}

func (s *FileStore) CreateNamespace(_ context.Context, namespace string) (NamespaceSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.namespacePath(namespace)
	if err != nil {
		return NamespaceSummary{}, err
	}

	if _, err := os.Stat(path); err == nil {
		return NamespaceSummary{}, conflictError(fmt.Sprintf("namespace %q already exists", namespace))
	} else if !errors.Is(err, os.ErrNotExist) {
		return NamespaceSummary{}, internalError("failed to inspect namespace config", err)
	}

	if err := writeConfigFileAtomic(path, normalizeConfig(proxyconfig.Config{})); err != nil {
		return NamespaceSummary{}, internalError("failed to create namespace config", err)
	}

	return NamespaceSummary{
		Namespace: namespace,
		Path:      path,
		Exists:    true,
	}, nil
}

func (s *FileStore) DeleteNamespace(_ context.Context, namespace string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.loadNamespaceFile(namespace)
	if err != nil {
		return err
	}
	if !file.exists {
		return notFoundError(fmt.Sprintf("namespace %q was not found", namespace))
	}

	if err := os.Remove(file.path); err != nil {
		return internalError("failed to delete namespace config", err)
	}

	return nil
}

func (s *FileStore) GetNamespaceConfig(_ context.Context, namespace string) (NamespaceConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.loadNamespaceFile(namespace)
	if err != nil {
		return NamespaceConfig{}, err
	}

	return NamespaceConfig{
		Namespace:     namespace,
		Exists:        file.exists,
		Routes:        cloneRoutes(file.config.Routes),
		UpstreamPools: cloneUpstreamPools(file.config.UpstreamPools),
		AppliedAt:     time.Now(),
	}, nil
}

func (s *FileStore) CreateRoute(ctx context.Context, namespace string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error) {
	savedRoute := cloneRoute(route)
	_, err := s.mutateNamespaceConfig(ctx, namespace, func(cfg *proxyconfig.Config) error {
		for _, existing := range cfg.Routes {
			if existing.ID == route.ID {
				return conflictError(fmt.Sprintf("route %q already exists", route.ID))
			}
		}

		cfg.Routes = append(cfg.Routes, cloneRoute(route))
		return nil
	})
	if err != nil {
		return proxyconfig.RouteConfig{}, err
	}

	return savedRoute, nil
}

func (s *FileStore) UpdateRoute(ctx context.Context, namespace, id string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error) {
	if route.ID != id {
		return proxyconfig.RouteConfig{}, &StoreError{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_request",
			Message:    "route id in body must match request path",
		}
	}

	_, err := s.mutateNamespaceConfig(ctx, namespace, func(cfg *proxyconfig.Config) error {
		for index, existing := range cfg.Routes {
			if existing.ID == id {
				cfg.Routes[index] = cloneRoute(route)
				return nil
			}
		}

		return notFoundError(fmt.Sprintf("route %q was not found", id))
	})
	if err != nil {
		return proxyconfig.RouteConfig{}, err
	}

	return cloneRoute(route), nil
}

func (s *FileStore) DeleteRoute(ctx context.Context, namespace, id string) error {
	_, err := s.mutateNamespaceConfig(ctx, namespace, func(cfg *proxyconfig.Config) error {
		for index, existing := range cfg.Routes {
			if existing.ID == id {
				cfg.Routes = append(cfg.Routes[:index], cfg.Routes[index+1:]...)
				return nil
			}
		}

		return notFoundError(fmt.Sprintf("route %q was not found", id))
	})
	return err
}

func (s *FileStore) CreateUpstreamPool(ctx context.Context, namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error) {
	savedPool := cloneUpstreamPool(pool)
	_, err := s.mutateNamespaceConfig(ctx, namespace, func(cfg *proxyconfig.Config) error {
		if _, exists := cfg.UpstreamPools[id]; exists {
			return conflictError(fmt.Sprintf("upstream pool %q already exists", id))
		}

		if cfg.UpstreamPools == nil {
			cfg.UpstreamPools = map[string]proxyconfig.UpstreamPool{}
		}
		cfg.UpstreamPools[id] = cloneUpstreamPool(pool)
		return nil
	})
	if err != nil {
		return proxyconfig.UpstreamPool{}, err
	}

	return savedPool, nil
}

func (s *FileStore) UpdateUpstreamPool(ctx context.Context, namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error) {
	_, err := s.mutateNamespaceConfig(ctx, namespace, func(cfg *proxyconfig.Config) error {
		if _, exists := cfg.UpstreamPools[id]; !exists {
			return notFoundError(fmt.Sprintf("upstream pool %q was not found", id))
		}

		cfg.UpstreamPools[id] = cloneUpstreamPool(pool)
		return nil
	})
	if err != nil {
		return proxyconfig.UpstreamPool{}, err
	}

	return cloneUpstreamPool(pool), nil
}

func (s *FileStore) DeleteUpstreamPool(ctx context.Context, namespace, id string) error {
	_, err := s.mutateNamespaceConfig(ctx, namespace, func(cfg *proxyconfig.Config) error {
		if _, exists := cfg.UpstreamPools[id]; !exists {
			return notFoundError(fmt.Sprintf("upstream pool %q was not found", id))
		}

		for _, route := range cfg.Routes {
			if route.UpstreamPool == id {
				return conflictError(fmt.Sprintf("upstream pool %q is still referenced by route %q", id, route.ID))
			}
		}

		delete(cfg.UpstreamPools, id)
		return nil
	})
	return err
}

func (s *FileStore) namespacePath(namespace string) (string, error) {
	if err := ValidateNamespaceName(namespace); err != nil {
		return "", err
	}

	return filepath.Join(s.appCfg.ProxyConfigDir, namespace+".json"), nil
}

func (s *FileStore) mutateNamespaceConfig(_ context.Context, namespace string, mutate func(cfg *proxyconfig.Config) error) (proxyconfig.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.loadNamespaceFile(namespace)
	if err != nil {
		return proxyconfig.Config{}, err
	}

	cfg := file.config
	if err := mutate(&cfg); err != nil {
		return proxyconfig.Config{}, err
	}

	cfg = normalizeConfig(cfg)
	if validationErrs := cfg.Validate(); len(validationErrs) > 0 {
		return proxyconfig.Config{}, &StoreError{
			StatusCode: http.StatusUnprocessableEntity,
			Code:       "validation_failed",
			Message:    "validation failed",
			Err:        proxyconfig.ValidationErrors(validationErrs),
		}
	}

	if err := writeConfigFileAtomic(file.path, cfg); err != nil {
		return proxyconfig.Config{}, internalError("failed to save namespace config", err)
	}

	return cfg, nil
}

func (s *FileStore) loadNamespaceFile(namespace string) (namespaceFile, error) {
	path, err := s.namespacePath(namespace)
	if err != nil {
		return namespaceFile{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return namespaceFile{
				path:   path,
				config: normalizeConfig(proxyconfig.Config{}),
			}, nil
		}

		return namespaceFile{}, internalError("failed to read namespace config", err)
	}

	loaded, err := proxyconfig.Decode(namespace, path, data)
	if err != nil {
		return namespaceFile{}, internalError("failed to decode namespace config", err)
	}

	return namespaceFile{
		path:    path,
		exists:  true,
		rawData: data,
		config:  normalizeConfig(loaded.Config),
	}, nil
}

func writeConfigFileAtomic(path string, cfg proxyconfig.Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(normalizeConfig(cfg), "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}

	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if err := tmp.Chmod(configFileMode); err != nil {
		_ = tmp.Close()
		return err
	}

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}

	return syncDir(filepath.Dir(path))
}

func syncDir(dir string) error {
	file, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()

	return file.Sync()
}

func cloneRoutes(routes []proxyconfig.RouteConfig) []proxyconfig.RouteConfig {
	cloned := make([]proxyconfig.RouteConfig, 0, len(routes))
	for _, route := range routes {
		cloned = append(cloned, cloneRoute(route))
	}
	return cloned
}

func cloneRoute(route proxyconfig.RouteConfig) proxyconfig.RouteConfig {
	cloned := route
	cloned.Match.Hosts = append([]string(nil), route.Match.Hosts...)
	if route.Match.Path != nil {
		path := *route.Match.Path
		cloned.Match.Path = &path
	}
	return cloned
}

func cloneUpstreamPools(pools map[string]proxyconfig.UpstreamPool) map[string]proxyconfig.UpstreamPool {
	cloned := make(map[string]proxyconfig.UpstreamPool, len(pools))
	for id, pool := range pools {
		cloned[id] = cloneUpstreamPool(pool)
	}
	return cloned
}

func cloneUpstreamPool(pool proxyconfig.UpstreamPool) proxyconfig.UpstreamPool {
	cloned := pool
	cloned.Upstreams = append([]string(nil), pool.Upstreams...)
	if pool.HealthCheck != nil {
		healthCheck := *pool.HealthCheck
		cloned.HealthCheck = &healthCheck
	}
	return cloned
}

func conflictError(message string) *StoreError {
	return &StoreError{
		StatusCode: http.StatusConflict,
		Code:       "resource_conflict",
		Message:    message,
	}
}

func notFoundError(message string) *StoreError {
	return &StoreError{
		StatusCode: http.StatusNotFound,
		Code:       "resource_not_found",
		Message:    message,
	}
}

func internalError(message string, err error) *StoreError {
	return &StoreError{
		StatusCode: http.StatusInternalServerError,
		Code:       "store_error",
		Message:    message,
		Err:        err,
	}
}
