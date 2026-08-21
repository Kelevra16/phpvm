package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Kelevra16/phpvm/internal/store"
	"github.com/Kelevra16/phpvm/internal/windowsphp"
)

func (a *App) resolve(s *store.Store, args []string) error {
	fs := flag.NewFlagSet("resolve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	showPath := fs.Bool("path", false, "")
	tool := fs.String("tool", "php", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) > 1 {
		return fmt.Errorf("usage: phpvm resolve [--path] [--tool php|phpize] [version]")
	}
	if *tool != "php" && *tool != "phpize" {
		return fmt.Errorf("tool must be php or phpize")
	}
	query := ""
	if len(rest) == 1 {
		query = rest[0]
	}
	id, err := resolveRuntimeBuild(s, query)
	if err != nil {
		return err
	}
	if !*showPath {
		fmt.Fprintln(a.Out, id)
		return nil
	}
	path := s.Executable(id)
	if *tool == "phpize" {
		path = filepath.Join(filepath.Dir(path), "phpize.bat")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("%s is unavailable in build %s", *tool, id)
	}
	fmt.Fprintln(a.Out, path)
	return nil
}

func resolveRuntimeBuild(s *store.Store, explicit string) (string, error) {
	if explicit == "" {
		if active := os.Getenv("PHPVM_ACTIVE"); active != "" {
			if s.IsInstalled(active) {
				return active, nil
			}
			return "", fmt.Errorf("session build %s is no longer installed", active)
		}
	}
	query := explicit
	variant, arch := "", ""
	source := ""
	if query == "" {
		cfg, err := findProjectConfig()
		if err != nil {
			return "", err
		}
		query = cfg.Version
		variant = cfg.Variant
		arch = cfg.Arch
		source = cfg.Source
	}
	if query == "" {
		return s.Current()
	}
	aliases, _, err := readAliases(s.Root)
	if err != nil {
		return "", err
	}
	if v, ok := aliases[query]; ok {
		query = v
	}
	builds, err := s.Installed()
	if err != nil {
		return "", err
	}
	sort.Slice(builds, func(i, j int) bool { return comparePHP(builds[i].Version, builds[j].Version) > 0 })
	for _, m := range builds {
		if variant != "" && m.Variant != variant {
			continue
		}
		if arch != "" && m.Arch != arch {
			continue
		}
		if m.ID() == query || m.Version == query || query == "latest" || windowsphp.Satisfies(m.Version, query) {
			return m.ID(), nil
		}
	}
	if source != "" {
		return "", fmt.Errorf("project requires PHP %s (%s), but no installed build matches; run phpvm sync", query, source)
	}
	return "", fmt.Errorf("no installed PHP build matches %s", query)
}
func comparePHP(a, b string) int {
	var aa, bb [3]int
	fmt.Sscanf(a, "%d.%d.%d", &aa[0], &aa[1], &aa[2])
	fmt.Sscanf(b, "%d.%d.%d", &bb[0], &bb[1], &bb[2])
	for i := 0; i < 3; i++ {
		if aa[i] < bb[i] {
			return -1
		}
		if aa[i] > bb[i] {
			return 1
		}
	}
	return 0
}

func (a *App) shell(ctx context.Context, s *store.Store, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: phpvm shell [version|--current]")
	}
	id := ""
	var err error
	if len(args) == 1 && args[0] == "--current" {
		id, err = s.Current()
	} else if len(args) == 1 {
		id, err = a.install(ctx, s, args[0], defaultOptions())
	} else {
		id, err = resolveRuntimeBuild(s, "")
		if err != nil {
			cfg, cfgErr := findProjectConfig()
			if cfgErr != nil {
				return cfgErr
			}
			if cfg.Version == "" {
				return err
			}
			o := defaultOptions()
			if cfg.Variant != "" {
				o.variant = cfg.Variant
			}
			if cfg.Arch != "" {
				o.arch = cfg.Arch
			}
			id, err = a.install(ctx, s, cfg.Version, o)
		}
	}
	if err != nil {
		return err
	}
	phpDir := filepath.Dir(s.Executable(id))
	promptID := strings.ReplaceAll(id, "'", "")
	script := fmt.Sprintf("$env:PHPVM_ACTIVE='%s'; $env:Path='%s;'+$env:Path; function global:prompt { '[phpvm:%s] ' + (Get-Location) + '> ' }; Write-Host 'PHP session %s. Type exit to return.' -ForegroundColor Cyan", promptID, strings.ReplaceAll(phpDir, "'", "''"), promptID, promptID)
	cmd := exec.CommandContext(ctx, "powershell", "-NoExit", "-Command", script)
	cmd.Stdin = os.Stdin
	cmd.Stdout = a.Out
	cmd.Stderr = a.Err
	return cmd.Run()
}
