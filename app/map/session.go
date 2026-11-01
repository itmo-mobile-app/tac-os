package tacticalmap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

// Run preserves the offline static demo. It does not stop WM.
func Run(ctx context.Context, client Client, cfg Config, targets []Target, log io.Writer) error {
	return runMap(ctx, client, cfg, targets, nil, nil, log)
}

func runMap(ctx context.Context, client Client, cfg Config, targets []Target, updates <-chan Target, notices <-chan string, log io.Writer) (result error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	targets = append([]Target(nil), targets...)
	commands, markers, err := BuildFrame(cfg, targets)
	if err != nil {
		return err
	}
	stream, err := client.subscribe(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	defer stream.close()
	if err := client.Send(ctx, Command{Command: "CREATE", WindowID: "map", Title: "Tactical Map", Width: cfg.Width, Height: cfg.Height}); err != nil {
		return err
	}
	windowExists := true
	defer func() {
		if windowExists {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), client.timeout())
			defer cancel()
			if err := client.Send(cleanupCtx, Command{Command: "CLOSE", WindowID: "map"}); err != nil {
				result = errors.Join(result, fmt.Errorf("close Map window: %w", err))
			}
		}
	}()
	for _, command := range commands {
		if err := client.Send(ctx, command); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
	if err := client.Send(ctx, Command{Command: "SHOW", WindowID: "map"}); err != nil {
		return err
	}
	PrintMarkers(log, markers, len(targets))
	fmt.Fprintln(log, "Map shown. Close the window or press Ctrl+C to exit.")
	type windowEvent struct {
		response response
		err      error
	}
	windowEvents := make(chan windowEvent, 1)
	eventsDone := make(chan struct{})
	go func() {
		defer close(eventsDone)
		for {
			event, err := readResponse(stream.reader)
			select {
			case windowEvents <- windowEvent{event, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	defer func() {
		cancel()
		stream.close()
		<-eventsDone
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case event := <-windowEvents:
			if event.err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("WM event stream: %w", event.err)
			}
			if event.response.Event == "window_close" && event.response.WindowID == "map" {
				// WM destroys the window before broadcasting this event.
				windowExists = false
				fmt.Fprintln(log, "Map window closed.")
				return nil
			}
		case target, ok := <-updates:
			if !ok {
				updates = nil
				continue
			}
			found := false
			for i := range targets {
				if targets[i].TargetID == target.TargetID {
					targets[i], found = target, true
					break
				}
			}
			if !found {
				targets = append(targets, target)
			}
			commands, markers, err := BuildFrame(cfg, targets)
			if err != nil {
				return err
			}
			for _, command := range commands {
				if err := client.Send(ctx, command); err != nil {
					if ctx.Err() != nil {
						return nil
					}
					var rejected *wmRejection
					if errors.As(err, &rejected) && rejected.message == "no window; send CREATE first" {
						// A native close may destroy the window during a redraw. WM broadcasts
						// window_close before accepting the next command; let its reader catch up.
						timer := time.NewTimer(client.timeout())
						defer timer.Stop()
						select {
						case event := <-windowEvents:
							if event.err == nil && event.response.Event == "window_close" && event.response.WindowID == "map" {
								windowExists = false
								fmt.Fprintln(log, "Map window closed.")
								return nil
							}
						case <-ctx.Done():
							return nil
						case <-timer.C:
						}
					}
					return err
				}
			}
			fmt.Fprintf(log, "Target updated: %s\n", target.TargetID)
			PrintMarkers(log, markers, len(targets))
		case message, ok := <-notices:
			if !ok {
				notices = nil
				continue
			}
			fmt.Fprintln(log, message)
		}
	}
}

func PrintMarkers(w io.Writer, markers []Marker, total int) {
	for _, marker := range markers {
		fmt.Fprintf(w, "%s: lat %.4f, lon %.4f -> (%d, %d), probability %.0f%%\n",
			marker.Target.TargetID, marker.Target.Latitude, marker.Target.Longitude,
			marker.Pixel.X, marker.Pixel.Y, marker.Target.Probability*100)
	}
	fmt.Fprintf(w, "%d visible, %d outside map\n", len(markers), total-len(markers))
}
