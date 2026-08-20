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
func TestPowerShellQuote(t *testing.T) {
	if got := psQuote(`C:\It's\phpvm.exe`); got != `'C:\It''s\phpvm.exe'` {
		t.Fatalf("unexpected quote %s", got)
	}
}
