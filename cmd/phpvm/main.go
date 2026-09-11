package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"

	"github.com/Kelevra16/phpvm/internal/app"
	"github.com/Kelevra16/phpvm/internal/update"
)

var version = "dev"

func main() {
	update.CleanupPrevious()
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
	if err := app.New(version).Run(ctx, os.Args[1:]); err != nil {
		prefix := "x phpvm:"
		if st, statErr := os.Stderr.Stat(); statErr == nil && st.Mode()&os.ModeCharDevice != 0 && os.Getenv("NO_COLOR") == "" {
			prefix = "\x1b[31m×\x1b[0m phpvm:"
		}
		fmt.Fprintln(os.Stderr, prefix, app.FriendlyError(err))
		code := 1
		if strings.HasPrefix(err.Error(), "usage:") || strings.HasPrefix(err.Error(), "unknown command") {
			code = 2
		}
		var child *exec.ExitError
		if errors.As(err, &child) {
			code = child.ExitCode()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			code = 124
		}
		os.Exit(code)
	}
}
