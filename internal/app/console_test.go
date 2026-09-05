package app

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestConsolePlainOutputHasNoANSI(t *testing.T) {
	var out, err bytes.Buffer
	c := newConsole(&out, &err, false, false)
	c.Success("installed %s", "8.4")
	c.Warn("EOL")
	if strings.Contains(out.String(), "\x1b[") || strings.Contains(err.String(), "\x1b[") {
		t.Fatal("non-terminal output contains ANSI escapes")
	}
}
func TestGlobalFlagsDoNotConsumeChildArguments(t *testing.T) {
	args, plain, verbose := stripGlobalFlags([]string{"--plain", "exec", "--", "tool", "--verbose"})
	if !plain || verbose {
		t.Fatal("unexpected global options")
	}
	if strings.Join(args, " ") != "exec -- tool --verbose" {
		t.Fatalf("child arguments changed: %v", args)
	}
}
func TestNoColorEnvironment(t *testing.T) {
	old := os.Getenv("NO_COLOR")
	t.Cleanup(func() { _ = os.Setenv("NO_COLOR", old) })
	_ = os.Setenv("NO_COLOR", "1")
	var b bytes.Buffer
	c := newConsole(&b, &b, false, false)
	if c.color {
		t.Fatal("NO_COLOR was ignored")
	}
}
