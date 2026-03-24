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

func writeConfigFile(t *testing.T, path, body string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
}
