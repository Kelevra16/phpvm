package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToggleExtensionDeduplicatesEnabledEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "php.ini")
	if err := os.WriteFile(path, []byte("extension=php_mysqli.dll\r\nextension=php_mysqli.dll\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := toggleExtension(path, "mysqli", true); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := 0
	for _, line := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "extension=php_mysqli.dll" {
			got++
		}
	}
	if got != 1 {
		t.Fatalf("enabled mysqli entries=%d; ini=%q", got, string(b))
	}
}
