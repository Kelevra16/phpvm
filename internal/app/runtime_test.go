package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kelevra16/phpvm/internal/store"
)

func addTestBuild(t *testing.T, s *store.Store, version string) string {
	t.Helper()
	m := store.Metadata{Version: version, Variant: "nts", Arch: "x64"}
	dir := filepath.Join(s.Root, "versions", m.ID())
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "php.exe"), []byte("php"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(dir, "phpvm.json"), m); err != nil {
		t.Fatal(err)
	}
	return m.ID()
}
func TestResolveRuntimePriority(t *testing.T) {
	s := store.New(t.TempDir())
	id83 := addTestBuild(t, s, "8.3.9")
	id84 := addTestBuild(t, s, "8.4.2")
	if err := os.WriteFile(filepath.Join(s.Root, "current"), []byte(id84), 0644); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".php-version"), []byte("8.3"), 0644); err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if got, err := resolveRuntimeBuild(s, ""); err != nil || got != id83 {
		t.Fatalf("project resolve got %q, %v", got, err)
	}
	t.Setenv("PHPVM_ACTIVE", id84)
	if got, err := resolveRuntimeBuild(s, ""); err != nil || got != id84 {
		t.Fatalf("session resolve got %q, %v", got, err)
	}
}
func TestResolveMissingProjectBuildSuggestsSync(t *testing.T) {
	s := store.New(t.TempDir())
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".php-version"), []byte("8.2"), 0644); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	_ = os.Chdir(project)
	defer os.Chdir(old)
	if _, err := resolveRuntimeBuild(s, ""); err == nil {
		t.Fatal("expected missing build error")
	}
}
