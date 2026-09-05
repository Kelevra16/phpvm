//go:build windows

package app

import "golang.org/x/sys/windows"

func isMenuTerminal(fd int) bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(fd), &mode) == nil
}

func makeMenuRaw(fd int) (func(), error) {
	handle := windows.Handle(fd)
	var original uint32
	if err := windows.GetConsoleMode(handle, &original); err != nil {
		return nil, err
	}
	raw := original &^ (windows.ENABLE_ECHO_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_PROCESSED_INPUT)
	raw |= windows.ENABLE_VIRTUAL_TERMINAL_INPUT
	if err := windows.SetConsoleMode(handle, raw); err != nil {
		return nil, err
	}
	return func() { _ = windows.SetConsoleMode(handle, original) }, nil
}
