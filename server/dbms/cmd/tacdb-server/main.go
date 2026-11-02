// tacdb-server runs the Linux Unix-socket DBMS; tacdb remains the stdin CLI.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/itmo-mobile-app/tac-os/server/dbms/internal/server"
)

func main() { os.Exit(run()) }
func run() int {
	path := flag.String("socket", "/tmp/tac-os-dbms.sock", "Unix socket path")
	timeout := flag.Duration("io-timeout", 5*time.Second, "read/write deadline per frame")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "Unexpected positional arguments")
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	s, err := server.Listen(*path, *timeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer s.Close()
	fmt.Fprintln(os.Stdout, "Listening on", *path)
	if err = s.Serve(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
