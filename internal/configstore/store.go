package configstore

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"reverseproxy-poc/internal/proxyconfig"
)

const DefaultNamespace = "default"

type DesiredState struct {
	Namespaces map[string]proxyconfig.Config
	Version    uint64
	AppliedAt  time.Time
}

type NamespaceSummary struct {
	Namespace         string
	Path              string
	Exists            bool
	RouteCount        int
	UpstreamPoolCount int
}

type NamespaceConfig struct {
	Namespace     string
	Exists        bool
	Routes        []proxyconfig.RouteConfig
	UpstreamPools map[string]proxyconfig.UpstreamPool
	AppliedAt     time.Time
}

type Store interface {
	DesiredState(ctx context.Context) (DesiredState, error)
	ListNamespaces(ctx context.Context) ([]NamespaceSummary, error)
	GetNamespaceConfig(ctx context.Context, namespace string) (NamespaceConfig, error)
	CreateNamespace(ctx context.Context, namespace string) (NamespaceSummary, error)
	DeleteNamespace(ctx context.Context, namespace string) error
	CreateRoute(ctx context.Context, namespace string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error)
	UpdateRoute(ctx context.Context, namespace, id string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error)
	DeleteRoute(ctx context.Context, namespace, id string) error
	CreateUpstreamPool(ctx context.Context, namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error)
	UpdateUpstreamPool(ctx context.Context, namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error)
	DeleteUpstreamPool(ctx context.Context, namespace, id string) error
}

type StoreError struct {
	StatusCode    int
	Code          string
	Message       string
	LeaderAddress string
	Err           error
}

func (e *StoreError) Error() string {
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

func (e *StoreError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func NewNotLeaderError(leader string) *StoreError {
	return &StoreError{
		StatusCode:    http.StatusConflict,
		Code:          "not_raft_leader",
		Message:       "configuration writes must be sent to the raft leader",
		LeaderAddress: leader,
	}
}

func IsNotLeader(err error) bool {
	var storeErr *StoreError
	return errors.As(err, &storeErr) && storeErr.Code == "not_raft_leader"
}

func ValidateNamespaceName(namespace string) error {
	if namespacePattern.MatchString(namespace) {
		return nil
	}
	return &StoreError{
		StatusCode: http.StatusBadRequest,
		Code:       "invalid_namespace",
		Message:    "namespace must contain only letters, numbers, dot, underscore, or hyphen",
	}
}

func ValidateIdentifier(value, field string) error {
	if namespacePattern.MatchString(value) {
		return nil
	}
	codeField := strings.ReplaceAll(field, " ", "_")
	return &StoreError{
		StatusCode: http.StatusBadRequest,
		Code:       "invalid_" + codeField,
		Message:    field + " must contain only letters, numbers, dot, underscore, or hyphen",
	}
}
