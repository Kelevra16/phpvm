package update

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractExecutable(t *testing.T) {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	f, err := z.Create("phpvm.exe")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte("binary"))
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "phpvm.exe.new")
	if err := extractExecutable(b.Bytes(), dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "binary" {
		t.Fatalf("unexpected executable %q", got)
	}
}
func TestScheduleSwapsExecutableTransactionally(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "phpvm.exe")
	staged := current + ".new"
	if err := os.WriteFile(current, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("new"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := Schedule(Result{CurrentPath: current, StagedPath: staged}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(current)
	if err != nil || string(got) != "new" {
		t.Fatalf("current=%q err=%v", got, err)
	}
	old, err := os.ReadFile(current + ".old")
	if err != nil || string(old) != "old" {
		t.Fatalf("backup=%q err=%v", old, err)
	}
	shim, err := os.ReadFile(filepath.Join(dir, "php.exe"))
	if err != nil || string(shim) != "new" {
		t.Fatalf("shim=%q err=%v", shim, err)
	}
}

func TestScheduleUpdatesExistingPHPShim(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "phpvm.exe")
	staged := current + ".new"
	shim := filepath.Join(dir, "php.exe")
	for path, body := range map[string]string{current: "old phpvm", staged: "new binary", shim: "old shim"} {
		if err := os.WriteFile(path, []byte(body), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := Schedule(Result{CurrentPath: current, StagedPath: staged}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(shim)
	if err != nil || string(got) != "new binary" {
		t.Fatalf("shim=%q err=%v", got, err)
	}
}

func TestEnsurePHPShimAfterLegacyUpdate(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "phpvm.exe")
	if err := os.WriteFile(exe, []byte("new binary"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe+".old", []byte("old binary"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := ensurePHPShim(exe); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "php.exe"))
	if err != nil || string(got) != "new binary" {
		t.Fatalf("shim=%q err=%v", got, err)
	}
}

func TestEnsurePHPShimCreatesMissingShimForRelease(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "phpvm.exe")
	if err := os.WriteFile(exe, []byte("release binary"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := ensurePHPShim(exe); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "php.exe"))
	if err != nil || string(got) != "release binary" {
		t.Fatalf("shim=%q err=%v", got, err)
	}
}
