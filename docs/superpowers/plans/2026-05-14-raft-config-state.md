# Raft Config State Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add HA-mode configuration replication with HashiCorp Raft while keeping request-time proxy state local to each node.

**Architecture:** Introduce a desired-configuration store boundary, keep the existing file-backed behavior behind `FileConfigStore`, and add a Raft-backed store that commits namespace mutations through a replicated FSM. Each committed desired-config state is projected locally into the existing `runtime.Snapshot`; health state, round-robin cursor, sticky cookie behavior, and least-connection counters remain node-local.

**Tech Stack:** Go 1.24, `github.com/hashicorp/raft`, `github.com/hashicorp/raft-boltdb/v2`, existing `net/http`, existing `internal/proxyconfig`, `internal/route`, `internal/upstream`, and `internal/runtime`.

---

## Scope Check

This plan implements the approved design in one sequential stream because each piece builds directly on the previous one:

1. Extract deterministic desired-config projection.
2. Put current file persistence behind a `ConfigStore`.
3. Rewire admin service to use the store.
4. Add Raft FSM and bootstrap rules.
5. Add Raft store and application wiring.
6. Add dashboard/API error behavior and integration tests.

The plan intentionally excludes cluster-wide health consensus and request-time state replication.

## File Structure

- Create `internal/configstore/store.go`: shared store interface, namespace summary types, mutation error type.
- Create `internal/configstore/project.go`: deterministic conversion from namespace configs to `runtime.Snapshot`.
- Create `internal/configstore/file.go`: file-backed implementation that preserves current single-node behavior.
- Create `internal/configstore/file_test.go`: tests for existing file semantics through the new store.
- Modify `internal/admin/service.go`: depend on `configstore.Store` instead of direct file mutation in the service.
- Modify `internal/admin/service_test.go`: keep existing behavioral tests and add store-call error coverage.
- Create `internal/raftconfig/command.go`: Raft command schema and validation helpers.
- Create `internal/raftconfig/fsm.go`: HashiCorp Raft FSM implementation for namespace desired config.
- Create `internal/raftconfig/fsm_snapshot.go`: FSM snapshot and restore support.
- Create `internal/raftconfig/fsm_test.go`: command, validation, snapshot, and restore tests.
- Create `internal/raftconfig/node.go`: HashiCorp Raft node construction, storage, bootstrap, and join helpers.
- Create `internal/raftconfig/store.go`: `ConfigStore` implementation backed by Raft apply.
- Create `internal/raftconfig/store_test.go`: leader/not-leader and command application tests with in-memory transports.
- Modify `internal/config/config.go`: add node-local HA settings to `AppConfig`.
- Modify `internal/config/validate.go`: validate HA settings.
- Modify `internal/config/config_test.go`: config default and validation tests.
- Modify `internal/app/app.go`: choose file or Raft store and subscribe to projected snapshots.
- Modify `internal/app/reload.go`: keep reload file-only for `FileConfigStore`; disable implicit proxy JSON reload in HA mode.
- Modify `internal/dashboard/config_api.go`: map not-leader errors to `409 Conflict` with `leader_address`.
- Modify `docs/api/dashboard-api.ko.md`: document HA-mode write errors and local-vs-cluster state.
- Create `docs/architecture/raft-config-state.ko.md`: operator-facing behavior notes.

## Task 1: Desired Config Projection

**Files:**
- Create: `internal/configstore/store.go`
- Create: `internal/configstore/project.go`
- Test: `internal/configstore/project_test.go`

- [ ] **Step 1: Write failing projection tests**

Create `internal/configstore/project_test.go`:

```go
package configstore

import (
	"testing"

	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/proxyconfig"
)

func TestProjectSnapshot_BuildsRuntimeFromNamespaceMap(t *testing.T) {
	appCfg := config.AppConfig{
		ProxyListenAddr:     ":8080",
		DashboardListenAddr: ":9090",
		ProxyConfigDir:      "configs/proxy",
	}
	state := DesiredState{
		Namespaces: map[string]proxyconfig.Config{
			"default": {
				Routes: []proxyconfig.RouteConfig{{
					ID:      "r-api",
					Enabled: true,
					Match: proxyconfig.RouteMatchConfig{
						Hosts: []string{"api.example.com"},
					},
					UpstreamPool: "pool-api",
				}},
				UpstreamPools: map[string]proxyconfig.UpstreamPool{
					"pool-api": {Upstreams: []string{"10.0.0.11:8080"}},
				},
			},
		},
	}

	snapshot, err := ProjectSnapshot(appCfg, state)
	if err != nil {
		t.Fatalf("ProjectSnapshot() error = %v", err)
	}
	if got, want := len(snapshot.ProxyConfigs), 1; got != want {
		t.Fatalf("len(snapshot.ProxyConfigs) = %d, want %d", got, want)
	}
	if got, want := snapshot.ProxyConfigs[0].Source, "default"; got != want {
		t.Fatalf("snapshot.ProxyConfigs[0].Source = %q, want %q", got, want)
	}
	if got, want := len(snapshot.RouteTable), 1; got != want {
		t.Fatalf("len(snapshot.RouteTable) = %d, want %d", got, want)
	}
	if got, want := snapshot.RouteTable[0].GlobalID, "default:r-api"; got != want {
		t.Fatalf("snapshot.RouteTable[0].GlobalID = %q, want %q", got, want)
	}
	if _, ok := snapshot.Upstreams.Get("default:pool-api"); !ok {
		t.Fatal("snapshot.Upstreams.Get(default:pool-api) returned no pool")
	}
}

func TestProjectSnapshot_RejectsInvalidDesiredConfig(t *testing.T) {
	_, err := ProjectSnapshot(config.AppConfig{}, DesiredState{
		Namespaces: map[string]proxyconfig.Config{
			"default": {
				Routes: []proxyconfig.RouteConfig{{
					ID:           "r-api",
					Enabled:      true,
					Match:        proxyconfig.RouteMatchConfig{Hosts: []string{"api.example.com"}},
					UpstreamPool: "missing",
				}},
				UpstreamPools: map[string]proxyconfig.UpstreamPool{},
			},
		},
	})
	if err == nil {
		t.Fatal("ProjectSnapshot() error = nil, want validation error")
	}
}
```

- [ ] **Step 2: Run projection tests and verify failure**

Run: `go test ./internal/configstore`

Expected: FAIL with `package reverseproxy-poc/internal/configstore is not in std` or undefined `DesiredState` / `ProjectSnapshot`.

- [ ] **Step 3: Add store types**

Create `internal/configstore/store.go`:

```go
package configstore

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"reverseproxy-poc/internal/proxyconfig"
)

const DefaultNamespace = "default"

type DesiredState struct {
	Namespaces map[string]proxyconfig.Config
	Version    uint64
	AppliedAt time.Time
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
		StatusCode:     http.StatusConflict,
		Code:           "not_raft_leader",
		Message:        "configuration writes must be sent to the raft leader",
		LeaderAddress: leader,
	}
}

func IsNotLeader(err error) bool {
	var storeErr *StoreError
	return errors.As(err, &storeErr) && storeErr.Code == "not_raft_leader"
}
```

- [ ] **Step 4: Add projection implementation**

Create `internal/configstore/project.go`:

