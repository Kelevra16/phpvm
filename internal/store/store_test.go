package store

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testArchive(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	f, err := z.Create("php.exe")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte("fake-php"))
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestFailedValidationIsNotPublished(t *testing.T) {
	archive := testArchive(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(archive) }))
	defer server.Close()
	h := sha256.Sum256(archive)
	s := New(t.TempDir())
	s.Validate = func(context.Context, string) error { return errors.New("missing runtime") }
	m := Metadata{Version: "5.6.40", Variant: "nts", Arch: "x64", URL: server.URL, ArchiveSHA256: hex.EncodeToString(h[:])}
	if err := s.Install(context.Background(), m); err == nil {
		t.Fatal("expected validation failure")
	}
	if s.IsInstalled(m.ID()) {
		t.Fatal("failed build was published")
	}
}
func TestTransactionalInstallAndVerify(t *testing.T) {
	archive := testArchive(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(archive) }))
	defer server.Close()
	h := sha256.Sum256(archive)
	s := New(t.TempDir())
	s.Validate = func(context.Context, string) error { return nil }
	m := Metadata{Version: "8.4.1", Variant: "nts", Arch: "x64", URL: server.URL, ArchiveSHA256: hex.EncodeToString(h[:])}
	if err := s.Install(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if !s.IsInstalled(m.ID()) {
		t.Fatal("build not installed")
	}
	if err := s.Verify(m.ID()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.Executable(m.ID()), []byte("tampered"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := s.Verify(m.ID()); err == nil {
		t.Fatal("expected checksum failure")
	}
}

func TestOfflineInstallUsesVerifiedArchiveCache(t *testing.T) {
	archive := testArchive(t)
	h := sha256.Sum256(archive)
	digest := hex.EncodeToString(h[:])
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache", "archives")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, digest+".zip"), archive, 0644); err != nil {
		t.Fatal(err)
	}
	s := New(root)
	s.Offline = true
	s.Validate = func(context.Context, string) error { return nil }
	m := Metadata{Version: "8.4.1", Variant: "nts", Arch: "x64", URL: "https://invalid.example/php.zip", ArchiveSHA256: digest}
	if err := s.Install(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if !s.IsInstalled(m.ID()) {
		t.Fatal("cached build not installed")
	}
}

func TestOfflineInstallRejectsTamperedArchiveCache(t *testing.T) {
	archive := testArchive(t)
	h := sha256.Sum256(archive)
	digest := hex.EncodeToString(h[:])
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache", "archives")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, digest+".zip"), []byte("tampered"), 0644); err != nil {
		t.Fatal(err)
	}
	s := New(root)
	s.Offline = true
	m := Metadata{Version: "8.4.1", Variant: "nts", Arch: "x64", URL: "https://invalid.example/php.zip", ArchiveSHA256: digest}
	if err := s.Install(context.Background(), m); err == nil || !strings.Contains(err.Error(), "not available in cache") {
		t.Fatalf("expected offline cache rejection, got %v", err)
	}
}
func TestZipTraversalRejected(t *testing.T) {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	f, _ := z.Create("../outside.txt")
	_, _ = f.Write([]byte("bad"))
	_ = z.Close()
	zipPath := filepath.Join(t.TempDir(), "bad.zip")
	_ = os.WriteFile(zipPath, b.Bytes(), 0644)
	if err := unzip(zipPath, t.TempDir()); err == nil {
		t.Fatal("expected traversal error")
	}
}

func TestAbandonedLockIsRecovered(t *testing.T) {
	s := New(t.TempDir())
	if err := os.WriteFile(filepath.Join(s.Root, ".lock"), []byte("999999999\n"), 0600); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := s.WithLock(context.Background(), func() error { called = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("locked operation was not called")
	}
}

func TestDynamicWrapperUsesProjectResolver(t *testing.T) {
	wrapper := dynamicWrapper("php")
	if !strings.Contains(wrapper, "phpvm resolve --path --tool php") {
		t.Fatalf("wrapper does not use dynamic resolver: %s", wrapper)
	}
}

func TestConfigureDefaultPHP(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ext"), 0755); err != nil {
		t.Fatal(err)
	}
	template := ";extension=mbstring\nextension=mysqli\nextension=mysqli\n;extension=curl\nmemory_limit = 128M\n"
	if err := os.WriteFile(filepath.Join(dir, "php.ini-development"), []byte(template), 0644); err != nil {
		t.Fatal(err)
	}
	for _, dll := range []string{"php_mbstring.dll", "php_mysqli.dll", "php_curl.dll"} {
		if err := os.WriteFile(filepath.Join(dir, "ext", dll), []byte("dll"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	enabled, err := configureDefaultPHP(dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(enabled, ",") != "curl,mbstring,mysqli" {
		t.Fatalf("enabled=%v", enabled)
	}
	b, err := os.ReadFile(filepath.Join(dir, "php.ini"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{"extension=php_curl.dll", "extension=php_mbstring.dll", "extension=php_mysqli.dll", "memory_limit = 512M", "display_errors = On"} {
		if !strings.Contains(text, want) {
			t.Errorf("php.ini lacks %q", want)
		}
	}
	if strings.Count(text, "extension=php_mysqli.dll") != 2 {
		t.Fatalf("expected one enabled and one commented normalized entry: %q", text)
	}
}
