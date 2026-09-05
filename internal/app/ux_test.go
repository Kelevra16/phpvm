package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kelevra16/phpvm/internal/store"
)

func TestSelectOneAcceptsNumberedChoice(t *testing.T) {
	var out bytes.Buffer
	a := New("test")
	a.In = strings.NewReader("2\n")
	a.Out = &out
	selected, err := a.selectOne("Choose", []string{"8.3", "8.4"})
	if err != nil {
		t.Fatal(err)
	}
	if selected != "8.4" {
		t.Fatalf("selected %q", selected)
	}
}

func TestSelectOneFiltersByText(t *testing.T) {
	a := New("test")
	a.In = strings.NewReader("legacy\n")
	a.Out = &bytes.Buffer{}
	selected, err := a.selectOne("Choose", []string{"8.4 stable", "7.4 legacy"})
	if err != nil || selected != "7.4 legacy" {
		t.Fatalf("selected=%q err=%v", selected, err)
	}
}

func TestConfirmAcceptsSpanishYes(t *testing.T) {
	a := New("test")
	a.In = strings.NewReader("sí\n")
	a.Out = &bytes.Buffer{}
	ok, err := a.confirm("Continue", false)
	if err != nil || !ok {
		t.Fatalf("confirmation=%v err=%v", ok, err)
	}
}

func TestFriendlyErrorUsesConfiguredLanguage(t *testing.T) {
	t.Setenv("PHPVM_LANG", "es")
	message := FriendlyError(os.ErrNotExist)
	if strings.Contains(message, "Sugerencia") {
		t.Fatalf("unrelated errors should not receive a hint: %q", message)
	}
	message = FriendlyError(assertError("PHP build 8.4 is not installed"))
	if !strings.Contains(message, "Sugerencia") || !strings.Contains(message, "phpvm install") {
		t.Fatalf("missing Spanish hint: %q", message)
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }

func TestInitProjectNonInteractive(t *testing.T) {
	project := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	var out bytes.Buffer
	a := New("test")
	a.Out = &out
	a.ui = newConsole(&out, &out, true, false)
	if err := a.initProject(store.New(t.TempDir()), []string{"--version", "8.4", "--preset", "codeigniter"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(project, "phpvm.toml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.Contains(text, `version = "8.4"`) || !strings.Contains(text, "[ini]") {
		t.Fatalf("unexpected config:\n%s", text)
	}
}

func TestWithoutYes(t *testing.T) {
	args, yes := withoutYes([]string{"8.4", "--yes"})
	if !yes || len(args) != 1 || args[0] != "8.4" {
		t.Fatalf("args=%v yes=%v", args, yes)
	}
}