```go
package configstore

import (
	"fmt"
	"path/filepath"
	"sort"

	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/proxyconfig"
	"reverseproxy-poc/internal/route"
	appruntime "reverseproxy-poc/internal/runtime"
	"reverseproxy-poc/internal/upstream"
)

func ProjectSnapshot(appCfg config.AppConfig, desired DesiredState) (appruntime.Snapshot, error) {
	loaded, err := LoadedConfigs(appCfg.ProxyConfigDir, desired)
	if err != nil {
		return appruntime.Snapshot{}, err
	}
	routes, err := route.BuildTable(loaded)
	if err != nil {
		return appruntime.Snapshot{}, fmt.Errorf("build route table: %w", err)
	}
	upstreams, err := upstream.BuildRegistry(loaded)
	if err != nil {
		return appruntime.Snapshot{}, fmt.Errorf("build upstream registry: %w", err)
	}
	snapshot := appruntime.NewSnapshot(appCfg, loaded, routes, upstreams)
	if !desired.AppliedAt.IsZero() {
		snapshot.AppliedAt = desired.AppliedAt
	}
	return snapshot, nil
}

func LoadedConfigs(dir string, desired DesiredState) ([]proxyconfig.LoadedConfig, error) {
	namespaces := sortedNamespaces(desired.Namespaces)
	loaded := make([]proxyconfig.LoadedConfig, 0, len(namespaces))
	for _, namespace := range namespaces {
		cfg := normalizeConfig(desired.Namespaces[namespace])
		if errs := cfg.Validate(); len(errs) > 0 {
			return nil, proxyconfig.ValidationErrors(errs)
		}
		loaded = append(loaded, proxyconfig.LoadedConfig{
			Source: namespace,
			Path:   filepath.Join(dir, namespace+".json"),
			Config: cfg,
		})
	}
	return loaded, nil
}

func sortedNamespaces(configs map[string]proxyconfig.Config) []string {
	namespaces := make([]string, 0, len(configs))
	for namespace := range configs {
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)
	return namespaces
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
```

- [ ] **Step 5: Run projection tests and commit**

Run: `go test ./internal/configstore`

Expected: PASS.

Commit:

```bash
git add internal/configstore/store.go internal/configstore/project.go internal/configstore/project_test.go
git commit -m "feat(config): add desired state projection"
```

## Task 2: FileConfigStore Compatibility Layer

**Files:**
- Create: `internal/configstore/file.go`
- Test: `internal/configstore/file_test.go`

- [ ] **Step 1: Write failing file store tests**

Create `internal/configstore/file_test.go`:

```go
package configstore

import (
	"context"
	"path/filepath"
	"testing"

	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/proxyconfig"
)

func TestFileStore_CreatePoolAndRoutePersistsDesiredState(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(config.AppConfig{
		ProxyListenAddr:     ":8080",
		DashboardListenAddr: ":9090",
		ProxyConfigDir:      dir,
	})

	_, err := store.CreateUpstreamPool(context.Background(), DefaultNamespace, "pool-api", proxyconfig.UpstreamPool{
		Upstreams: []string{"10.0.0.11:8080"},
	})
	if err != nil {
		t.Fatalf("CreateUpstreamPool() error = %v", err)
	}
	_, err = store.CreateRoute(context.Background(), DefaultNamespace, proxyconfig.RouteConfig{
		ID:      "r-api",
		Enabled: true,
		Match: proxyconfig.RouteMatchConfig{
			Hosts: []string{"api.example.com"},
		},
		UpstreamPool: "pool-api",
	})
	if err != nil {
		t.Fatalf("CreateRoute() error = %v", err)
	}

	loaded, err := proxyconfig.LoadFile(filepath.Join(dir, "default.json"))
	if err != nil {
		t.Fatalf("LoadFile(default.json) error = %v", err)
	}
	if got, want := len(loaded.Config.Routes), 1; got != want {
		t.Fatalf("len(loaded.Config.Routes) = %d, want %d", got, want)
	}
	if _, ok := loaded.Config.UpstreamPools["pool-api"]; !ok {
		t.Fatal("loaded.Config.UpstreamPools[pool-api] missing")
	}
}

func TestFileStore_DesiredStateLoadsSortedNamespaces(t *testing.T) {
	dir := t.TempDir()
	writeConfigFileForTest(t, filepath.Join(dir, "b.json"), `{"routes":[],"upstream_pools":{}}`)
	writeConfigFileForTest(t, filepath.Join(dir, "a.json"), `{"routes":[],"upstream_pools":{}}`)
	store := NewFileStore(config.AppConfig{ProxyConfigDir: dir})

	state, err := store.DesiredState(context.Background())
	if err != nil {
		t.Fatalf("DesiredState() error = %v", err)
	}
	loaded, err := LoadedConfigs(dir, state)
	if err != nil {
		t.Fatalf("LoadedConfigs() error = %v", err)
	}
	if got, want := loaded[0].Source, "a"; got != want {
		t.Fatalf("loaded[0].Source = %q, want %q", got, want)
	}
	if got, want := loaded[1].Source, "b"; got != want {
		t.Fatalf("loaded[1].Source = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run file store tests and verify failure**

Run: `go test ./internal/configstore`

Expected: FAIL with undefined `NewFileStore`.

- [ ] **Step 3: Implement file store**

Create `internal/configstore/file.go` with the file persistence behavior currently implemented by `internal/admin/service.go`. Use this public surface:

```go
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

func NewFileStore(appCfg config.AppConfig) *FileStore {
	return &FileStore{appCfg: appCfg}
}

func (s *FileStore) DesiredState(_ context.Context) (DesiredState, error) {
	loaded, err := proxyconfig.LoadDir(s.appCfg.ProxyConfigDir)
	if err != nil {
		return DesiredState{}, err
	}
	namespaces := make(map[string]proxyconfig.Config, len(loaded))
	for _, item := range loaded {
		namespaces[item.Source] = normalizeConfig(item.Config)
	}
	return DesiredState{Namespaces: namespaces, AppliedAt: time.Now()}, nil
}

func (s *FileStore) namespacePath(namespace string) (string, error) {
	if !namespacePattern.MatchString(namespace) {
		return "", &StoreError{StatusCode: http.StatusBadRequest, Code: "invalid_namespace", Message: "namespace must contain only letters, numbers, dot, underscore, or hyphen"}
	}
	return filepath.Join(s.appCfg.ProxyConfigDir, namespace+".json"), nil
}

