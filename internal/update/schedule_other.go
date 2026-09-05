//go:build !windows

package update

import "fmt"

func startReplacement(string) error {
	return fmt.Errorf("scheduled replacement currently supports Windows only")
}
