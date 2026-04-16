package proxyconfig

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadDir_LoadsJSONFilesInSortedOrder(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, filepath.Join(dir, "b.json"), `{"routes":[],"upstream_pools":{}}`)
	writeTestFile(t, filepath.Join(dir, "a.json"), `{"routes":[],"upstream_pools":{}}`)
	writeTestFile(t, filepath.Join(dir, "note.txt"), `ignored`)

	loaded, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir() error = %v", err)
	}

	got := []string{loaded[0].Source, loaded[1].Source}
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadDir() sources = %v, want %v", got, want)
	}
}

func TestSourceFromPath(t *testing.T) {
	got, err := SourceFromPath(filepath.Join("configs", "proxy", "default.json"))
	if err != nil {
		t.Fatalf("SourceFromPath() error = %v", err)
	}
	if got != "default" {
		t.Fatalf("SourceFromPath() = %q, want %q", got, "default")
	}
}

func writeTestFile(t *testing.T, path, data string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
}