func (s *FileStore) ListNamespaces(ctx context.Context) ([]NamespaceSummary, error) {
	state, err := s.DesiredState(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]NamespaceSummary, 0, len(state.Namespaces)+1)
	hasDefault := false
	for namespace, cfg := range state.Namespaces {
		if namespace == DefaultNamespace {
			hasDefault = true
		}
		items = append(items, NamespaceSummary{
			Namespace:         namespace,
			Path:              filepath.Join(s.appCfg.ProxyConfigDir, namespace+".json"),
			Exists:            true,
			RouteCount:        len(cfg.Routes),
			UpstreamPoolCount: len(cfg.UpstreamPools),
		})
	}
	if !hasDefault {
		items = append(items, NamespaceSummary{Namespace: DefaultNamespace, Path: filepath.Join(s.appCfg.ProxyConfigDir, DefaultNamespace+".json")})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Namespace < items[j].Namespace })
	return items, nil
}
```

Add these methods to `internal/configstore/file.go` with the listed behavior:

```go
func (s *FileStore) CreateNamespace(ctx context.Context, namespace string) (NamespaceSummary, error)
func (s *FileStore) DeleteNamespace(ctx context.Context, namespace string) error
func (s *FileStore) GetNamespaceConfig(ctx context.Context, namespace string) (NamespaceConfig, error)
func (s *FileStore) CreateRoute(ctx context.Context, namespace string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error)
func (s *FileStore) UpdateRoute(ctx context.Context, namespace, id string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error)
func (s *FileStore) DeleteRoute(ctx context.Context, namespace, id string) error
func (s *FileStore) CreateUpstreamPool(ctx context.Context, namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error)
func (s *FileStore) UpdateUpstreamPool(ctx context.Context, namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error)
func (s *FileStore) DeleteUpstreamPool(ctx context.Context, namespace, id string) error
```

Method behavior:

- `CreateNamespace`: lock `s.mu`, reject an existing namespace with `409`, write an empty normalized config to `<proxyConfigDir>/<namespace>.json`, and return `NamespaceSummary{Namespace: namespace, Path: path, Exists: true}`.
- `DeleteNamespace`: lock `s.mu`, load the namespace file, return `404` when it does not exist, remove the file, and return nil.
- `GetNamespaceConfig`: read the namespace file, return an empty normalized config with `Exists: false` when the file does not exist, and set `AppliedAt` to `time.Now()`.
- `CreateRoute`: lock `s.mu`, load or initialize the namespace config, reject duplicate route IDs with `409`, append the route, validate the whole config, and atomically write the file.
- `UpdateRoute`: lock `s.mu`, require URL ID and body ID to match, replace the matching route, return `404` if missing, validate the whole config, and atomically write the file.
- `DeleteRoute`: lock `s.mu`, remove the matching route, return `404` if missing, validate the whole config, and atomically write the file.
- `CreateUpstreamPool`: lock `s.mu`, reject duplicate pool IDs with `409`, add the pool, validate the whole config, and atomically write the file.
- `UpdateUpstreamPool`: lock `s.mu`, replace an existing pool, return `404` if missing, validate the whole config, and atomically write the file.
- `DeleteUpstreamPool`: lock `s.mu`, reject missing pool IDs with `404`, reject pools still referenced by any route with `409`, delete the pool, validate the whole config, and atomically write the file.

Use this conflict error shape:

```go
return &StoreError{StatusCode: http.StatusConflict, Code: "resource_conflict", Message: fmt.Sprintf("route %q already exists", route.ID)}
```

- [ ] **Step 4: Add test helper**

Append this helper to `internal/configstore/file_test.go`:

```go
func writeConfigFileForTest(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
}
```

Add `os` to the test imports.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/configstore`

Expected: PASS.

Commit:

```bash
git add internal/configstore/file.go internal/configstore/file_test.go
git commit -m "feat(config): add file config store"
```

## Task 3: Admin Service Store Refactor

**Files:**
- Modify: `internal/admin/service.go`
- Modify: `internal/admin/service_test.go`
- Test: `internal/admin/service_test.go`

- [ ] **Step 1: Write failing admin constructor test**

Add this test to `internal/admin/service_test.go`:

```go
func TestNewWithStore_UsesConfigStore(t *testing.T) {
	store := &stubConfigStore{
		namespaces: []configstore.NamespaceSummary{{
			Namespace:         "default",
			Path:              "configs/proxy/default.json",
			Exists:            true,
			RouteCount:        1,
			UpstreamPoolCount: 1,
		}},
	}
	service := NewWithStore(store)

	items, err := service.ListNamespaces(context.Background())
	if err != nil {
		t.Fatalf("ListNamespaces() error = %v", err)
	}
	if got, want := len(items), 1; got != want {
		t.Fatalf("len(items) = %d, want %d", got, want)
	}
	if got, want := items[0].Namespace, "default"; got != want {
		t.Fatalf("items[0].Namespace = %q, want %q", got, want)
	}
	if !store.listCalled {
		t.Fatal("store.ListNamespaces was not called")
	}
}
```

Add the stub in the same test file:

```go
type stubConfigStore struct {
	listCalled bool
	namespaces []configstore.NamespaceSummary
}

func (s *stubConfigStore) DesiredState(context.Context) (configstore.DesiredState, error) {
	return configstore.DesiredState{}, nil
}

func (s *stubConfigStore) ListNamespaces(context.Context) ([]configstore.NamespaceSummary, error) {
	s.listCalled = true
	return s.namespaces, nil
}
```

Add no-op methods required by `configstore.Store` with `t.Fatal`-free deterministic returns:

```go
func (s *stubConfigStore) GetNamespaceConfig(context.Context, string) (configstore.NamespaceConfig, error) {
	return configstore.NamespaceConfig{}, nil
}
func (s *stubConfigStore) CreateNamespace(context.Context, string) (configstore.NamespaceSummary, error) {
	return configstore.NamespaceSummary{}, nil
}
func (s *stubConfigStore) DeleteNamespace(context.Context, string) error { return nil }
func (s *stubConfigStore) CreateRoute(context.Context, string, proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error) {
	return proxyconfig.RouteConfig{}, nil
}
func (s *stubConfigStore) UpdateRoute(context.Context, string, string, proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error) {
	return proxyconfig.RouteConfig{}, nil
}
func (s *stubConfigStore) DeleteRoute(context.Context, string, string) error { return nil }
func (s *stubConfigStore) CreateUpstreamPool(context.Context, string, string, proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error) {
	return proxyconfig.UpstreamPool{}, nil
}
func (s *stubConfigStore) UpdateUpstreamPool(context.Context, string, string, proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error) {
	return proxyconfig.UpstreamPool{}, nil
}
func (s *stubConfigStore) DeleteUpstreamPool(context.Context, string, string) error { return nil }
```

- [ ] **Step 2: Run admin test and verify failure**

Run: `go test ./internal/admin -run TestNewWithStore_UsesConfigStore`

Expected: FAIL with undefined `configstore` import or `NewWithStore`.

- [ ] **Step 3: Refactor admin service constructor**

Modify `internal/admin/service.go`:

```go
import "reverseproxy-poc/internal/configstore"

type service struct {
	store configstore.Store
}

func New(runtime Runtime) Service {
	return NewWithStore(configstore.NewFileStore(runtime.Snapshot().AppConfig))
}

func NewWithStore(store configstore.Store) Service {
	return &service{store: store}
}
```

Convert `ListNamespaces` to delegate:

```go
func (s *service) ListNamespaces(ctx context.Context) ([]NamespaceView, error) {
	items, err := s.store.ListNamespaces(ctx)
	if err != nil {
		return nil, toAPIError(err)
	}
	views := make([]NamespaceView, 0, len(items))
	for _, item := range items {
		views = append(views, NamespaceView{
			Namespace:         item.Namespace,
			Path:              item.Path,
			Exists:            item.Exists,
			RouteCount:        item.RouteCount,
			UpstreamPoolCount: item.UpstreamPoolCount,
		})
	}
	return views, nil
}
```

