package configstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestFileStore_DesiredStateWaitsForStoreMutex(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(config.AppConfig{ProxyConfigDir: dir})

	store.mu.Lock()
	done := make(chan error, 1)
	go func() {
		_, err := store.DesiredState(context.Background())
		done <- err
	}()

	select {
	case err := <-done:
		store.mu.Unlock()
		t.Fatalf("DesiredState() completed before mutex unlock with error = %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	store.mu.Unlock()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("DesiredState() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("DesiredState() did not complete after mutex unlock")
	}
}

func writeConfigFileForTest(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
}
