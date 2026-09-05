//go:build windows

package update

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestReplacementCommandDoesNotChangeTerminalWindow(t *testing.T) {
	cmd := replacementCommand("Write-Output ok")
	joined := strings.Join(cmd.Args, " ")
	if strings.Contains(strings.ToLower(joined), "windowstyle") {
		t.Fatalf("replacement command changes window style: %s", joined)
	}
	if cmd.SysProcAttr == nil || cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatal("replacement command must run without creating or modifying a console window")
	}
}
