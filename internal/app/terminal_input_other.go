//go:build !windows

package app

import "golang.org/x/term"

func isMenuTerminal(fd int) bool { return term.IsTerminal(fd) }

func makeMenuRaw(fd int) (func(), error) {
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	return func() { _ = term.Restore(fd, state) }, nil
}
