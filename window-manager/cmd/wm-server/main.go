package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/itmo-mobile-app/tac-os/window-manager/backend"
	"github.com/itmo-mobile-app/tac-os/window-manager/backend/headless"
	"github.com/itmo-mobile-app/tac-os/window-manager/backend/sdl"
	"github.com/itmo-mobile-app/tac-os/window-manager/server"
)

func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	socket := flag.String("socket", "/tmp/tacos-wm.sock", "Unix socket path")
	backendName := flag.String("backend", "sdl", "backend: headless or sdl")
	flag.Parse()

	var b backend.Backend
	switch *backendName {
	case "headless":
		b = headless.New()
	case "sdl":
		b = sdl.New()
	default:
		fmt.Fprintln(os.Stderr, "unknown backend:", *backendName)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	s := &server.Server{SocketPath: *socket, Backend: b}
	if err := s.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "wm-server:", err)
		os.Exit(1)
	}
}