Add error conversion:

```go
func toAPIError(err error) error {
	if err == nil {
		return nil
	}
	var storeErr *configstore.StoreError
	if errors.As(err, &storeErr) {
		return &APIError{
			StatusCode: storeErr.StatusCode,
			Message:    storeErr.Message,
			Err:        storeErr.Err,
		}
	}
	return err
}
```

- [ ] **Step 4: Delegate remaining admin methods**

Replace direct file mutation methods with store calls:

```go
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
```

Use these converters:

```go
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
```

Delete the now-unused file helpers from `internal/admin/service.go` after all calls delegate to the store.

- [ ] **Step 5: Run admin and dashboard tests**

Run: `go test ./internal/admin ./internal/dashboard`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/admin/service.go internal/admin/service_test.go
git commit -m "refactor(admin): use config store"
```

## Task 4: Raft FSM Commands

**Files:**
- Create: `internal/raftconfig/command.go`
- Create: `internal/raftconfig/fsm.go`
- Create: `internal/raftconfig/fsm_snapshot.go`
- Test: `internal/raftconfig/fsm_test.go`

- [ ] **Step 1: Add dependencies**

Run:

```bash
go get github.com/hashicorp/raft@latest
```

Expected: `go.mod` gains `github.com/hashicorp/raft`.

- [ ] **Step 2: Write failing FSM tests**

Create `internal/raftconfig/fsm_test.go`:

```go
package raftconfig

import (
	"bytes"
	"io"
	"testing"

	"github.com/hashicorp/raft"

	"reverseproxy-poc/internal/configstore"
	"reverseproxy-poc/internal/proxyconfig"
)

func TestFSMApplyCreatePoolAndRoute(t *testing.T) {
	fsm := NewFSM()
	applyCommand(t, fsm, Command{
		Type:      CommandCreateUpstreamPool,
		Namespace: configstore.DefaultNamespace,
		PoolID:    "pool-api",
		Pool:      proxyconfig.UpstreamPool{Upstreams: []string{"10.0.0.11:8080"}},
	})
	applyCommand(t, fsm, Command{
		Type:      CommandCreateRoute,
		Namespace: configstore.DefaultNamespace,
		Route: proxyconfig.RouteConfig{
			ID:           "r-api",
			Enabled:      true,
			Match:        proxyconfig.RouteMatchConfig{Hosts: []string{"api.example.com"}},
			UpstreamPool: "pool-api",
		},
	})

	state := fsm.DesiredState()
	cfg := state.Namespaces[configstore.DefaultNamespace]
	if got, want := len(cfg.Routes), 1; got != want {
		t.Fatalf("len(cfg.Routes) = %d, want %d", got, want)
	}
	if _, ok := cfg.UpstreamPools["pool-api"]; !ok {
		t.Fatal("cfg.UpstreamPools[pool-api] missing")
	}
}

func TestFSMApplyInvalidCommandLeavesStateUnchanged(t *testing.T) {
	fsm := NewFSM()
	resp := applyCommand(t, fsm, Command{
		Type:      CommandCreateRoute,
		Namespace: configstore.DefaultNamespace,
		Route: proxyconfig.RouteConfig{
			ID:           "r-api",
			Enabled:      true,
			Match:        proxyconfig.RouteMatchConfig{Hosts: []string{"api.example.com"}},
			UpstreamPool: "missing",
		},
	})
	if resp.Error == "" {
		t.Fatal("response error is empty, want validation error")
	}
	if got := len(fsm.DesiredState().Namespaces); got != 0 {
		t.Fatalf("len(fsm.DesiredState().Namespaces) = %d, want 0", got)
	}
}

func TestFSMSnapshotRestoreRoundTrip(t *testing.T) {
	fsm := NewFSM()
	applyCommand(t, fsm, Command{
		Type:      CommandCreateNamespace,
		Namespace: "admin",
	})
	snapshot, err := fsm.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	var buf bytes.Buffer
	if err := snapshot.Persist(&memorySink{Buffer: &buf}); err != nil {
		t.Fatalf("Persist() error = %v", err)
	}

	restored := NewFSM()
	if err := restored.Restore(io.NopCloser(bytes.NewReader(buf.Bytes()))); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if _, ok := restored.DesiredState().Namespaces["admin"]; !ok {
		t.Fatal("restored namespace admin missing")
	}
}

func applyCommand(t *testing.T, fsm *FSM, cmd Command) ApplyResponse {
	t.Helper()
	data, err := EncodeCommand(cmd)
	if err != nil {
		t.Fatalf("EncodeCommand() error = %v", err)
	}
	resp, ok := fsm.Apply(&raft.Log{Data: data}).(ApplyResponse)
	if !ok {
		t.Fatalf("Apply() response type = %T, want ApplyResponse", resp)
	}
	return resp
}

type memorySink struct {
	*bytes.Buffer
}

func (s *memorySink) ID() string { return "memory" }
func (s *memorySink) Close() error { return nil }
func (s *memorySink) Cancel() error { return nil }
```

- [ ] **Step 3: Run FSM tests and verify failure**

Run: `go test ./internal/raftconfig`

Expected: FAIL with undefined package or undefined `NewFSM`.

- [ ] **Step 4: Implement command schema**

Create `internal/raftconfig/command.go`:

```go
package raftconfig

import (
	"encoding/json"

	"reverseproxy-poc/internal/proxyconfig"
)

type CommandType string

const (
	CommandCreateNamespace     CommandType = "create_namespace"
	CommandDeleteNamespace     CommandType = "delete_namespace"
	CommandCreateRoute         CommandType = "create_route"
	CommandUpdateRoute         CommandType = "update_route"
	CommandDeleteRoute         CommandType = "delete_route"
	CommandCreateUpstreamPool  CommandType = "create_upstream_pool"
	CommandUpdateUpstreamPool  CommandType = "update_upstream_pool"
	CommandDeleteUpstreamPool  CommandType = "delete_upstream_pool"
	CommandImportJSONConfig    CommandType = "import_json_config"
)

type Command struct {
	Type      CommandType                     `json:"type"`
	Namespace string                          `json:"namespace,omitempty"`
	RouteID   string                          `json:"route_id,omitempty"`
	PoolID    string                          `json:"pool_id,omitempty"`
	Route     proxyconfig.RouteConfig         `json:"route,omitempty"`
	Pool      proxyconfig.UpstreamPool        `json:"pool,omitempty"`
	Import    map[string]proxyconfig.Config   `json:"import,omitempty"`
}

type ApplyResponse struct {
	Error string `json:"error,omitempty"`
}

func EncodeCommand(cmd Command) ([]byte, error) {
	return json.Marshal(cmd)
}

func DecodeCommand(data []byte) (Command, error) {
	var cmd Command
	if err := json.Unmarshal(data, &cmd); err != nil {
		return Command{}, err
	}
	return cmd, nil
}
```

- [ ] **Step 5: Implement FSM and dry-run projection**

Create `internal/raftconfig/fsm.go` with:

```go
package raftconfig

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/hashicorp/raft"

	"reverseproxy-poc/internal/config"
	"reverseproxy-poc/internal/configstore"
	"reverseproxy-poc/internal/proxyconfig"
)

