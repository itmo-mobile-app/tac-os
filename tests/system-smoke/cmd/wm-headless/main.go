// wm-headless only configures the production server's two private socket paths.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/itmo-mobile-app/tac-os/window-manager/backend/headless"
	"github.com/itmo-mobile-app/tac-os/window-manager/server"
)

func main() {
	command := flag.String("socket", "", "Command socket path (required)")
	events := flag.String("events", "", "Event socket path (required)")
	flag.Parse()
	if *command == "" || *events == "" || *command == *events || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: wm-headless -socket PATH -events OTHER_PATH")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	wm := server.Server{SocketPath: *command, EventSocketPath: *events, Backend: headless.New()}
	if err := wm.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "wm-headless:", err)
		os.Exit(1)
	}
}
