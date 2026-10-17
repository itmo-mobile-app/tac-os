package tacticalmap

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
)

func nrgba(c *Color) color.NRGBA { return color.NRGBA{R: c.R, G: c.G, B: c.B, A: c.A} }

// RenderPreview is a software interpreter of the drawing contract for local QA.
// It does not provide a WM implementation or open a graphical window.
func RenderPreview(cfg Config, commands []Command) (*image.RGBA, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	canvas := image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))
	started, presented := false, false
	for _, command := range commands {
		if command.WindowID != "map" || presented {
			return nil, fmt.Errorf("unexpected window or command after PRESENT")
		}
		if command.Command != "BEGIN_FRAME" && !started {
			return nil, fmt.Errorf("BEGIN_FRAME must precede drawing")
		}
		switch command.Command {
		case "BEGIN_FRAME":
			if started || command.Color == nil {
				return nil, fmt.Errorf("BEGIN_FRAME requires a color and must occur once")
			}
			started = true
			draw.Draw(canvas, canvas.Bounds(), image.NewUniform(nrgba(command.Color)), image.Point{}, draw.Src)
		case "DRAW_IMAGE":
			if command.X == nil || command.Y == nil || *command.X != 0 || *command.Y != 0 || command.Width != cfg.Width || command.Height != cfg.Height {
				return nil, fmt.Errorf("prototype DRAW_IMAGE must cover the entire fixed-size map")
			}
			file, err := os.Open(command.ImagePath)
			if err != nil {
				return nil, fmt.Errorf("open drawing PNG: %w", err)
			}
			background, decodeErr := png.Decode(file)
			closeErr := file.Close()
			if decodeErr != nil {
				return nil, fmt.Errorf("decode drawing PNG: %w", decodeErr)
			}
			if closeErr != nil {
				return nil, closeErr
			}
			if background.Bounds().Dx() != cfg.Width || background.Bounds().Dy() != cfg.Height {
				return nil, fmt.Errorf("drawing PNG dimensions do not match map")
			}
			draw.Draw(canvas, canvas.Bounds(), background, background.Bounds().Min, draw.Over)
		case "DRAW_CIRCLE":
			if command.X == nil || command.Y == nil || command.Color == nil || command.Radius <= 0 || command.Radius > 128 {
				return nil, fmt.Errorf("DRAW_CIRCLE requires x, y, radius in [1, 128] and color")
			}
			cx, cy, radius := *command.X, *command.Y, command.Radius
			if cx < 0 || cx >= cfg.Width || cy < 0 || cy >= cfg.Height {
				return nil, fmt.Errorf("circle center must be inside the map")
			}
			fill := image.NewUniform(nrgba(command.Color))
			for dy := -radius; dy <= radius; dy++ {
				dx := int(math.Sqrt(float64(radius*radius - dy*dy)))
				row := image.Rect(cx-dx, cy+dy, cx+dx+1, cy+dy+1).Intersect(canvas.Bounds())
				draw.Draw(canvas, row, fill, image.Point{}, draw.Over)
			}
		case "PRESENT":
			presented = true
		default:
			return nil, fmt.Errorf("unsupported preview command %q", command.Command)
		}
	}
	if !presented {
		return nil, fmt.Errorf("frame is missing PRESENT")
	}
	return canvas, nil
}

func SavePNG(path string, img image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create preview: %w", err)
	}
	encodeErr := png.Encode(file, img)
	closeErr := file.Close()
	if encodeErr != nil {
		return fmt.Errorf("encode preview: %w", encodeErr)
	}
	return closeErr
}