type FSM struct {
	mu       sync.RWMutex
	state    configstore.DesiredState
	appCfg   config.AppConfig
	onApply  func(configstore.DesiredState)
}

func NewFSM() *FSM {
	return NewFSMWithConfig(config.AppConfig{}, nil)
}

func NewFSMWithConfig(appCfg config.AppConfig, onApply func(configstore.DesiredState)) *FSM {
	return &FSM{
		appCfg:  appCfg,
		onApply: onApply,
		state: configstore.DesiredState{
			Namespaces: map[string]proxyconfig.Config{},
			AppliedAt:  time.Now(),
		},
	}
}

func (f *FSM) Apply(log *raft.Log) interface{} {
	cmd, err := DecodeCommand(log.Data)
	if err != nil {
		return ApplyResponse{Error: err.Error()}
	}
	next, err := f.applyCommand(cmd)
	if err != nil {
		return ApplyResponse{Error: err.Error()}
	}
	if _, err := configstore.ProjectSnapshot(f.appCfg, next); err != nil {
		return ApplyResponse{Error: err.Error()}
	}
	f.mu.Lock()
	next.Version = log.Index
	next.AppliedAt = time.Now()
	f.state = next
	f.mu.Unlock()
	if f.onApply != nil {
		f.onApply(next)
	}
	return ApplyResponse{}
}

func (f *FSM) DesiredState() configstore.DesiredState {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return cloneDesiredState(f.state)
}

func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	return newFSMSnapshot(f.DesiredState()), nil
}

func (f *FSM) Restore(reader io.ReadCloser) error {
	defer reader.Close()
	state, err := decodeSnapshot(reader)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.state = state
	f.mu.Unlock()
	if f.onApply != nil {
		f.onApply(state)
	}
	return nil
}

func (f *FSM) applyCommand(cmd Command) (configstore.DesiredState, error) {
	next := f.DesiredState()
	if next.Namespaces == nil {
		next.Namespaces = map[string]proxyconfig.Config{}
	}
	switch cmd.Type {
	case CommandCreateNamespace:
		if _, exists := next.Namespaces[cmd.Namespace]; exists {
			return configstore.DesiredState{}, fmt.Errorf("namespace %q already exists", cmd.Namespace)
		}
		next.Namespaces[cmd.Namespace] = proxyconfig.Config{Routes: []proxyconfig.RouteConfig{}, UpstreamPools: map[string]proxyconfig.UpstreamPool{}}
	case CommandCreateUpstreamPool:
		cfg := ensureNamespace(next.Namespaces, cmd.Namespace)
		if _, exists := cfg.UpstreamPools[cmd.PoolID]; exists {
			return configstore.DesiredState{}, fmt.Errorf("upstream pool %q already exists", cmd.PoolID)
		}
		cfg.UpstreamPools[cmd.PoolID] = cmd.Pool
		next.Namespaces[cmd.Namespace] = cfg
	case CommandCreateRoute:
		cfg := ensureNamespace(next.Namespaces, cmd.Namespace)
		for _, existing := range cfg.Routes {
			if existing.ID == cmd.Route.ID {
				return configstore.DesiredState{}, fmt.Errorf("route %q already exists", cmd.Route.ID)
			}
		}
		cfg.Routes = append(cfg.Routes, cmd.Route)
		next.Namespaces[cmd.Namespace] = cfg
	default:
		return configstore.DesiredState{}, fmt.Errorf("unsupported command type %q", cmd.Type)
	}
	if errs := validateDesiredState(next); len(errs) > 0 {
		return configstore.DesiredState{}, proxyconfig.ValidationErrors(errs)
	}
	return next, nil
}
```

Extend the `switch cmd.Type` in `applyCommand` with these cases before committing this task:

```go
case CommandDeleteNamespace:
	if _, exists := next.Namespaces[cmd.Namespace]; !exists {
		return configstore.DesiredState{}, fmt.Errorf("namespace %q was not found", cmd.Namespace)
	}
	delete(next.Namespaces, cmd.Namespace)
case CommandUpdateRoute:
	cfg := ensureNamespace(next.Namespaces, cmd.Namespace)
	replaced := false
	for i, existing := range cfg.Routes {
		if existing.ID == cmd.RouteID {
			if cmd.Route.ID != cmd.RouteID {
				return configstore.DesiredState{}, fmt.Errorf("route id in body must match command route id")
			}
			cfg.Routes[i] = cmd.Route
			replaced = true
			break
		}
	}
	if !replaced {
		return configstore.DesiredState{}, fmt.Errorf("route %q was not found", cmd.RouteID)
	}
	next.Namespaces[cmd.Namespace] = cfg
case CommandDeleteRoute:
	cfg := ensureNamespace(next.Namespaces, cmd.Namespace)
	deleted := false
	for i, existing := range cfg.Routes {
		if existing.ID == cmd.RouteID {
			cfg.Routes = append(cfg.Routes[:i], cfg.Routes[i+1:]...)
			deleted = true
			break
		}
	}
	if !deleted {
		return configstore.DesiredState{}, fmt.Errorf("route %q was not found", cmd.RouteID)
	}
	next.Namespaces[cmd.Namespace] = cfg
case CommandUpdateUpstreamPool:
	cfg := ensureNamespace(next.Namespaces, cmd.Namespace)
	if _, exists := cfg.UpstreamPools[cmd.PoolID]; !exists {
		return configstore.DesiredState{}, fmt.Errorf("upstream pool %q was not found", cmd.PoolID)
	}
	cfg.UpstreamPools[cmd.PoolID] = cmd.Pool
	next.Namespaces[cmd.Namespace] = cfg
case CommandDeleteUpstreamPool:
	cfg := ensureNamespace(next.Namespaces, cmd.Namespace)
	if _, exists := cfg.UpstreamPools[cmd.PoolID]; !exists {
		return configstore.DesiredState{}, fmt.Errorf("upstream pool %q was not found", cmd.PoolID)
	}
	for _, routeCfg := range cfg.Routes {
		if routeCfg.UpstreamPool == cmd.PoolID {
			return configstore.DesiredState{}, fmt.Errorf("upstream pool %q is still referenced by route %q", cmd.PoolID, routeCfg.ID)
		}
	}
	delete(cfg.UpstreamPools, cmd.PoolID)
	next.Namespaces[cmd.Namespace] = cfg
case CommandImportJSONConfig:
	if len(next.Namespaces) > 0 {
		return configstore.DesiredState{}, fmt.Errorf("import_json_config requires empty FSM state")
	}
	next.Namespaces = make(map[string]proxyconfig.Config, len(cmd.Import))
	for namespace, cfg := range cmd.Import {
		next.Namespaces[namespace] = cfg
	}
```

Add these helper functions below `applyCommand`:

```go
func ensureNamespace(namespaces map[string]proxyconfig.Config, namespace string) proxyconfig.Config {
	cfg, exists := namespaces[namespace]
	if !exists {
		cfg = proxyconfig.Config{}
	}
	cfg.Routes = append([]proxyconfig.RouteConfig(nil), cfg.Routes...)
	if cfg.UpstreamPools == nil {
		cfg.UpstreamPools = map[string]proxyconfig.UpstreamPool{}
	} else {
		pools := make(map[string]proxyconfig.UpstreamPool, len(cfg.UpstreamPools))
		for id, pool := range cfg.UpstreamPools {
			pools[id] = pool
		}
		cfg.UpstreamPools = pools
	}
	return cfg
}

