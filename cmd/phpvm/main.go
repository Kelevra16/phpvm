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
)

var version = "dev"

func main() {
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
		fmt.Fprintln(os.Stderr, "phpvm:", err)
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
