// Command runtimehelper is a test fixture for the C++ Runtime camera client.
// It uses the detector's existing IPC server and fake sources without OpenCV.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"tac-os/marker-detect/ipc"
)

func main() { os.Exit(run()) }

func run() int {
	flags := flag.NewFlagSet("marker-detect test helper", flag.ContinueOnError)
	socketPath := flags.String("socket", "", "Unix-domain socket path")
	mode := flags.String("fake", "", "required synthetic detector mode")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return 2
	}
	if *socketPath == "" || *mode == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "test helper requires -socket PATH -fake MODE")
		return 2
	}
	source, err := ipc.NewFakeSource(*mode)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	listener, err := ipc.Listen(*socketPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer listener.Close()
	server := ipc.NewServer(source)
	defer server.Close()
	go func() {
		<-ctx.Done()
		server.Close()
	}()
	if err := server.Serve(listener); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