func validateDesiredState(state configstore.DesiredState) []proxyconfig.ValidationError {
	var errs []proxyconfig.ValidationError
	for namespace, cfg := range state.Namespaces {
		for _, err := range cfg.Validate() {
			err.Field = namespace + "." + err.Field
			errs = append(errs, err)
		}
	}
	return errs
}
```

- [ ] **Step 6: Implement snapshot persistence**

Create `internal/raftconfig/fsm_snapshot.go`:

```go
package raftconfig

import (
	"encoding/json"
	"io"

	"github.com/hashicorp/raft"

	"reverseproxy-poc/internal/configstore"
	"reverseproxy-poc/internal/proxyconfig"
)

type fsmSnapshot struct {
	state configstore.DesiredState
}

func newFSMSnapshot(state configstore.DesiredState) raft.FSMSnapshot {
	return &fsmSnapshot{state: state}
}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	encoder := json.NewEncoder(sink)
	if err := encoder.Encode(s.state); err != nil {
		_ = sink.Cancel()
		return err
	}
	return sink.Close()
}

func (s *fsmSnapshot) Release() {}

func decodeSnapshot(reader io.Reader) (configstore.DesiredState, error) {
	var state configstore.DesiredState
	if err := json.NewDecoder(reader).Decode(&state); err != nil {
		return configstore.DesiredState{}, err
	}
	if state.Namespaces == nil {
		state.Namespaces = map[string]proxyconfig.Config{}
	}
	return state, nil
}

func cloneDesiredState(state configstore.DesiredState) configstore.DesiredState {
	cloned := configstore.DesiredState{
		Namespaces: make(map[string]proxyconfig.Config, len(state.Namespaces)),
		Version:    state.Version,
		AppliedAt: state.AppliedAt,
	}
	for namespace, cfg := range state.Namespaces {
		cloned.Namespaces[namespace] = cloneConfig(cfg)
	}
	return cloned
}

func cloneConfig(cfg proxyconfig.Config) proxyconfig.Config {
	cloned := proxyconfig.Config{Name: cfg.Name}
	cloned.Routes = append([]proxyconfig.RouteConfig(nil), cfg.Routes...)
	cloned.UpstreamPools = make(map[string]proxyconfig.UpstreamPool, len(cfg.UpstreamPools))
	for id, pool := range cfg.UpstreamPools {
		cloned.UpstreamPools[id] = pool
	}
	return cloned
}
```

- [ ] **Step 7: Run FSM tests and commit**

Run: `go test ./internal/raftconfig`

Expected: PASS.

Commit:

```bash
git add go.mod go.sum internal/raftconfig/command.go internal/raftconfig/fsm.go internal/raftconfig/fsm_snapshot.go internal/raftconfig/fsm_test.go
git commit -m "feat(raft): add config fsm"
```

## Task 5: Raft Store and Node Wiring

**Files:**
- Create: `internal/raftconfig/node.go`
- Create: `internal/raftconfig/store.go`
- Test: `internal/raftconfig/store_test.go`

- [ ] **Step 1: Add Bolt store dependency**

Run:

```bash
go get github.com/hashicorp/raft-boltdb/v2@latest
```

Expected: `go.mod` gains `github.com/hashicorp/raft-boltdb/v2`.

- [ ] **Step 2: Write failing not-leader store test**

Add to `internal/raftconfig/store_test.go`:

```go
package raftconfig

import (
	"context"
	"testing"

	"reverseproxy-poc/internal/configstore"
	"reverseproxy-poc/internal/proxyconfig"
)

func TestStoreReturnsNotLeaderWhenNodeIsFollower(t *testing.T) {
	store := NewStore(&fakeRaft{leader: "127.0.0.1:7001", state: "Follower"}, NewFSM())

	_, err := store.CreateNamespace(context.Background(), "admin")
	if err == nil {
		t.Fatal("CreateNamespace() error = nil, want not leader")
	}
	if !configstore.IsNotLeader(err) {
		t.Fatalf("CreateNamespace() error = %v, want not leader", err)
	}
}

func TestStoreAppliesCommandOnLeader(t *testing.T) {
	fsm := NewFSM()
	store := NewStore(&fakeRaft{state: "Leader", apply: fsm.Apply}, fsm)

	_, err := store.CreateUpstreamPool(context.Background(), "default", "pool-api", proxyconfig.UpstreamPool{
		Upstreams: []string{"10.0.0.11:8080"},
	})
	if err != nil {
		t.Fatalf("CreateUpstreamPool() error = %v", err)
	}
	state := fsm.DesiredState()
	if _, ok := state.Namespaces["default"].UpstreamPools["pool-api"]; !ok {
		t.Fatal("pool-api missing from FSM state")
	}
}
```

- [ ] **Step 3: Define Raft interface and store**

Create `internal/raftconfig/store.go`:

```go
package raftconfig

import (
	"context"
	"time"

	"github.com/hashicorp/raft"

	"reverseproxy-poc/internal/configstore"
	"reverseproxy-poc/internal/proxyconfig"
)

type raftApplier interface {
	State() raft.RaftState
	Leader() raft.ServerAddress
	Apply(cmd []byte, timeout time.Duration) raft.ApplyFuture
}

type Store struct {
	raft raftApplier
	fsm  *FSM
	timeout time.Duration
}

func NewStore(node raftApplier, fsm *FSM) *Store {
	return &Store{raft: node, fsm: fsm, timeout: 5 * time.Second}
}

func (s *Store) DesiredState(context.Context) (configstore.DesiredState, error) {
	return s.fsm.DesiredState(), nil
}

func (s *Store) CreateNamespace(ctx context.Context, namespace string) (configstore.NamespaceSummary, error) {
	err := s.apply(ctx, Command{Type: CommandCreateNamespace, Namespace: namespace})
	return configstore.NamespaceSummary{Namespace: namespace, Exists: err == nil}, err
}

func (s *Store) CreateUpstreamPool(ctx context.Context, namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error) {
	err := s.apply(ctx, Command{Type: CommandCreateUpstreamPool, Namespace: namespace, PoolID: id, Pool: pool})
	return pool, err
}

func (s *Store) apply(ctx context.Context, cmd Command) error {
	if s.raft.State() != raft.Leader {
		return configstore.NewNotLeaderError(string(s.raft.Leader()))
	}
	data, err := EncodeCommand(cmd)
	if err != nil {
		return err
	}
	future := s.raft.Apply(data, s.timeout)
	if err := future.Error(); err != nil {
		return err
	}
	if resp, ok := future.Response().(ApplyResponse); ok && resp.Error != "" {
		return &configstore.StoreError{StatusCode: 422, Code: "raft_apply_rejected", Message: resp.Error}
	}
	return nil
}
```

Add the remaining `configstore.Store` methods to `internal/raftconfig/store.go`:

```go
func (s *Store) ListNamespaces(ctx context.Context) ([]configstore.NamespaceSummary, error) {
	state := s.fsm.DesiredState()
	items := make([]configstore.NamespaceSummary, 0, len(state.Namespaces)+1)
	hasDefault := false
	for namespace, cfg := range state.Namespaces {
		if namespace == configstore.DefaultNamespace {
			hasDefault = true
		}
		items = append(items, configstore.NamespaceSummary{Namespace: namespace, Exists: true, RouteCount: len(cfg.Routes), UpstreamPoolCount: len(cfg.UpstreamPools)})
	}
	if !hasDefault {
		items = append(items, configstore.NamespaceSummary{Namespace: configstore.DefaultNamespace})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Namespace < items[j].Namespace })
	return items, nil
}

