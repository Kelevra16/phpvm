package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/Kelevra16/phpvm/internal/app"
	"github.com/Kelevra16/phpvm/internal/update"
)

var version = "dev"

func main() {
	if err := update.EnsurePHPShim(version); err != nil {
		fmt.Fprintln(os.Stderr, "phpvm: could not refresh native php.exe shim:", err)
	} else {
		update.CleanupPrevious()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if raw := os.Getenv("PHPVM_TIMEOUT"); raw != "" && raw != "0" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			fmt.Fprintln(os.Stderr, "phpvm: invalid PHPVM_TIMEOUT:", err)
			os.Exit(2)
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(os.Args[0])), ".exe")
	args := os.Args[1:]
	shim := name == "php"
	if shim {
		exe, err := os.Executable()
		if err != nil {
			fail(err, "php", false)
		}
		phpvm := filepath.Join(filepath.Dir(exe), "phpvm.exe")
		cmd := exec.CommandContext(ctx, phpvm, append([]string{"_php-shim", "--"}, args...)...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fail(err, "php", true)
		}
		return
	}
	if err := app.New(version).Run(ctx, args); err != nil {
		silentChild := len(args) > 0 && args[0] == "_php-shim"
		fail(err, "phpvm", silentChild)
	}
}

func fail(err error, name string, silentChild bool) {
	var child *exec.ExitError
	if silentChild && errors.As(err, &child) {
		os.Exit(child.ExitCode())
	}
	prefix := "x " + name + ":"
	if st, statErr := os.Stderr.Stat(); statErr == nil && st.Mode()&os.ModeCharDevice != 0 && os.Getenv("NO_COLOR") == "" {
		prefix = "\x1b[31m×\x1b[0m " + name + ":"
	}
	fmt.Fprintln(os.Stderr, prefix, app.FriendlyError(err))
	code := 1
	if strings.HasPrefix(err.Error(), "usage:") || strings.HasPrefix(err.Error(), "unknown command") {
		code = 2
	}
	if errors.As(err, &child) {
		code = child.ExitCode()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		code = 124
	}
	os.Exit(code)
}
