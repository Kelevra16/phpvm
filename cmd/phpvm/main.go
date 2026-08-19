package main

import (
	"context"
	"fmt"
	"os"

	"github.com/megaj/phpvm/internal/app"
)

var version = "dev"

func main() {
	if err := app.New(version).Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "phpvm:", err)
		os.Exit(1)
	}
}