func (s *Store) GetNamespaceConfig(ctx context.Context, namespace string) (configstore.NamespaceConfig, error) {
	state := s.fsm.DesiredState()
	cfg, exists := state.Namespaces[namespace]
	return configstore.NamespaceConfig{Namespace: namespace, Exists: exists, Routes: cfg.Routes, UpstreamPools: cfg.UpstreamPools, AppliedAt: state.AppliedAt}, nil
}

func (s *Store) DeleteNamespace(ctx context.Context, namespace string) error {
	return s.apply(ctx, Command{Type: CommandDeleteNamespace, Namespace: namespace})
}

func (s *Store) CreateRoute(ctx context.Context, namespace string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error) {
	err := s.apply(ctx, Command{Type: CommandCreateRoute, Namespace: namespace, Route: route})
	return route, err
}

func (s *Store) UpdateRoute(ctx context.Context, namespace, id string, route proxyconfig.RouteConfig) (proxyconfig.RouteConfig, error) {
	err := s.apply(ctx, Command{Type: CommandUpdateRoute, Namespace: namespace, RouteID: id, Route: route})
	return route, err
}

func (s *Store) DeleteRoute(ctx context.Context, namespace, id string) error {
	return s.apply(ctx, Command{Type: CommandDeleteRoute, Namespace: namespace, RouteID: id})
}

func (s *Store) UpdateUpstreamPool(ctx context.Context, namespace, id string, pool proxyconfig.UpstreamPool) (proxyconfig.UpstreamPool, error) {
	err := s.apply(ctx, Command{Type: CommandUpdateUpstreamPool, Namespace: namespace, PoolID: id, Pool: pool})
	return pool, err
}

func (s *Store) DeleteUpstreamPool(ctx context.Context, namespace, id string) error {
	return s.apply(ctx, Command{Type: CommandDeleteUpstreamPool, Namespace: namespace, PoolID: id})
}
```

Add `sort` to the imports in `internal/raftconfig/store.go`.

- [ ] **Step 4: Add node constructor**

Create `internal/raftconfig/node.go`:

```go
package raftconfig

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"

	"reverseproxy-poc/internal/config"
)

type NodeOptions struct {
	AppConfig config.AppConfig
	FSM       *FSM
}

func NewNode(opts NodeOptions) (*raft.Raft, error) {
	if opts.FSM == nil {
		return nil, fmt.Errorf("raft FSM is required")
	}
	if err := os.MkdirAll(opts.AppConfig.RaftDataDir, 0o755); err != nil {
		return nil, err
	}
	conf := raft.DefaultConfig()
	conf.LocalID = raft.ServerID(opts.AppConfig.RaftNodeID)
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(opts.AppConfig.RaftDataDir, "raft-log.bolt"))
	if err != nil {
		return nil, err
	}
	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(opts.AppConfig.RaftDataDir, "raft-stable.bolt"))
	if err != nil {
		return nil, err
	}
	snapshotStore, err := raft.NewFileSnapshotStore(filepath.Join(opts.AppConfig.RaftDataDir, "snapshots"), 2, os.Stderr)
	if err != nil {
		return nil, err
	}
	addr, err := net.ResolveTCPAddr("tcp", opts.AppConfig.RaftBindAddr)
	if err != nil {
		return nil, err
	}
	transport, err := raft.NewTCPTransport(opts.AppConfig.RaftBindAddr, addr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return nil, err
	}
	node, err := raft.NewRaft(conf, opts.FSM, logStore, stableStore, snapshotStore, transport)
	if err != nil {
		return nil, err
	}
	hasState, err := raft.HasExistingState(logStore, stableStore, snapshotStore)
	if err != nil {
		return nil, err
	}
	if opts.AppConfig.RaftBootstrap && !hasState {
		cfg := raft.Configuration{Servers: []raft.Server{{
			ID:      conf.LocalID,
			Address: raft.ServerAddress(opts.AppConfig.RaftAdvertiseAddr),
		}}}
		if err := node.BootstrapCluster(cfg).Error(); err != nil {
			return nil, err
		}
	}
	return node, nil
}
```

- [ ] **Step 5: Run raftconfig tests and commit**

Run: `go test ./internal/raftconfig`

Expected: PASS.

Commit:

```bash
git add go.mod go.sum internal/raftconfig/node.go internal/raftconfig/store.go internal/raftconfig/store_test.go
git commit -m "feat(raft): add config store"
```

## Task 6: Application Config and Runtime Wiring

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/validate.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/app/app.go`
- Modify: `internal/app/reload.go`
- Test: `internal/app/app_test.go`

- [ ] **Step 1: Write failing config validation tests**

Add to `internal/config/config_test.go`:

```go
func TestValidate_RaftModeRequiresNodeSettings(t *testing.T) {
	cfg := Default()
	cfg.ConfigStore = "raft"

	if err := Validate(cfg); err == nil {
		t.Fatal("Validate() error = nil, want missing raft settings")
	}
}

func TestValidate_RaftModeAcceptsRequiredNodeSettings(t *testing.T) {
	cfg := Default()
	cfg.ConfigStore = "raft"
	cfg.RaftNodeID = "node-1"
	cfg.RaftBindAddr = "127.0.0.1:7001"
	cfg.RaftAdvertiseAddr = "127.0.0.1:7001"
	cfg.RaftDataDir = "data/node-1"

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
```

- [ ] **Step 2: Add config fields and validation**

Modify `internal/config/config.go`:

```go
type AppConfig struct {
	ProxyListenAddr     string `json:"proxyListenAddr"`
	DashboardListenAddr string `json:"dashboardListenAddr"`
	ProxyConfigDir      string `json:"proxyConfigDir"`
	ConfigStore         string `json:"configStore,omitempty"`
	RaftNodeID          string `json:"raftNodeId,omitempty"`
	RaftBindAddr        string `json:"raftBindAddr,omitempty"`
	RaftAdvertiseAddr   string `json:"raftAdvertiseAddr,omitempty"`
	RaftDataDir         string `json:"raftDataDir,omitempty"`
	RaftBootstrap       bool   `json:"raftBootstrap,omitempty"`
	RaftJoinAddr        string `json:"raftJoinAddr,omitempty"`
	RaftJSONSeedDir     string `json:"raftJsonSeedDir,omitempty"`
}
```

Set default:

```go
ConfigStore: "file",
```

Modify `internal/config/validate.go` to reject missing Raft settings when `ConfigStore == "raft"` and to reject unknown store values.

- [ ] **Step 3: Wire app store selection**

Modify `internal/app/app.go` so `New` creates:

```go
fileStore := configstore.NewFileStore(cfg)
desired, err := fileStore.DesiredState(context.Background())
snapshot, err := configstore.ProjectSnapshot(cfg, desired)
```

