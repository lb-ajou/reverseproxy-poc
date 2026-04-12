package admin

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

	"reverseproxy-poc/internal/proxyconfig"
	appruntime "reverseproxy-poc/internal/runtime"
)

const DefaultNamespace = "default"

var namespacePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type Runtime interface {
	Snapshot() appruntime.Snapshot
	ReloadFromFile(ctx context.Context) error
}

type Service interface {
	ListNamespaces(ctx context.Context) ([]NamespaceView, error)
	CreateNamespace(ctx context.Context, namespace string) (NamespaceView, error)
	DeleteNamespace(ctx context.Context, namespace string) error
	GetNamespaceConfig(ctx context.Context, namespace string) (NamespaceConfigView, error)
	GetNamespaceRoutes(ctx context.Context, namespace string) ([]proxyconfig.RouteConfig, error)
	CreateRoute(ctx context.Context, namespace string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error)
	UpdateRoute(ctx context.Context, namespace, id string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error)
	DeleteRoute(ctx context.Context, namespace, id string) error
	GetNamespaceUpstreamPools(ctx context.Context, namespace string) (map[string]proxyconfig.UpstreamPool, error)
	CreateUpstreamPool(ctx context.Context, namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error)
	UpdateUpstreamPool(ctx context.Context, namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error)
	DeleteUpstreamPool(ctx context.Context, namespace, id string) error
}

type NamespaceConfigView struct {
	Namespace     string                              `json:"namespace"`
	Exists        bool                                `json:"exists"`
	Routes        []proxyconfig.RouteConfig           `json:"routes"`
	UpstreamPools map[string]proxyconfig.UpstreamPool `json:"upstream_pools"`
	AppliedAt     time.Time                           `json:"applied_at,omitempty"`
}

type NamespaceView struct {
	Namespace         string `json:"namespace"`
	Path              string `json:"path"`
	Exists            bool   `json:"exists"`
	RouteCount        int    `json:"route_count"`
	UpstreamPoolCount int    `json:"upstream_pool_count"`
}

type NamespaceListView struct {
	Items            []NamespaceView `json:"items"`
	DefaultNamespace string          `json:"default_namespace"`
}

type APIError struct {
	StatusCode       int                           `json:"-"`
	Message          string                        `json:"message"`
	ValidationErrors []proxyconfig.ValidationError `json:"errors,omitempty"`
	Err              error                         `json:"-"`
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Message
	}
	if e.Message == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
}

func (e *APIError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type service struct {
	runtime Runtime
	mu      sync.Mutex
}

type namespaceFile struct {
	path    string
	exists  bool
	rawData []byte
	config  proxyconfig.Config
}

func New(runtime Runtime) Service {
	return &service{runtime: runtime}
}

func (s *service) ListNamespaces(_ context.Context) ([]NamespaceView, error) {
	dir := s.runtime.Snapshot().AppConfig.ProxyConfigDir
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, &APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to read proxy config directory",
			Err:        err,
		}
	}

	items := make([]NamespaceView, 0, len(entries)+1)
	hasDefault := false

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		loaded, err := proxyconfig.LoadFile(path)
		if err != nil {
			return nil, &APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    "failed to load namespace config",
				Err:        err,
			}
		}

		if loaded.Source == DefaultNamespace {
			hasDefault = true
		}

		items = append(items, NamespaceView{
			Namespace:         loaded.Source,
			Path:              loaded.Path,
			Exists:            true,
			RouteCount:        len(loaded.Config.Routes),
			UpstreamPoolCount: len(loaded.Config.UpstreamPools),
		})
	}

	if !hasDefault {
		items = append(items, NamespaceView{
			Namespace: DefaultNamespace,
			Path:      filepath.Join(dir, DefaultNamespace+".json"),
			Exists:    false,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Namespace < items[j].Namespace
	})

	return items, nil
}

func (s *service) CreateNamespace(ctx context.Context, namespace string) (NamespaceView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.namespacePath(namespace)
	if err != nil {
		return NamespaceView{}, err
	}

	if _, err := os.Stat(path); err == nil {
		return NamespaceView{}, &APIError{
			StatusCode: http.StatusConflict,
			Message:    fmt.Sprintf("namespace %q already exists", namespace),
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return NamespaceView{}, &APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to inspect namespace config",
			Err:        err,
		}
	}

	if err := writeConfigFileAtomic(path, normalizeConfig(proxyconfig.Config{})); err != nil {
		return NamespaceView{}, &APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to create namespace config",
			Err:        err,
		}
	}

	if err := s.runtime.ReloadFromFile(ctx); err != nil {
		restoreErr := os.Remove(path)
		if restoreErr != nil && !errors.Is(restoreErr, os.ErrNotExist) {
			err = errors.Join(err, restoreErr)
		}

		return NamespaceView{}, &APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to reload proxy configuration",
			Err:        err,
		}
	}

	return NamespaceView{
		Namespace: namespace,
		Path:      path,
		Exists:    true,
	}, nil
}

