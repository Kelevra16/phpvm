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
}
