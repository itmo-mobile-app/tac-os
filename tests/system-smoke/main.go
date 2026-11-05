package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

func run(ctx context.Context, root string, output io.Writer) (result error) {
	// A short path also fits Unix socket limits on macOS during development.
	dir, err := os.MkdirTemp("/tmp", "tac-smoke-")
	if err != nil {
		return err
	}
	fmt.Fprintln(output, "Resources:", dir)
	defer func() {
		result = errors.Join(result, os.RemoveAll(dir), checkAbsent(dir))
		if result == nil {
			fmt.Fprintln(output, "PASS cleanup")
		}
	}()
	buildCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	b, err := buildComponents(buildCtx, root, dir)
	cancel()
	if err != nil {
		return err
	}
	fmt.Fprintln(output, "PASS build components (offline, headless)")
	if err := positiveScenario(ctx, root, dir, b, output); err != nil {
		return err
	}
	if err := unavailableBrokerScenario(ctx, b.target); err != nil {
		return err
	}
	fmt.Fprintln(output, "PASS unavailable Broker")
	return nil
}

func main() {
	timeout := flag.Duration("timeout", 3*time.Minute, "Overall deadline, including builds")
	flag.Parse()
	if flag.NArg() != 0 || *timeout <= 0 {
		fmt.Fprintln(os.Stderr, "usage: system-smoke [-timeout 3m]")
		os.Exit(2)
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		fmt.Fprintln(os.Stderr, "system-smoke requires Linux (macOS is supported for development)")
		os.Exit(2)
	}
	root, err := repositoryRoot()
	if err == nil {
		signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		ctx, cancel := context.WithTimeout(signalCtx, *timeout)
		err = run(ctx, root, os.Stdout)
		cancel()
		stop()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "FAIL:", err)
		os.Exit(1)
	}
}
