package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tacticalmap "github.com/itmo-mobile-app/tac-os/app/map"
)

func run() error {
	config := flag.String("config", "assets/test-map.json", "Map metadata JSON")
	socket := flag.String("socket", "/tmp/tacos-wm.sock", "WM command socket")
	events := flag.String("events", "/tmp/tacos-wm-events.sock", "WM event socket")
	preview := flag.String("preview", "", "Save a PNG instead of connecting to WM")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments; use -help")
	}
	cfg, err := tacticalmap.LoadConfig(*config)
	if err != nil {
		return err
	}
	targets := tacticalmap.DemoTargets()
	if *preview != "" {
		output, err := filepath.Abs(*preview)
		if err != nil {
			return err
		}
		metadata, err := filepath.Abs(*config)
		if err != nil {
			return err
		}
		if output == cfg.Image || output == metadata {
			return fmt.Errorf("preview must not overwrite the source image or metadata")
		}
		commands, markers, err := tacticalmap.BuildFrame(cfg, targets)
		if err != nil {
			return err
		}
		img, err := tacticalmap.RenderPreview(cfg, commands)
		if err != nil {
			return err
		}
		if err := tacticalmap.SavePNG(output, img); err != nil {
			return err
		}
		tacticalmap.PrintMarkers(os.Stdout, markers, len(targets))
		fmt.Fprintln(os.Stdout, "Saved preview:", output)
		return nil
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return tacticalmap.Run(ctx, tacticalmap.Client{Socket: *socket, EventSocket: *events}, cfg, targets, os.Stdout)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "map-demo:", err)
		os.Exit(1)
	}
}