For raft mode:

```go
fsm := raftconfig.NewFSMWithConfig(cfg, func(state configstore.DesiredState) {
	snapshot, err := configstore.ProjectSnapshot(cfg, state)
	if err != nil {
		logger.Printf("project raft config: %v", err)
		return
	}
	app.state.Swap(snapshot)
	app.swapHealthChecker(snapshot.Upstreams)
})
node, err := raftconfig.NewNode(raftconfig.NodeOptions{AppConfig: cfg, FSM: fsm})
store := raftconfig.NewStore(node, fsm)
```

Preserve file mode as the default path.

- [ ] **Step 4: Disable implicit proxy JSON reload in Raft mode**

Modify `internal/app/reload.go`:

```go
if cfg.ConfigStore == "raft" {
	return fmt.Errorf("file reload is disabled when configStore is raft")
}
```

- [ ] **Step 5: Run app/config tests and commit**

Run: `go test ./internal/config ./internal/app`

Expected: PASS.

Commit:

```bash
git add internal/config/config.go internal/config/validate.go internal/config/config_test.go internal/app/app.go internal/app/reload.go internal/app/app_test.go
git commit -m "feat(app): wire config store modes"
```

## Task 7: Dashboard API Error Mapping and Docs

**Files:**
- Modify: `internal/dashboard/handler.go`
- Modify: `internal/dashboard/config_api.go`
- Test: `internal/dashboard/api_test.go`
- Modify: `docs/api/dashboard-api.ko.md`
- Create: `docs/architecture/raft-config-state.ko.md`

- [ ] **Step 1: Write failing not-leader API test**

Add to `internal/dashboard/api_test.go`:

```go
func TestConfigAPI_NotLeaderErrorIncludesLeaderAddress(t *testing.T) {
	handler := NewHandler(runtime.NewState(runtime.Snapshot{}), stubService{
		createNamespaceErr: &admin.APIError{
			StatusCode: http.StatusConflict,
			Message:    "configuration writes must be sent to the raft leader",
			Err:        configstore.NewNotLeaderError("127.0.0.1:9090"),
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/namespaces", strings.NewReader(`{"namespace":"admin"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusConflict; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(rec.Body.String(), `"code":"not_raft_leader"`) {
		t.Fatalf("response body = %s, want not_raft_leader code", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"leader_address":"127.0.0.1:9090"`) {
		t.Fatalf("response body = %s, want leader_address", rec.Body.String())
	}
}
```

- [ ] **Step 2: Extend API error response**

Modify dashboard API error serialization to include fields:

```go
type errorResponse struct {
	Message       string                         `json:"message"`
	Code          string                         `json:"code,omitempty"`
	LeaderAddress string                        `json:"leader_address,omitempty"`
	Errors        []proxyconfig.ValidationError `json:"errors,omitempty"`
}
```

When `errors.As(apiErr.Err, &configstore.StoreError{})` succeeds, copy `Code` and `LeaderAddress` into the JSON body.

- [ ] **Step 3: Update docs**

Add a section to `docs/api/dashboard-api.ko.md`:

```markdown
## HA 모드 오류

`configStore`가 `raft`인 노드에서 설정 쓰기 요청이 follower에 도착하면 첫 구현은 leader forward를 하지 않는다.
응답은 `409 Conflict`이며 body는 다음 형태다.

```json
{
  "message": "configuration writes must be sent to the raft leader",
  "code": "not_raft_leader",
  "leader_address": "127.0.0.1:9090"
}
```

런타임 health 상태는 Raft 복제 상태가 아니라 응답한 노드의 로컬 관측값이다.
```

Create `docs/architecture/raft-config-state.ko.md` with the operational rules from the design spec: JSON seed only on brand-new bootstrap, Raft state wins on restart/join, and proxy JSON is not auto-reloaded in HA mode.

- [ ] **Step 4: Run dashboard tests and commit**

Run: `go test ./internal/dashboard`

Expected: PASS.

Commit:

```bash
git add internal/dashboard/handler.go internal/dashboard/config_api.go internal/dashboard/api_test.go docs/api/dashboard-api.ko.md docs/architecture/raft-config-state.ko.md
git commit -m "feat(dashboard): expose raft write errors"
```

## Task 8: Integration and Full Verification

**Files:**
- Create: `internal/raftconfig/integration_test.go`
- Modify: `docs/superpowers/specs/2026-05-14-raft-config-state-design.md` if implementation changes the approved behavior

- [ ] **Step 1: Write integration test for in-memory cluster**

Create `internal/raftconfig/integration_test.go`:

```go
package raftconfig

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/raft"

	"reverseproxy-poc/internal/configstore"
)

func TestIntegrationThreeNodeClusterReplicatesNamespace(t *testing.T) {
	cluster := newInmemCluster(t, 3)
	defer cluster.Close()

	leader := cluster.LeaderStore(t)
	if _, err := leader.CreateNamespace(context.Background(), "admin"); err != nil {
		t.Fatalf("CreateNamespace() error = %v", err)
	}

	eventually(t, 3*time.Second, func() bool {
		for _, store := range cluster.stores {
			state, err := store.DesiredState(context.Background())
			if err != nil {
				return false
			}
			if _, ok := state.Namespaces["admin"]; !ok {
				return false
			}
		}
		return true
	})
}

func TestIntegrationFollowerRejectsWriteWithLeader(t *testing.T) {
	cluster := newInmemCluster(t, 3)
	defer cluster.Close()

	follower := cluster.FollowerStore(t)
	_, err := follower.CreateNamespace(context.Background(), "admin")
	if err == nil {
		t.Fatal("CreateNamespace() error = nil, want not leader")
	}
	if !configstore.IsNotLeader(err) {
		t.Fatalf("CreateNamespace() error = %v, want not leader", err)
	}
}
```

Implement `newInmemCluster`, `LeaderStore`, `FollowerStore`, `Close`, and `eventually` in the same file using `raft.NewInmemTransport`, `raft.NewInmemStore`, `raft.NewDiscardSnapshotStore`, and `raft.BootstrapCluster`.

- [ ] **Step 2: Run integration tests**

Run: `go test ./internal/raftconfig -run Integration -count=1`

Expected: PASS.

- [ ] **Step 3: Run full test suite**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 4: Check worktree**

Run: `git status --short`

Expected: only intentional implementation files are modified. Existing unrelated user changes, such as `D AGENTS.md`, must not be staged unless the user explicitly asks.

- [ ] **Step 5: Commit**

```bash
git add internal/raftconfig/integration_test.go
git commit -m "test(raft): verify config replication"
```

## Self-Review Checklist

- Spec coverage: desired config is Raft-managed, request-time state remains local, JSON seed is bootstrap-only, follower writes return leader information, and runtime projection is local per node.
- Placeholder scan: no task uses open-ended placeholders; each implementation task gives concrete files, functions, commands, and expected results.
- Type consistency: the plan consistently uses `configstore.DesiredState`, `configstore.Store`, `raftconfig.Command`, `raftconfig.FSM`, and `raftconfig.Store`.

## Execution Notes

Use frequent commits exactly as listed. Before each commit, run the task-specific test command. Before final handoff, run `go test ./...` and inspect `git status --short`.
