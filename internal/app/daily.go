package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Kelevra16/phpvm/internal/store"
	"github.com/Kelevra16/phpvm/internal/update"
)

func (a *App) which(s *store.Store, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: phpvm which [build]")
	}
	id, err := s.Current()
	if len(args) == 1 {
		id, err = resolveInstalled(s, args[0])
	}
	if err != nil {
		return err
	}
	path := s.Executable(id)
	if _, err := os.Stat(path); err != nil {
		return err
	}
	fmt.Fprintln(a.Out, path)
	return nil
}

func (a *App) cache(s *store.Store, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: phpvm cache <dir|clear>")
	}
	path := filepath.Join(s.Root, "cache")
	switch args[0] {
	case "dir":
		fmt.Fprintln(a.Out, path)
		return nil
	case "clear":
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		fmt.Fprintln(a.Out, "Cleared", path)
		return nil
	default:
		return fmt.Errorf("usage: phpvm cache <dir|clear>")
	}
}

func (a *App) selfUpdate(ctx context.Context, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: phpvm self-update [version]")
	}
	version := "latest"
	if len(args) == 1 {
		version = args[0]
	}
	if runtime.GOOS != "windows" {
		return fmt.Errorf("self-update currently supports Windows only")
	}
	result, err := update.Prepare(ctx, "Kelevra16/phpvm", version, a.Version)
	if err != nil {
		return err
	}
	if result.UpToDate {
		fmt.Fprintln(a.Out, "phpvm", result.Version, "is already current")
		return nil
	}
	if err := update.Schedule(result); err != nil {
		return err
	}
	fmt.Fprintln(a.Out, "Prepared phpvm", result.Version+"; the executable will be replaced after this command exits")
	return nil
}
