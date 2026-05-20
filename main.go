package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/d00p1/filtrate-backups/internal/app"
	"github.com/d00p1/filtrate-backups/internal/version"
)

func main() {
	if wantsVersion(os.Args[1:]) {
		fmt.Printf("mysql-dump-cleaner %s\n", version.String())
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, os.Args[1:]); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

func wantsVersion(args []string) bool {
	for _, arg := range args {
		if arg == "--version" || arg == "-version" {
			return true
		}
	}
	return false
}