func (s *service) DeleteNamespace(ctx context.Context, namespace string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.loadNamespaceFile(namespace)
	if err != nil {
		return err
	}
	if !file.exists {
		return &APIError{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("namespace %q was not found", namespace),
		}
	}

	if err := os.Remove(file.path); err != nil {
		return &APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to delete namespace config",
			Err:        err,
		}
	}

	if err := s.runtime.ReloadFromFile(ctx); err != nil {
		restoreErr := restoreNamespaceFile(file)
		if restoreErr != nil {
			err = errors.Join(err, restoreErr)
		}

		return &APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to reload proxy configuration",
			Err:        err,
		}
	}

	return nil
}

func (s *service) GetNamespaceConfig(_ context.Context, namespace string) (NamespaceConfigView, error) {
	file, err := s.loadNamespaceFile(namespace)
	if err != nil {
		return NamespaceConfigView{}, err
	}

	return NamespaceConfigView{
		Namespace:     namespace,
		Exists:        file.exists,
		Routes:        cloneRoutes(file.config.Routes),
		UpstreamPools: cloneUpstreamPools(file.config.UpstreamPools),
		AppliedAt:     s.runtime.Snapshot().AppliedAt,
	}, nil
}

func (s *service) GetNamespaceRoutes(ctx context.Context, namespace string) ([]proxyconfig.RouteConfig, error) {
	view, err := s.GetNamespaceConfig(ctx, namespace)
	if err != nil {
		return nil, err
	}

	return view.Routes, nil
}

func (s *service) CreateRoute(ctx context.Context, namespace string, routeCfg proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error) {
	savedRoute := cloneRoute(routeCfg)

	_, err := s.mutateNamespaceConfig(ctx, namespace, func(cfg *proxyconfig.Config) error {
		for _, existing := range cfg.Routes {
			if existing.ID == routeCfg.ID {
				return &APIError{
					StatusCode: http.StatusConflict,
					Message:    fmt.Sprintf("route %q already exists", routeCfg.ID),
				}
			}
		}

		cfg.Routes = append(cfg.Routes, cloneRoute(routeCfg))
		return nil
	})
	if err != nil {
		return proxyconfig.RouteConfig{}, err
	}

	return savedRoute, nil
}

func (s *service) UpdateRoute(ctx context.Context, namespace, id string, routeCfg proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error) {
	if routeCfg.ID != id {
		return proxyconfig.RouteConfig{}, &APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "route id in body must match request path",
		}
	}

	_, err := s.mutateNamespaceConfig(ctx, namespace, func(cfg *proxyconfig.Config) error {
		for index, existing := range cfg.Routes {
			if existing.ID == id {
				cfg.Routes[index] = cloneRoute(routeCfg)
				return nil
			}
		}

		return &APIError{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("route %q was not found", id),
		}
	})
	if err != nil {
		return proxyconfig.RouteConfig{}, err
	}

	return cloneRoute(routeCfg), nil
}

func (s *service) DeleteRoute(ctx context.Context, namespace, id string) error {
	_, err := s.mutateNamespaceConfig(ctx, namespace, func(cfg *proxyconfig.Config) error {
		for index, existing := range cfg.Routes {
			if existing.ID == id {
				cfg.Routes = append(cfg.Routes[:index], cfg.Routes[index+1:]...)
				return nil
			}
		}

		return &APIError{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("route %q was not found", id),
		}
	})

	return err
}

func (s *service) GetNamespaceUpstreamPools(ctx context.Context, namespace string) (map[string]proxyconfig.UpstreamPool, error) {
	view, err := s.GetNamespaceConfig(ctx, namespace)
	if err != nil {
		return nil, err
	}

	return view.UpstreamPools, nil
}

