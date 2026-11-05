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
	broker := flag.String("broker", "", "Broker host:port; enable live targets instead of demo targets")
	observations := flag.String("observations", "", "JSONL Observation fixture to publish once; requires -broker")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments; use -help")
	}
	if (*observations != "" && *broker == "") || (*preview != "" && (*broker != "" || *observations != "")) {
		return fmt.Errorf("-observations requires -broker; -preview cannot be combined with Broker mode")
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
	client := tacticalmap.Client{Socket: *socket, EventSocket: *events}
	if *broker != "" {
		options := tacticalmap.BrokerOptions{Address: *broker}
		if *observations != "" {
			file, err := os.Open(*observations)
			if err != nil {
				return err
			}
			options.Observations, err = tacticalmap.LoadObservations(file)
			_ = file.Close()
			if err != nil {
				return err
			}
		}
		return tacticalmap.RunWithBroker(ctx, client, cfg, options, os.Stdout)
	}
	return tacticalmap.Run(ctx, client, cfg, targets, os.Stdout)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "map-demo:", err)
		os.Exit(1)
	}
}
