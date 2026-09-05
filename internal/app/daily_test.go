package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kelevra16/phpvm/internal/store"
)

func TestWhichAndCache(t *testing.T) {
	root := t.TempDir()
	s := store.New(root)
	m := store.Metadata{Version: "8.4.1", Variant: "nts", Arch: "x64"}
	dir := filepath.Join(root, "versions", m.ID())
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "php.exe"), []byte("php"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(dir, "phpvm.json"), m); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "current"), []byte(m.ID()), 0644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	a := New("test")
	a.Out = &out
	if err := a.which(s, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "php.exe") {
		t.Fatalf("unexpected which output %q", out.String())
	}
	out.Reset()
	if err := a.cache(s, []string{"dir"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "cache") {
		t.Fatalf("unexpected cache output %q", out.String())
	}
}
func TestCompletionPowerShell(t *testing.T) {
	var out bytes.Buffer
	a := New("test")
	a.Out = &out
	if err := a.completion([]string{"powershell"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Register-ArgumentCompleter") {
		t.Fatal("completion script missing registration")
	}
}

func TestCacheBundleRoundTrip(t *testing.T) {
	sourceRoot := t.TempDir()
	payload := []byte("cached archive")
	h := sha256.Sum256(payload)
	digest := hex.EncodeToString(h[:])
	archiveDir := filepath.Join(sourceRoot, "cache", "archives")
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archiveDir, digest+".zip"), payload, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "cache", "windows-releases.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	bundlePath := filepath.Join(t.TempDir(), "offline.zip")
	var out bytes.Buffer
	a := New("test")
	a.Out = &out
	if err := a.bundle(store.New(sourceRoot), []string{"create", bundlePath}); err != nil {
		t.Fatal(err)
	}
	targetRoot := t.TempDir()
	if err := a.bundle(store.New(targetRoot), []string{"import", bundlePath}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(targetRoot, "cache", "archives", digest+".zip"))
	if err != nil || string(got) != string(payload) {
		t.Fatalf("round-trip payload=%q err=%v", got, err)
	}
}
