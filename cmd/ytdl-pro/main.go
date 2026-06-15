package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ytdl-pro/internal/ytdlpro"
)

func main() {
	cfg, err := ytdlpro.ParseConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	cfg, err = ytdlpro.CompleteInteractive(os.Stdin, os.Stdout, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	if err := ytdlpro.Run(ctx, cfg); err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "error: cancelled")
			os.Exit(130)
		}

		if errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintf(os.Stderr, "error: timed out after %s\n", cfg.Timeout.Round(time.Second))
			os.Exit(124)
		}

		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
