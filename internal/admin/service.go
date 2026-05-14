package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"reverseproxy-poc/internal/configstore"
	"reverseproxy-poc/internal/proxyconfig"
	appruntime "reverseproxy-poc/internal/runtime"
)

const (
	DefaultNamespace = configstore.DefaultNamespace
	configFileMode   = 0o644
)

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
	store configstore.Store
}

func New(runtime Runtime) Service {
	return NewWithStore(configstore.NewFileStore(runtime.Snapshot().AppConfig))
}

func NewWithStore(store configstore.Store) Service {
	return &service{store: store}
}

func (s *service) ListNamespaces(ctx context.Context) ([]NamespaceView, error) {
	items, err := s.store.ListNamespaces(ctx)
	if err != nil {
		return nil, toAPIError(err)
	}

	views := make([]NamespaceView, 0, len(items))
	for _, item := range items {
		views = append(views, namespaceViewFromStore(item))
	}
	return views, nil
}

func (s *service) CreateNamespace(ctx context.Context, namespace string) (NamespaceView, error) {
	item, err := s.store.CreateNamespace(ctx, namespace)
	if err != nil {
		return NamespaceView{}, toAPIError(err)
	}
	return namespaceViewFromStore(item), nil
}

func (s *service) DeleteNamespace(ctx context.Context, namespace string) error {
	return toAPIError(s.store.DeleteNamespace(ctx, namespace))
}

func (s *service) GetNamespaceConfig(ctx context.Context, namespace string) (NamespaceConfigView, error) {
	item, err := s.store.GetNamespaceConfig(ctx, namespace)
	if err != nil {
		return NamespaceConfigView{}, toAPIError(err)
	}
	return namespaceConfigFromStore(item), nil
}

func (s *service) GetNamespaceRoutes(ctx context.Context, namespace string) ([]proxyconfig.RouteConfig, error) {
	view, err := s.GetNamespaceConfig(ctx, namespace)
	if err != nil {
		return nil, err
	}
	return view.Routes, nil
}

func (s *service) CreateRoute(ctx context.Context, namespace string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error) {
	item, err := s.store.CreateRoute(ctx, namespace, route)
	if err != nil {
		return proxyconfig.RouteConfig{}, toAPIError(err)
	}
	return item, nil
}

func (s *service) UpdateRoute(ctx context.Context, namespace, id string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error) {
	item, err := s.store.UpdateRoute(ctx, namespace, id, route)
	if err != nil {
		return proxyconfig.RouteConfig{}, toAPIError(err)
	}
	return item, nil
}

func (s *service) DeleteRoute(ctx context.Context, namespace, id string) error {
	return toAPIError(s.store.DeleteRoute(ctx, namespace, id))
}

func (s *service) GetNamespaceUpstreamPools(ctx context.Context, namespace string) (map[string]proxyconfig.UpstreamPool, error) {
	view, err := s.GetNamespaceConfig(ctx, namespace)
	if err != nil {
		return nil, err
	}
	return view.UpstreamPools, nil
}

func (s *service) CreateUpstreamPool(ctx context.Context, namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error) {
	item, err := s.store.CreateUpstreamPool(ctx, namespace, id, pool)
	if err != nil {
		return proxyconfig.UpstreamPool{}, toAPIError(err)
	}
	return item, nil
}

func (s *service) UpdateUpstreamPool(ctx context.Context, namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error) {
	item, err := s.store.UpdateUpstreamPool(ctx, namespace, id, pool)
	if err != nil {
		return proxyconfig.UpstreamPool{}, toAPIError(err)
	}
	return item, nil
}

func (s *service) DeleteUpstreamPool(ctx context.Context, namespace, id string) error {
	return toAPIError(s.store.DeleteUpstreamPool(ctx, namespace, id))
}

func namespaceViewFromStore(item configstore.NamespaceSummary) NamespaceView {
	return NamespaceView{
		Namespace:         item.Namespace,
		Path:              item.Path,
		Exists:            item.Exists,
		RouteCount:        item.RouteCount,
		UpstreamPoolCount: item.UpstreamPoolCount,
	}
}

func namespaceConfigFromStore(item configstore.NamespaceConfig) NamespaceConfigView {
	return NamespaceConfigView{
		Namespace:     item.Namespace,
		Exists:        item.Exists,
		Routes:        item.Routes,
		UpstreamPools: item.UpstreamPools,
		AppliedAt:     item.AppliedAt,
	}
}

func toAPIError(err error) error {
	if err == nil {
		return nil
	}

	var storeErr *configstore.StoreError
	if errors.As(err, &storeErr) {
		apiErr := &APIError{
			StatusCode: storeErr.StatusCode,
			Message:    storeErr.Message,
			Err:        storeErr.Err,
		}

		var validationErrs proxyconfig.ValidationErrors
		if errors.As(storeErr.Err, &validationErrs) {
			apiErr.ValidationErrors = validationErrs
		}

		return apiErr
	}

	return err
}
