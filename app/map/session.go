package tacticalmap

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// Run draws one static frame, then blocks on window events. It does not stop WM.
func Run(ctx context.Context, client Client, cfg Config, targets []Target, log io.Writer) (result error) {
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
	for {
		event, err := readResponse(stream.reader)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("WM event stream: %w", err)
		}
		if event.Event == "window_close" && event.WindowID == "map" {
			// The supplied WM already destroys the window before broadcasting this event.
			windowExists = false
			fmt.Fprintln(log, "Map window closed.")
			return nil
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
