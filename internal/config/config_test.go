package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if got, want := cfg.ProxyListenAddr, ":8080"; got != want {
		t.Fatalf("ProxyListenAddr = %q, want %q", got, want)
	}
	if got, want := cfg.DashboardListenAddr, ":9090"; got != want {
		t.Fatalf("DashboardListenAddr = %q, want %q", got, want)
	}
	if got, want := cfg.ProxyConfigDir, "configs/proxy"; got != want {
		t.Fatalf("ProxyConfigDir = %q, want %q", got, want)
	}
}

func TestLoad_AppliesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.json")
	writeConfigFile(t, path, `{"proxyListenAddr":":18080"}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := cfg.ProxyListenAddr, ":18080"; got != want {
		t.Fatalf("ProxyListenAddr = %q, want %q", got, want)
	}
	if got, want := cfg.DashboardListenAddr, ":9090"; got != want {
		t.Fatalf("DashboardListenAddr = %q, want %q", got, want)
	}
	if got, want := cfg.ProxyConfigDir, "configs/proxy"; got != want {
		t.Fatalf("ProxyConfigDir = %q, want %q", got, want)
	}
}

func TestValidate_RequiresProxyConfigDir(t *testing.T) {
	err := Validate(AppConfig{
		ProxyListenAddr:     ":8080",
		DashboardListenAddr: ":9090",
	})
	if err == nil {
		t.Fatal("Validate() returned nil")
	}
}

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

func TestValidate_RejectsUnknownConfigStore(t *testing.T) {
	cfg := Default()
	cfg.ConfigStore = "postgres"

	if err := Validate(cfg); err == nil {
		t.Fatal("Validate() error = nil, want unknown config store")
	}
}

func writeConfigFile(t *testing.T, path, body string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
}
