package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kelevra16/phpvm/internal/store"
)

func TestContainedPathRejectsProjectEscape(t *testing.T) {
	root := t.TempDir()
	if _, err := containedPath(root, filepath.Join("..", "outside")); err == nil {
		t.Fatal("expected project escape rejection")
	}
	if p, err := containedPath(root, "public"); err != nil || p != filepath.Join(root, "public") {
		t.Fatalf("contained path=%q %v", p, err)
	}
}

func TestProjectTrustFollowsConfigAndInvalidatesOnChange(t *testing.T) {
	s := store.New(t.TempDir())
	project := t.TempDir()
	nested := filepath.Join(project, "src")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(project, "phpvm.toml")
	if err := os.WriteFile(config, []byte("version=\"8.4\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	_ = os.Chdir(nested)
	defer os.Chdir(old)
	root, sum, err := projectFingerprint()
	if err != nil || root != project {
		t.Fatalf("fingerprint root=%q err=%v", root, err)
	}
	if err = writeJSON(trustPath(s), trustDB{root: sum}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := projectTrusted(s); !ok {
		t.Fatal("project should be trusted")
	}
	if err = os.WriteFile(config, []byte("version=\"8.5\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if ok, _ := projectTrusted(s); ok {
		t.Fatal("changed project should invalidate trust")
	}
}

func TestComposerLockRefreshPreservesTrustOnlyForExpectedLockChange(t *testing.T) {
	s := store.New(t.TempDir())
	project := t.TempDir()
	composerJSON := filepath.Join(project, "composer.json")
	composerLock := filepath.Join(project, "composer.lock")
	if err := os.WriteFile(composerJSON, []byte(`{"require":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	_ = os.Chdir(project)
	defer os.Chdir(old)
	root, sum, err := projectFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if err = rememberProjectTrust(s, root, sum); err != nil {
		t.Fatal(err)
	}
	rootBefore, stableBefore, err := projectStableFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(composerLock, []byte(`{"packages":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if ok, _ := projectTrusted(s); ok {
		t.Fatal("new composer.lock should initially invalidate trust")
	}
	if err = refreshProjectTrustAfterComposer(s, rootBefore, stableBefore); err != nil {
		t.Fatal(err)
	}
	if ok, _ := projectTrusted(s); !ok {
		t.Fatal("expected composer.lock change should preserve trust")
	}

	rootBefore, stableBefore, _ = projectStableFingerprint()
	if err = os.WriteFile(composerJSON, []byte(`{"scripts":{"test":"unknown"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err = refreshProjectTrustAfterComposer(s, rootBefore, stableBefore); err != nil {
		t.Fatal(err)
	}
	if ok, _ := projectTrusted(s); ok {
		t.Fatal("composer.json change must still invalidate trust")
	}
}

func TestSafeModeBlocksRiskyCommands(t *testing.T) {
	s := store.New(t.TempDir())
	t.Setenv("PHPVM_SAFE_MODE", "1")
	for _, args := range [][]string{{"exec", "--", "php"}, {"serve"}, {"ext", "install", "https://example.test/x.zip"}, {"sync"}} {
		if err := guardRiskyCommand(s, args); err == nil {
			t.Fatalf("safe mode allowed %v", args)
		}
	}
	if err := guardRiskyCommand(s, []string{"status"}); err != nil {
		t.Fatalf("safe command blocked: %v", err)
	}
}
