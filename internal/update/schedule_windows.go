//go:build windows

package update

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func replacementCommand(script string) *exec.Cmd {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd
}

func startReplacement(script string) error { return replacementCommand(script).Start() }