func (s *service) CreateUpstreamPool(ctx context.Context, namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error) {
	savedPool := cloneUpstreamPool(pool)

	_, err := s.mutateNamespaceConfig(ctx, namespace, func(cfg *proxyconfig.Config) error {
		if _, exists := cfg.UpstreamPools[id]; exists {
			return &APIError{
				StatusCode: http.StatusConflict,
				Message:    fmt.Sprintf("upstream pool %q already exists", id),
			}
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

func (s *service) UpdateUpstreamPool(ctx context.Context, namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error) {
	_, err := s.mutateNamespaceConfig(ctx, namespace, func(cfg *proxyconfig.Config) error {
		if _, exists := cfg.UpstreamPools[id]; !exists {
			return &APIError{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("upstream pool %q was not found", id),
			}
		}

		cfg.UpstreamPools[id] = cloneUpstreamPool(pool)
		return nil
	})
	if err != nil {
		return proxyconfig.UpstreamPool{}, err
	}

	return cloneUpstreamPool(pool), nil
}

func (s *service) DeleteUpstreamPool(ctx context.Context, namespace, id string) error {
	_, err := s.mutateNamespaceConfig(ctx, namespace, func(cfg *proxyconfig.Config) error {
		if _, exists := cfg.UpstreamPools[id]; !exists {
			return &APIError{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("upstream pool %q was not found", id),
			}
		}

		for _, routeCfg := range cfg.Routes {
			if routeCfg.UpstreamPool == id {
				return &APIError{
					StatusCode: http.StatusConflict,
					Message:    fmt.Sprintf("upstream pool %q is still referenced by route %q", id, routeCfg.ID),
				}
			}
		}

		delete(cfg.UpstreamPools, id)
		return nil
	})

	return err
}

func (s *service) mutateNamespaceConfig(ctx context.Context, namespace string, mutate func(cfg *proxyconfig.Config) error) (proxyconfig.Config, error) {
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
		return proxyconfig.Config{}, &APIError{
			StatusCode:       http.StatusUnprocessableEntity,
			Message:          "validation failed",
			ValidationErrors: validationErrs,
		}
	}

	if err := writeConfigFileAtomic(file.path, cfg); err != nil {
		return proxyconfig.Config{}, &APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to save namespace config",
			Err:        err,
		}
	}

	if err := s.runtime.ReloadFromFile(ctx); err != nil {
		restoreErr := restoreNamespaceFile(file)
		if restoreErr != nil {
			err = errors.Join(err, restoreErr)
		}

		return proxyconfig.Config{}, &APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to reload proxy configuration",
			Err:        err,
		}
	}

	return cfg, nil
}

func (s *service) loadNamespaceFile(namespace string) (namespaceFile, error) {
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

		return namespaceFile{}, &APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to read namespace config",
			Err:        err,
		}
	}

	loaded, err := proxyconfig.Decode(namespace, path, data)
	if err != nil {
		return namespaceFile{}, &APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to decode namespace config",
			Err:        err,
		}
	}

	return namespaceFile{
		path:    path,
		exists:  true,
		rawData: data,
		config:  normalizeConfig(loaded.Config),
	}, nil
}

func (s *service) namespacePath(namespace string) (string, error) {
	if !namespacePattern.MatchString(namespace) {
		return "", &APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "namespace must contain only letters, numbers, dot, underscore, or hyphen",
		}
	}

	dir := s.runtime.Snapshot().AppConfig.ProxyConfigDir
	return filepath.Join(dir, namespace+".json"), nil
}

func restoreNamespaceFile(file namespaceFile) error {
	if file.exists {
		return os.WriteFile(file.path, file.rawData, 0o644)
	}

	if err := os.Remove(file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
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

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

func normalizeConfig(cfg proxyconfig.Config) proxyconfig.Config {
	if cfg.Routes == nil {
		cfg.Routes = []proxyconfig.RouteConfig{}
	}
	if cfg.UpstreamPools == nil {
		cfg.UpstreamPools = map[string]proxyconfig.UpstreamPool{}
	}
	return cfg
}

func cloneRoutes(routes []proxyconfig.RouteConfig) []proxyconfig.RouteConfig {
	cloned := make([]proxyconfig.RouteConfig, 0, len(routes))
	for _, routeCfg := range routes {
		cloned = append(cloned, cloneRoute(routeCfg))
	}
	return cloned
}

func cloneRoute(routeCfg proxyconfig.RouteConfig) proxyconfig.RouteConfig {
	cloned := routeCfg
	cloned.Match.Hosts = append([]string(nil), routeCfg.Match.Hosts...)
	if routeCfg.Match.Path != nil {
		path := *routeCfg.Match.Path
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
