package store

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
func TestTransactionalInstallAndVerify(t *testing.T) {
	archive := testArchive(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(archive) }))
	defer server.Close()
	h := sha256.Sum256(archive)
	s := New(t.TempDir())
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
