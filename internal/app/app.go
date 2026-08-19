package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/megaj/phpvm/internal/store"
	"github.com/megaj/phpvm/internal/windowsphp"
)

type App struct {
	Version string
	Out     io.Writer
	Err     io.Writer
}

func New(version string) *App { return &App{Version: version, Out: os.Stdout, Err: os.Stderr} }

func (a *App) Run(ctx context.Context, args []string) error {
	root, err := rootDir()
	if err != nil {
		return err
	}
	s := store.New(root)
	if len(args) == 0 {
		return a.smart(ctx, s)
	}

	switch args[0] {
	case "help", "-h", "--help":
		a.help()
		return nil
	case "version", "--version":
		fmt.Fprintln(a.Out, "phpvm", a.Version)
		return nil
	case "current":
		v, err := s.Current()
		if err != nil {
			return err
		}
		fmt.Fprintln(a.Out, v)
		return nil
	case "list", "ls":
		versions, err := s.Installed()
		if err != nil {
			return err
		}
		current, _ := s.Current()
		for _, v := range versions {
			mark := "  "
			if v == current {
				mark = "* "
			}
			fmt.Fprintln(a.Out, mark+v)
		}
		return nil
	case "ls-remote":
		p, err := provider()
		if err != nil {
			return err
		}
		versions, err := p.Versions(ctx)
		if err != nil {
			return err
		}
		for _, v := range versions {
			fmt.Fprintln(a.Out, v.Version)
		}
		return nil
	case "install":
		if len(args) != 2 {
			return fmt.Errorf("usage: phpvm install <version>")
		}
		_, err := a.install(ctx, s, args[1])
		return err
	case "use":
		if len(args) != 2 {
			return fmt.Errorf("usage: phpvm use <version|latest|auto>")
		}
		v, err := a.install(ctx, s, args[1])
		if err != nil {
			return err
		}
		if err := s.Use(v); err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "Using PHP %s\n", v)
		return nil
	case "uninstall", "remove", "rm":
		if len(args) != 2 {
			return fmt.Errorf("usage: phpvm uninstall <version>")
		}
		if err := s.Uninstall(args[1]); err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "Removed PHP %s\n", args[1])
		return nil
	case "prune":
		removed, err := s.Prune()
		if err != nil {
			return err
		}
		for _, v := range removed {
			fmt.Fprintf(a.Out, "Removed PHP %s\n", v)
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q (run phpvm help)", args[0])
	}
}

func (a *App) smart(ctx context.Context, s *store.Store) error {
	v, file, err := findVersionFile()
	if err != nil {
		return err
	}
	if v == "" {
		a.help()
		return nil
	}
	fmt.Fprintf(a.Out, "Found %s (%s)\n", v, file)
	resolved, err := a.install(ctx, s, v)
	if err != nil {
		return err
	}
	if err := s.Use(resolved); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Using PHP %s\n", resolved)
	return nil
}

func (a *App) install(ctx context.Context, s *store.Store, requested string) (string, error) {
	if requested == "auto" {
		v, _, err := findVersionFile()
		if err != nil {
			return "", err
		}
		if v == "" {
			return "", fmt.Errorf("no .php-version found")
		}
		requested = v
	}
	p, err := provider()
	if err != nil {
		return "", err
	}
	rel, err := p.Resolve(ctx, requested)
	if err != nil {
		return "", err
	}
	if s.IsInstalled(rel.Version) {
		fmt.Fprintf(a.Out, "PHP %s is already installed\n", rel.Version)
		return rel.Version, nil
	}
	fmt.Fprintf(a.Out, "Installing PHP %s...\n", rel.Version)
	if err := s.Install(ctx, rel.Version, rel.URL, rel.SHA256); err != nil {
		return "", err
	}
	fmt.Fprintf(a.Out, "Installed PHP %s\n", rel.Version)
	return rel.Version, nil
}

func provider() (*windowsphp.Provider, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("binary installation currently supports Windows only")
	}
	return windowsphp.New(), nil
}

func rootDir() (string, error) {
	if v := os.Getenv("PHPVM_ROOT"); v != "" {
		return filepath.Abs(v)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".phpvm"), nil
}

func findVersionFile() (string, string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	for {
		path := filepath.Join(dir, ".php-version")
		b, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimSpace(string(b)), path, nil
		}
		if !os.IsNotExist(err) {
			return "", "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", nil
		}
		dir = parent
	}
}

func (a *App) help() {
	fmt.Fprint(a.Out, `phpvm - a small PHP version manager

Usage:
  phpvm                         Use the version from .php-version
  phpvm use <version|latest>    Install and activate a PHP version
  phpvm install <version>       Install without activating
  phpvm ls                      List installed versions
  phpvm ls-remote               List available versions
  phpvm current                 Show the active version
  phpvm uninstall <version>     Remove an installed version
  phpvm prune                   Remove every version except the active one
  phpvm version                 Show phpvm's version

Environment:
  PHPVM_ROOT                    Storage root (default: ~/.phpvm)
`)
}
