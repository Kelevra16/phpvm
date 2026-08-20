package app

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Kelevra16/phpvm/internal/store"
)

func (a *App) logs(ctx context.Context, s *store.Store, args []string) error {
	if len(args) == 0 {
		args = []string{"show"}
	}
	path, iniPath, err := ensureLogConfig(s)
	if err != nil {
		return err
	}
	switch args[0] {
	case "path":
		fmt.Fprintln(a.Out, path)
		return nil
	case "show", "tail":
		fs := flag.NewFlagSet("logs "+args[0], flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		lines := fs.Int("lines", 100, "")
		follow := fs.Bool("follow", false, "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if args[0] == "show" && *follow {
			return fmt.Errorf("--follow is only valid with logs tail")
		}
		return tailLog(ctx, path, *lines, *follow, a.Out)
	case "open":
		return openFile(path)
	case "clear":
		force := len(args) > 1 && args[1] == "--force"
		if !force {
			return fmt.Errorf("logs clear is destructive; rerun with --force")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		return os.WriteFile(path, nil, 0644)
	case "doctor":
		return a.logsDoctor(s, path, iniPath)
	default:
		return fmt.Errorf("usage: phpvm logs path|show [--lines N]|tail [--lines N] [--follow]|open|clear --force|doctor")
	}
}

func ensureLogConfig(s *store.Store) (string, string, error) {
	id, dir, err := activeDir(s)
	if err != nil {
		return "", "", err
	}
	iniPath, err := ensureINI(dir)
	if err != nil {
		return "", "", err
	}
	logPath := filepath.Join(s.Root, "logs", id, "php-error.log")
	cfg, err := findProjectConfig()
	if err != nil {
		return "", "", err
	}
	if cfg.LogScope == "project" {
		base := filepath.Dir(cfg.Source)
		if cfg.LogPath != "" {
			logPath = cfg.LogPath
			if !filepath.IsAbs(logPath) {
				logPath = filepath.Join(base, logPath)
			}
		} else {
			logPath = filepath.Join(base, ".phpvm", "php-error.log")
		}
	} else if cfg.LogPath != "" {
		logPath = cfg.LogPath
		if !filepath.IsAbs(logPath) {
			logPath = filepath.Join(filepath.Dir(cfg.Source), logPath)
		}
	}
	logPath, err = filepath.Abs(logPath)
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return "", "", err
	}
	if err := setINI(iniPath, "log_errors", "On"); err != nil {
		return "", "", err
	}
	if err := setINI(iniPath, "error_log", strconv.Quote(logPath)); err != nil {
		return "", "", err
	}
	return logPath, iniPath, nil
}

func tailLog(ctx context.Context, path string, n int, follow bool, out io.Writer) error {
	if n < 0 {
		return fmt.Errorf("lines must be non-negative")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if !follow {
			return nil
		}
		if err := os.WriteFile(path, nil, 0644); err != nil {
			return err
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimRight(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	start := len(lines) - n
	if start < 0 {
		start = 0
	}
	for _, line := range lines[start:] {
		fmt.Fprintln(out, line)
	}
	if !follow {
		return nil
	}
	offset := int64(len(b))
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			f, err := os.Open(path)
			if err != nil {
				continue
			}
			st, err := f.Stat()
			if err != nil {
				f.Close()
				continue
			}
			if st.Size() < offset {
				offset = 0
			}
			_, _ = f.Seek(offset, io.SeekStart)
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				fmt.Fprintln(out, sc.Text())
			}
			pos, _ := f.Seek(0, io.SeekCurrent)
			offset = pos
			f.Close()
		}
	}
}
func openFile(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, nil, 0644); err != nil {
			return err
		}
	}
	if runtime.GOOS == "windows" {
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
	}
	if runtime.GOOS == "darwin" {
		return exec.Command("open", path).Start()
	}
	return exec.Command("xdg-open", path).Start()
}
func (a *App) logsDoctor(s *store.Store, path, iniPath string) error {
	values, err := readINI(iniPath)
	if err != nil {
		return err
	}
	checks := []struct {
		name   string
		ok     bool
		detail string
	}{{"log_errors", strings.EqualFold(strings.Trim(values["log_errors"], "\""), "on"), values["log_errors"]}, {"error_log", samePath(strings.Trim(values["error_log"], "\""), path), values["error_log"]}}
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
		checks = append(checks, struct {
			name   string
			ok     bool
			detail string
		}{"writable", true, path})
		f.Close()
	} else {
		checks = append(checks, struct {
			name   string
			ok     bool
			detail string
		}{"writable", false, err.Error()})
	}
	failed := false
	for _, c := range checks {
		mark := "OK"
		if !c.ok {
			mark = "FAIL"
			failed = true
		}
		fmt.Fprintf(a.Out, "%-4s %-12s %s\n", mark, c.name, c.detail)
	}
	if failed {
		return fmt.Errorf("log configuration has problems")
	}
	return nil
}
func samePath(a, b string) bool {
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	return strings.EqualFold(filepath.Clean(aa), filepath.Clean(bb))
}
