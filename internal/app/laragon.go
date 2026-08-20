package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Kelevra16/phpvm/internal/store"
)

func laragonRoot() (string, error) {
	candidates := []string{os.Getenv("LARAGON_ROOT"), `C:\laragon`, `C:\tools\laragon`}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		full, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if st, err := os.Stat(filepath.Join(full, "bin")); err == nil && st.IsDir() {
			return full, nil
		}
	}
	return "", fmt.Errorf("Laragon was not found; set LARAGON_ROOT to its installation directory")
}
func (a *App) laragon(s *store.Store, args []string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("Laragon integration supports Windows only")
	}
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("usage: phpvm laragon <detect|link|unlink> [build]")
	}
	root, err := laragonRoot()
	if err != nil {
		return err
	}
	if args[0] == "detect" {
		if len(args) != 1 {
			return fmt.Errorf("usage: phpvm laragon detect")
		}
		fmt.Fprintln(a.Out, root)
		return nil
	}
	id, err := s.Current()
	if len(args) == 2 {
		id, err = resolveInstalled(s, args[1])
	}
	if err != nil {
		return err
	}
	phpRoot := filepath.Join(root, "bin", "php")
	if err := os.MkdirAll(phpRoot, 0755); err != nil {
		return err
	}
	link := filepath.Join(phpRoot, "phpvm-"+id)
	cleanRoot := filepath.Clean(phpRoot) + string(os.PathSeparator)
	if !strings.HasPrefix(filepath.Clean(link)+string(os.PathSeparator), cleanRoot) {
		return fmt.Errorf("invalid Laragon link path")
	}
	switch args[0] {
	case "link":
		if _, err := os.Lstat(link); err == nil {
			return fmt.Errorf("Laragon link already exists: %s", link)
		}
		out, err := exec.Command("cmd", "/c", "mklink", "/J", link, filepath.Dir(s.Executable(id))).CombinedOutput()
		if err != nil {
			return fmt.Errorf("create Laragon junction: %s: %w", strings.TrimSpace(string(out)), err)
		}
		fmt.Fprintln(a.Out, "Linked", link)
		fmt.Fprintln(a.Out, "Select phpvm-"+id, "from Laragon's PHP version menu, then reload Laragon")
		return nil
	case "unlink":
		if _, err := os.Lstat(link); err != nil {
			return fmt.Errorf("Laragon link does not exist: %s", link)
		}
		if err := os.Remove(link); err != nil {
			return err
		}
		fmt.Fprintln(a.Out, "Unlinked", link)
		return nil
	default:
		return fmt.Errorf("usage: phpvm laragon <detect|link|unlink> [build]")
	}
}
